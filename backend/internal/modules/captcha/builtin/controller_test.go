package builtin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/settings"
)

type routeErrorBody struct {
	Type  string  `json:"type"`
	Code  string  `json:"code"`
	Param *string `json:"param"`
}

type routeErrorEnvelope struct {
	Error routeErrorBody `json:"error"`
}

func newChallengeRouteTestApp(t *testing.T, db *database.Database, policies Policies) (*httptest.Server, *settings.Service) {
	t.Helper()

	settingsSvc := newTestSettings(t, db)
	service := newService(settingsSvc, newRepository(db), mustGenerator(t), ratelimit.New(), policies)
	controller := NewController(service)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	controller.Register(engine.Group("/api/v1"))

	ts := httptest.NewServer(engine)
	t.Cleanup(ts.Close)
	return ts, settingsSvc
}

func postJSONBody(t *testing.T, url, contentType, body string) (*http.Response, []byte) {
	t.Helper()

	request, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer response.Body.Close()

	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return response, data
}

func decodeError(t *testing.T, data []byte) routeErrorBody {
	t.Helper()

	var envelope routeErrorEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode error envelope: %v (%s)", err, data)
	}
	return envelope.Error
}

func TestChallengeRouteRequiresJSON(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "text/plain", `{"scope":"login"}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.StatusCode)
	}
	if body := decodeError(t, data); body.Code != "invalid_request" {
		t.Fatalf("code = %q, want invalid_request", body.Code)
	}
}

func TestChallengeRouteCreatesChallenge(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "application/json", `{"scope":"login"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", response.StatusCode, data)
	}

	var challenge ChallengeResponse
	if err := json.Unmarshal(data, &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	if challenge.ID == uuid.Nil {
		t.Fatal("id is empty")
	}
	if !strings.HasPrefix(challenge.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", challenge.MasterImage)
	}
	if !strings.HasPrefix(challenge.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q", challenge.TileImage)
	}
	if _, err := time.Parse(time.RFC3339, challenge.ExpiresAt); err != nil {
		t.Fatalf("expires_at = %q: %v", challenge.ExpiresAt, err)
	}
}

func TestChallengeRouteRejectsUnknownScope(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "application/json", `{"scope":"signup"}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.StatusCode)
	}
	body := decodeError(t, data)
	if body.Code != "invalid_request" || body.Param == nil || *body.Param != "scope" {
		t.Fatalf("error = %+v, want invalid_request param scope", body)
	}
}

func TestChallengeRouteRejectsWhenInactive(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	cases := []struct {
		name     string
		enabled  bool
		provider string
	}{
		{"disabled", false, "builtin"},
		{"turnstile", true, "turnstile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setCaptchaSettings(t, settingsSvc, tc.enabled, tc.provider, "login,register")
			response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "application/json", `{"scope":"login"}`)
			if response.StatusCode != http.StatusForbidden {
				t.Fatalf("status = %d, want 403 (%s)", response.StatusCode, data)
			}
			if body := decodeError(t, data); body.Code != "captcha_disabled" {
				t.Fatalf("code = %q, want captcha_disabled", body.Code)
			}
		})
	}
}

func TestChallengeRouteRateLimited(t *testing.T) {
	db := testDB(t)
	policies := DefaultPolicies()
	policies.ChallengeLimit = 1
	ts, settingsSvc := newChallengeRouteTestApp(t, db, policies)
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "application/json", `{"scope":"login"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201 (%s)", response.StatusCode, data)
	}
	var challenge ChallengeResponse
	if err := json.Unmarshal(data, &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	response, data = postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "application/json", `{"scope":"login"}`)
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429 (%s)", response.StatusCode, data)
	}
	if body := decodeError(t, data); body.Code != "rate_limited" {
		t.Fatalf("code = %q, want rate_limited", body.Code)
	}
	if response.Header.Get("Retry-After") == "" {
		t.Fatal("Retry-After header is empty")
	}
}

func TestSolveRouteRejectsMalformedID(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges/not-a-uuid/solutions", "application/json", `{"x":1,"y":2}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.StatusCode)
	}
	body := decodeError(t, data)
	if body.Code != "invalid_request" || body.Param == nil || *body.Param != "id" {
		t.Fatalf("error = %+v, want invalid_request param id", body)
	}
}

func TestSolveRouteRejectsMissingAndNegativeCoordinates(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	challenge := createChallengeViaRoute(t, ts, db)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	cases := []struct {
		name  string
		body  string
		param string
	}{
		{"missing x", `{}`, "x"},
		{"missing y", `{"x":1}`, "y"},
		{"negative x", `{"x":-1,"y":2}`, "x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, data := postJSONBody(t, endpoint, "application/json", tc.body)
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
			}
			body := decodeError(t, data)
			if body.Code != "invalid_request" || body.Param == nil || *body.Param != tc.param {
				t.Fatalf("error = %+v, want invalid_request param %s", body, tc.param)
			}
		})
	}
}

func TestSolveRouteInvalidInputKeepsChallengePending(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	challenge := createChallengeViaRoute(t, ts, db)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, _ := postJSONBody(t, endpoint, "application/json", `{"x":-1,"y":2}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400", response.StatusCode)
	}

	x, y := challengeAnswer(t, db, challenge.ID)
	response, data := postJSONBody(t, endpoint, "application/json", `{"x":`+strconv.Itoa(x)+`,"y":`+strconv.Itoa(y)+`}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200 (%s)", response.StatusCode, data)
	}
}

func TestSolveRouteWrongAnswer(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	challenge := createChallengeViaRoute(t, ts, db)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, data := postJSONBody(t, endpoint, "application/json", `{"x":1,"y":1}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	body := decodeError(t, data)
	if body.Code != "captcha_failed" || body.Param != nil {
		t.Fatalf("error = %+v, want captcha_failed param nil", body)
	}

	x, y := challengeAnswer(t, db, challenge.ID)
	response, data = postJSONBody(t, endpoint, "application/json", `{"x":`+strconv.Itoa(x)+`,"y":`+strconv.Itoa(y)+`}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("solved after wrong answer status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if body := decodeError(t, data); body.Code != "captcha_failed" {
		t.Fatalf("solved-after-wrong code = %q, want captcha_failed", body.Code)
	}
}

func createChallengeViaRoute(t *testing.T, ts *httptest.Server, db *database.Database) ChallengeResponse {
	t.Helper()

	response, data := postJSONBody(t, ts.URL+"/api/v1/captcha/challenges", "application/json", `{"scope":"login"}`)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201 (%s)", response.StatusCode, data)
	}
	var challenge ChallengeResponse
	if err := json.Unmarshal(data, &challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)
	return challenge
}
