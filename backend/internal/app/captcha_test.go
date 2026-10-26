package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"website/backend/internal/config"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/settings"
)

func TestBuiltinCaptchaLoginFlow(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	ctx := context.Background()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	settingsSvc := settingsModule.Service
	if err := settingsSvc.SetBool(ctx, "captcha", "captcha_enabled", true); err != nil {
		t.Fatalf("enable captcha: %v", err)
	}
	if err := settingsSvc.SetString(ctx, "captcha", "captcha_provider", "builtin"); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	t.Cleanup(func() {
		_ = settingsSvc.SetBool(context.Background(), "captcha", "captcha_enabled", false)
		_ = settingsSvc.SetString(context.Background(), "captcha", "captcha_provider", "none")
	})

	user, password := createTestUser(t, db)
	client := newClient(t)

	createBody, err := json.Marshal(map[string]string{"scope": "login"})
	if err != nil {
		t.Fatalf("marshal create: %v", err)
	}
	response, err := client.Post(ts.URL+"/api/v1/captcha/challenges", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", response.StatusCode)
	}
	var challenge struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	response.Body.Close()

	challengeID := uuid.MustParse(challenge.ID)
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(challengeID).Exec(context.Background()) })

	row, err := db.Client.CaptchaChallenge.Get(ctx, challengeID)
	if err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	var payload builtin.AnswerDots
	if err := json.Unmarshal(row.AnswerDots, &payload); err != nil {
		t.Fatalf("decode answer dots: %v", err)
	}
	dots := payload.Dots

	solveBody, err := json.Marshal(map[string]int{"x": dots[0].X, "y": dots[0].Y})
	if err != nil {
		t.Fatalf("marshal solve: %v", err)
	}
	response, err = client.Post(ts.URL+"/api/v1/captcha/challenges/"+challenge.ID+"/solutions", "application/json", bytes.NewReader(solveBody))
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("solve status = %d, want 200", response.StatusCode)
	}
	var solved struct {
		CaptchaToken string `json:"captcha_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&solved); err != nil {
		t.Fatalf("decode solve: %v", err)
	}
	response.Body.Close()
	if solved.CaptchaToken == "" {
		t.Fatal("captcha_token is empty")
	}

	loginBody, err := json.Marshal(map[string]string{
		"identifier":    user.Email,
		"password":      password,
		"captcha_token": solved.CaptchaToken,
	})
	if err != nil {
		t.Fatalf("marshal login: %v", err)
	}
	response, err = client.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", response.StatusCode)
	}
	response.Body.Close()

	response, err = client.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("replay login: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("replay status = %d, want 400", response.StatusCode)
	}
	var envelope errorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode replay error: %v", err)
	}
	if envelope.Error.Code != "captcha_failed" {
		t.Fatalf("replay code = %q, want captcha_failed", envelope.Error.Code)
	}
}

func TestBuiltinClickCaptchaLoginFlow(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	ctx := context.Background()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	settingsSvc := settingsModule.Service
	if err := settingsSvc.SetBool(ctx, "captcha", "captcha_enabled", true); err != nil {
		t.Fatalf("enable captcha: %v", err)
	}
	if err := settingsSvc.SetString(ctx, "captcha", "captcha_provider", "builtin"); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if err := settingsSvc.SetString(ctx, "captcha", "captcha_mode", "click"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	t.Cleanup(func() {
		_ = settingsSvc.SetBool(context.Background(), "captcha", "captcha_enabled", false)
		_ = settingsSvc.SetString(context.Background(), "captcha", "captcha_provider", "none")
		_ = settingsSvc.SetString(context.Background(), "captcha", "captcha_mode", "slide")
	})

	user, password := createTestUser(t, db)
	client := newClient(t)

	createBody, err := json.Marshal(map[string]string{"scope": "login"})
	if err != nil {
		t.Fatalf("marshal create: %v", err)
	}
	response, err := client.Post(ts.URL+"/api/v1/captcha/challenges", "application/json", bytes.NewReader(createBody))
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create status = %d, want 201", response.StatusCode)
	}
	var challenge struct {
		ID             string `json:"id"`
		PromptImage    string `json:"prompt_image"`
		PromptText     string `json:"prompt_text"`
		ExpectedClicks int    `json:"expected_clicks"`
		TileImage      string `json:"tile_image"`
	}
	if err := json.NewDecoder(response.Body).Decode(&challenge); err != nil {
		t.Fatalf("decode challenge: %v", err)
	}
	response.Body.Close()
	if challenge.PromptImage == "" || challenge.PromptText == "" {
		t.Fatalf("click challenge missing prompt fields: %+v", challenge)
	}
	if challenge.ExpectedClicks != 3 {
		t.Fatalf("expected clicks = %d, want 3", challenge.ExpectedClicks)
	}
	if challenge.TileImage != "" {
		t.Fatal("tile image present in click challenge")
	}

	challengeID := uuid.MustParse(challenge.ID)
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(challengeID).Exec(context.Background()) })

	row, err := db.Client.CaptchaChallenge.Get(ctx, challengeID)
	if err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	var payload builtin.AnswerDots
	if err := json.Unmarshal(row.AnswerDots, &payload); err != nil {
		t.Fatalf("decode answer dots: %v", err)
	}
	if len(payload.Dots) != 3 {
		t.Fatalf("stored dots = %d, want 3", len(payload.Dots))
	}

	solveBody, err := json.Marshal(map[string]any{"clicks": []map[string]int{
		{"x": payload.Dots[0].X, "y": payload.Dots[0].Y},
		{"x": payload.Dots[1].X, "y": payload.Dots[1].Y},
		{"x": payload.Dots[2].X, "y": payload.Dots[2].Y},
	}})
	if err != nil {
		t.Fatalf("marshal solve: %v", err)
	}
	response, err = client.Post(ts.URL+"/api/v1/captcha/challenges/"+challenge.ID+"/solutions", "application/json", bytes.NewReader(solveBody))
	if err != nil {
		t.Fatalf("solve: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("solve status = %d, want 200", response.StatusCode)
	}
	var solved struct {
		CaptchaToken string `json:"captcha_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&solved); err != nil {
		t.Fatalf("decode solve: %v", err)
	}
	response.Body.Close()
	if solved.CaptchaToken == "" {
		t.Fatal("captcha_token is empty")
	}

	loginBody, err := json.Marshal(map[string]string{
		"identifier":    user.Email,
		"password":      password,
		"captcha_token": solved.CaptchaToken,
	})
	if err != nil {
		t.Fatalf("marshal login: %v", err)
	}
	response, err = client.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", response.StatusCode)
	}
	response.Body.Close()

	response, err = client.Post(ts.URL+"/api/v1/sessions", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("replay login: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("replay status = %d, want 400", response.StatusCode)
	}
	var envelope errorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode replay error: %v", err)
	}
	if envelope.Error.Code != "captcha_failed" {
		t.Fatalf("replay code = %q, want captcha_failed", envelope.Error.Code)
	}
}

func TestBuiltinCaptchaChallengeFailsWhenDatabaseUnavailable(t *testing.T) {
	ts := newDegradedServer(t)

	body, err := json.Marshal(map[string]string{"scope": "login"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	response, err := http.Post(ts.URL+"/api/v1/captcha/challenges", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
	var envelope errorEnvelope
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if envelope.Error.Code != "service_unavailable" {
		t.Fatalf("code = %q, want service_unavailable", envelope.Error.Code)
	}
}
