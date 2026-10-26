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
	service := newService(settingsSvc, newRepository(db), mustGenerator(t), mustClickGenerator(t), ratelimit.New(), policies)
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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
			setCaptchaSettings(t, settingsSvc, tc.enabled, tc.provider, modeSlide)
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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeSlide)
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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeSlide)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, _ := postJSONBody(t, endpoint, "application/json", `{"x":-1,"y":2}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, want 400", response.StatusCode)
	}

	answer := challengeAnswer(t, db, challenge.ID)
	response, data := postJSONBody(t, endpoint, "application/json", `{"x":`+strconv.Itoa(answer[0].X)+`,"y":`+strconv.Itoa(answer[0].Y)+`}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200 (%s)", response.StatusCode, data)
	}
}

func TestSolveRouteWrongAnswer(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeSlide)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, data := postJSONBody(t, endpoint, "application/json", `{"x":1,"y":1}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	body := decodeError(t, data)
	if body.Code != "captcha_failed" || body.Param != nil {
		t.Fatalf("error = %+v, want captcha_failed param nil", body)
	}

	answer := challengeAnswer(t, db, challenge.ID)
	response, data = postJSONBody(t, endpoint, "application/json", `{"x":`+strconv.Itoa(answer[0].X)+`,"y":`+strconv.Itoa(answer[0].Y)+`}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("solved after wrong answer status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if body := decodeError(t, data); body.Code != "captcha_failed" {
		t.Fatalf("solved-after-wrong code = %q, want captcha_failed", body.Code)
	}
}

func TestChallengeRouteClickShape(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeClick)

	if !strings.HasPrefix(challenge.PromptImage, "data:image/png;base64,") {
		t.Fatalf("prompt image = %.40q", challenge.PromptImage)
	}
	if challenge.PromptText == "" {
		t.Fatal("prompt text is empty")
	}
	if challenge.ExpectedClicks != 3 {
		t.Fatalf("expected clicks = %d, want 3", challenge.ExpectedClicks)
	}
	if challenge.TileImage != "" {
		t.Fatalf("tile image = %.40q, want empty for click mode", challenge.TileImage)
	}
	if !strings.HasPrefix(challenge.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", challenge.MasterImage)
	}
}

func TestSolveRouteClickRequiresThreeClicks(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeClick)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	for _, body := range []string{
		`{"clicks":[{"x":10,"y":50},{"x":92,"y":80}]}`,
		`{"clicks":[{"x":10,"y":50},{"x":92,"y":80},{"x":170,"y":110},{"x":200,"y":150}]}`,
	} {
		response, data := postJSONBody(t, endpoint, "application/json", body)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
		}
		if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
			t.Fatalf("error = %+v, want invalid_request param clicks", got)
		}
	}

	response, data := postJSONBody(t, endpoint, "application/json", clickBody(t, challengeAnswer(t, db, challenge.ID)))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200 (%s)", response.StatusCode, data)
	}
}

func TestSolveRouteClickCountCheckedBeforeLookup(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	setCaptchaSettings(t, settingsSvc, true, "builtin", modeClick)
	missing := uuid.New()
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + missing.String() + "/solutions"

	response, data := postJSONBody(t, endpoint, "application/json", `{"clicks":[{"x":10,"y":50},{"x":92,"y":80}]}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
		t.Fatalf("error = %+v, want invalid_request param clicks", got)
	}
}

func TestSolveRouteClickRejectsMalformedClicks(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeClick)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	for _, body := range []string{
		`{"clicks":[{"y":50},{"x":92,"y":80},{"x":170,"y":110}]}`,
		`{"clicks":[{"x":10},{"x":92,"y":80},{"x":170,"y":110}]}`,
		`{"clicks":[{"x":-1,"y":50},{"x":92,"y":80},{"x":170,"y":110}]}`,
		`{"clicks":[{"x":10,"y":-5},{"x":92,"y":80},{"x":170,"y":110}]}`,
	} {
		response, data := postJSONBody(t, endpoint, "application/json", body)
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
		}
		if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
			t.Fatalf("error = %+v, want invalid_request param clicks", got)
		}
	}

	response, data := postJSONBody(t, endpoint, "application/json", clickBody(t, challengeAnswer(t, db, challenge.ID)))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200 (%s)", response.StatusCode, data)
	}
}

func TestSolveRouteClickRejectsMissingClicks(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeClick)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, data := postJSONBody(t, endpoint, "application/json", `{}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "x" {
		t.Fatalf("error = %+v, want invalid_request param x", got)
	}
}

func TestSolveRouteRejectsMixedShapes(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeClick)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, data := postJSONBody(t, endpoint, "application/json", `{"x":10,"y":50,"clicks":[{"x":10,"y":50},{"x":92,"y":80},{"x":170,"y":110}]}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
		t.Fatalf("error = %+v, want invalid_request param clicks", got)
	}
}

func TestSolveRouteSlideRejectsSingleClickShape(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeSlide)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	answer := challengeAnswer(t, db, challenge.ID)
	body, err := json.Marshal(map[string]any{"clicks": []Point{{X: answer[0].X, Y: answer[0].Y}}})
	if err != nil {
		t.Fatalf("marshal clicks: %v", err)
	}

	response, data := postJSONBody(t, endpoint, "application/json", string(body))
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
		t.Fatalf("error = %+v, want invalid_request param clicks", got)
	}
}

func TestSolveRouteSlideRejectsClickShape(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeSlide)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	response, data := postJSONBody(t, endpoint, "application/json", `{"clicks":[{"x":10,"y":50},{"x":92,"y":80},{"x":170,"y":110}]}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
		t.Fatalf("error = %+v, want invalid_request param clicks", got)
	}

	answer := challengeAnswer(t, db, challenge.ID)
	response, data = postJSONBody(t, endpoint, "application/json", `{"x":`+strconv.Itoa(answer[0].X)+`,"y":`+strconv.Itoa(answer[0].Y)+`}`)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200 (%s)", response.StatusCode, data)
	}
}

func TestSolveRouteClickRejectsSinglePointShape(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newChallengeRouteTestApp(t, db, DefaultPolicies())

	challenge := createChallengeViaRoute(t, ts, db, settingsSvc, modeClick)
	endpoint := ts.URL + "/api/v1/captcha/challenges/" + challenge.ID.String() + "/solutions"

	dots := challengeAnswer(t, db, challenge.ID)
	response, data := postJSONBody(t, endpoint, "application/json", `{"x":`+strconv.Itoa(dots[0].X)+`,"y":`+strconv.Itoa(dots[0].Y)+`}`)
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", response.StatusCode, data)
	}
	if got := decodeError(t, data); got.Code != "invalid_request" || got.Param == nil || *got.Param != "clicks" {
		t.Fatalf("error = %+v, want invalid_request param clicks", got)
	}

	response, data = postJSONBody(t, endpoint, "application/json", clickBody(t, dots))
	if response.StatusCode != http.StatusOK {
		t.Fatalf("valid status = %d, want 200 (%s)", response.StatusCode, data)
	}
}

func clickBody(t *testing.T, dots []AnswerDot) string {
	t.Helper()

	clicks := make([]Point, 0, len(dots))
	for _, dot := range dots {
		clicks = append(clicks, Point{X: dot.X, Y: dot.Y})
	}
	encoded, err := json.Marshal(map[string]any{"clicks": clicks})
	if err != nil {
		t.Fatalf("marshal clicks: %v", err)
	}
	return string(encoded)
}

func createChallengeViaRoute(t *testing.T, ts *httptest.Server, db *database.Database, settingsSvc *settings.Service, mode string) ChallengeResponse {
	t.Helper()

	setCaptchaSettings(t, settingsSvc, true, "builtin", mode)
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
