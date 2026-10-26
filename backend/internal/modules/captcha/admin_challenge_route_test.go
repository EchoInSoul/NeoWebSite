package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/settings"
)

const adminChallengePath = "/api/v1/admin/captcha/challenges"

func seedCaptchaEditUser(t *testing.T, db *database.Database, ts *httptest.Server) *http.Client {
	t.Helper()

	userID := seedUserInGroup(t, db, uniqueTag(t), captchaEditGroup(t, db))
	client := testClient(t)
	seedSession(t, client, ts, userID)
	return client
}

func decodeAdminChallenge(t *testing.T, resp *http.Response) builtin.ChallengeResponse {
	t.Helper()
	defer resp.Body.Close()

	var challenge builtin.ChallengeResponse
	if err := json.NewDecoder(resp.Body).Decode(&challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	return challenge
}

func deleteAdminChallenge(t *testing.T, db *database.Database, id uuid.UUID) {
	t.Helper()

	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(id).Exec(context.Background()) })
}

func TestAdminChallengeRouteCreatesRequestedMode(t *testing.T) {
	db := testDB(t)
	ts, _ := newCaptchaRouteTestApp(t, db, &fakeVerifier{})
	client := seedCaptchaEditUser(t, db, ts)

	cases := []struct {
		name      string
		body      map[string]any
		wantClick bool
	}{
		{"slide", map[string]any{"mode": "slide"}, false},
		{"click", map[string]any{"mode": "click"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, client, ts.URL+adminChallengePath, tc.body)
			if resp.StatusCode != http.StatusCreated {
				t.Fatalf("status = %d, want 201", resp.StatusCode)
			}

			challenge := decodeAdminChallenge(t, resp)
			deleteAdminChallenge(t, db, challenge.ID)

			row, err := db.Client.CaptchaChallenge.Get(context.Background(), challenge.ID)
			if err != nil {
				t.Fatalf("read challenge: %v", err)
			}
			if row.Scope != "test" {
				t.Fatalf("scope = %q, want test", row.Scope)
			}

			if tc.wantClick {
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
					t.Fatalf("tile image = %.40q, want empty", challenge.TileImage)
				}
				return
			}

			if !strings.HasPrefix(challenge.TileImage, "data:image/png;base64,") {
				t.Fatalf("tile image = %.40q", challenge.TileImage)
			}
			if challenge.PromptImage != "" {
				t.Fatalf("prompt image = %.40q, want empty", challenge.PromptImage)
			}
		})
	}
}

func TestAdminChallengeRouteIgnoresSavedSettings(t *testing.T) {
	db := testDB(t)
	ts, settingsSvc := newCaptchaRouteTestApp(t, db, &fakeVerifier{})
	client := seedCaptchaEditUser(t, db, ts)

	setSavedCaptcha(t, settingsSvc, false, "turnstile", "click")

	resp := postJSON(t, client, ts.URL+adminChallengePath, map[string]any{"mode": "slide"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	challenge := decodeAdminChallenge(t, resp)
	deleteAdminChallenge(t, db, challenge.ID)

	if !strings.HasPrefix(challenge.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q, want the requested slide challenge", challenge.TileImage)
	}
	if challenge.PromptImage != "" {
		t.Fatalf("prompt image = %.40q, want the requested slide challenge", challenge.PromptImage)
	}
}

func TestAdminChallengeRouteIsNotRateLimited(t *testing.T) {
	db := testDB(t)
	ts, _ := newCaptchaRouteTestApp(t, db, &fakeVerifier{})
	client := seedCaptchaEditUser(t, db, ts)

	for i := 0; i < 7; i++ {
		resp := postJSON(t, client, ts.URL+adminChallengePath, map[string]any{"mode": "slide"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("call %d status = %d, want 201", i+1, resp.StatusCode)
		}
		deleteAdminChallenge(t, db, decodeAdminChallenge(t, resp).ID)
	}
}

func TestAdminChallengeRouteRejectsUnknownMode(t *testing.T) {
	db := testDB(t)
	ts, _ := newCaptchaRouteTestApp(t, db, &fakeVerifier{})
	client := seedCaptchaEditUser(t, db, ts)

	for _, body := range []map[string]any{
		{"mode": "rotate"},
		{"mode": ""},
		{},
	} {
		resp := postJSON(t, client, ts.URL+adminChallengePath, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %v = %d, want 400", body, resp.StatusCode)
		}
		code, param := decodeVerifyError(t, resp)
		if code != "invalid_request" || param == nil || *param != "mode" {
			t.Fatalf("error for %v = %q param %v, want invalid_request param mode", body, code, param)
		}
	}
}

func TestAdminChallengeRouteRequiresJSONRequest(t *testing.T) {
	db := testDB(t)
	ts, _ := newCaptchaRouteTestApp(t, db, &fakeVerifier{})
	client := seedCaptchaEditUser(t, db, ts)

	payload := `{"mode":"slide"}`
	for _, headers := range []map[string]string{
		{"Content-Type": "text/plain"},
		{"Content-Type": "application/json", "Origin": "https://evil.example"},
	} {
		resp := postRawWithHeaders(t, client, ts.URL+adminChallengePath, payload, headers)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %v = %d, want 400", headers, resp.StatusCode)
		}
		if code, _ := decodeVerifyError(t, resp); code != "invalid_request" {
			t.Fatalf("code for %v = %q, want invalid_request", headers, code)
		}
	}
}

func TestAdminChallengeRouteRequiresPermission(t *testing.T) {
	db := testDB(t)
	ts, _ := newCaptchaRouteTestApp(t, db, &fakeVerifier{})

	userID := seedUserInGroup(t, db, uniqueTag(t), rbac.GroupIDUser)
	client := testClient(t)
	seedSession(t, client, ts, userID)

	resp := postJSON(t, client, ts.URL+adminChallengePath, map[string]any{"mode": "slide"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("plain user status = %d, want 403", resp.StatusCode)
	}
	if code, _ := decodeVerifyError(t, resp); code != "forbidden" {
		t.Fatalf("plain user code = %q, want forbidden", code)
	}

	resp = postJSON(t, &http.Client{}, ts.URL+adminChallengePath, map[string]any{"mode": "slide"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", resp.StatusCode)
	}
	if code, _ := decodeVerifyError(t, resp); code != "unauthorized" {
		t.Fatalf("anonymous code = %q, want unauthorized", code)
	}
}

func setSavedCaptcha(t *testing.T, svc *settings.Service, enabled bool, provider, mode string) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "captcha", "captcha_enabled", enabled); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "captcha_provider", provider); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "captcha_mode", mode); err != nil {
		t.Fatalf("set mode: %v", err)
	}

	t.Cleanup(func() {
		_ = svc.SetBool(context.Background(), "captcha", "captcha_enabled", false)
		_ = svc.SetString(context.Background(), "captcha", "captcha_provider", "none")
		_ = svc.SetString(context.Background(), "captcha", "captcha_mode", "slide")
	})
}
