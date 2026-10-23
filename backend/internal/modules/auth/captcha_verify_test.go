package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
)

type captchaVerifyError struct {
	Code  string  `json:"code"`
	Param *string `json:"param"`
}

func loginCaptchaVerifyUser(t *testing.T, db *database.Database, verifier captchaVerifier, groupID uuid.UUID) (*httptest.Server, *http.Client) {
	t.Helper()

	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)

	user, password := createTestUser(t, db, uniqueTag(t))
	if _, err := db.Client.User.UpdateOneID(uuid.MustParse(user.ID)).SetGroupID(groupID).Save(context.Background()); err != nil {
		t.Fatalf("assign group: %v", err)
	}

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: user.Email, Password: password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	return ts, client
}

func decodeCaptchaVerifyError(t *testing.T, resp *http.Response) captchaVerifyError {
	t.Helper()
	defer resp.Body.Close()

	var body struct {
		Error captchaVerifyError `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body.Error
}

func TestVerifyCaptchaConfigPasses(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaPassed}
	ts, client := loginCaptchaVerifyUser(t, db, verifier, rbac.GroupIDOwner)

	resp := postJSON(t, client, ts.URL+"/api/v1/admin/captcha/verifications", CaptchaVerificationRequest{SecretKey: "the-secret", Token: "the-token"})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body CaptchaVerificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !body.Verified {
		t.Fatal("verified = false, want true")
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", verifier.calls)
	}
	if verifier.secret != "the-secret" || verifier.token != "the-token" {
		t.Fatalf("verifier saw secret = %q, token = %q", verifier.secret, verifier.token)
	}
	if verifier.ip == "" {
		t.Fatal("verifier ip is empty, want the request address")
	}
}

func TestVerifyCaptchaConfigMapsRejection(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaRejected}
	ts, client := loginCaptchaVerifyUser(t, db, verifier, rbac.GroupIDOwner)

	resp := postJSON(t, client, ts.URL+"/api/v1/admin/captcha/verifications", CaptchaVerificationRequest{SecretKey: "wrong-secret", Token: "the-token"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}
	body := decodeCaptchaVerifyError(t, resp)
	if body.Code != "captcha_verification_failed" {
		t.Fatalf("code = %q, want captcha_verification_failed", body.Code)
	}
	if body.Param != nil {
		t.Fatalf("param = %v, want null", body.Param)
	}
}

func TestVerifyCaptchaConfigMapsUnavailable(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaUnavailable}
	ts, client := loginCaptchaVerifyUser(t, db, verifier, rbac.GroupIDOwner)

	resp := postJSON(t, client, ts.URL+"/api/v1/admin/captcha/verifications", CaptchaVerificationRequest{SecretKey: "the-secret", Token: "the-token"})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	body := decodeCaptchaVerifyError(t, resp)
	if body.Code != "service_unavailable" {
		t.Fatalf("code = %q, want service_unavailable", body.Code)
	}
}

func TestVerifyCaptchaConfigRequiresKeys(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaPassed}
	ts, client := loginCaptchaVerifyUser(t, db, verifier, rbac.GroupIDOwner)

	cases := []CaptchaVerificationRequest{
		{SecretKey: "the-secret"},
		{Token: "the-token"},
		{},
	}
	for _, req := range cases {
		resp := postJSON(t, client, ts.URL+"/api/v1/admin/captcha/verifications", req)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %+v = %d, want %d", req, resp.StatusCode, http.StatusBadRequest)
		}
		if body := decodeCaptchaVerifyError(t, resp); body.Code != "invalid_request" {
			t.Fatalf("code for %+v = %q, want invalid_request", req, body.Code)
		}
	}

	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestVerifyCaptchaConfigRequiresJSONRequest(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaPassed}
	ts, client := loginCaptchaVerifyUser(t, db, verifier, rbac.GroupIDOwner)

	payload := `{"secret_key":"the-secret","token":"the-token"}`
	cases := []map[string]string{
		{"Content-Type": "text/plain"},
		{"Content-Type": "application/json", "Origin": "https://evil.example"},
	}
	for _, headers := range cases {
		resp := postRawWithHeaders(t, client, ts.URL+"/api/v1/admin/captcha/verifications", payload, headers)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %v = %d, want %d", headers, resp.StatusCode, http.StatusBadRequest)
		}
		if body := decodeCaptchaVerifyError(t, resp); body.Code != "invalid_request" {
			t.Fatalf("code for %v = %q, want invalid_request", headers, body.Code)
		}
	}

	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestVerifyCaptchaConfigForbiddenForPlainUser(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaPassed}
	ts, client := loginCaptchaVerifyUser(t, db, verifier, rbac.GroupIDUser)

	resp := postJSON(t, client, ts.URL+"/api/v1/admin/captcha/verifications", CaptchaVerificationRequest{SecretKey: "the-secret", Token: "the-token"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
	if body := decodeCaptchaVerifyError(t, resp); body.Code != "forbidden" {
		t.Fatalf("code = %q, want forbidden", body.Code)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestVerifyCaptchaConfigRequiresAuth(t *testing.T) {
	db := testDB(t)
	verifier := &fakeCaptchaVerifier{outcome: captchaPassed}
	ts, _ := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())

	resp := postJSON(t, &http.Client{}, ts.URL+"/api/v1/admin/captcha/verifications", CaptchaVerificationRequest{SecretKey: "the-secret", Token: "the-token"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if body := decodeCaptchaVerifyError(t, resp); body.Code != "unauthorized" {
		t.Fatalf("code = %q, want unauthorized", body.Code)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}
