package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	captchaProviderTurnstile = "turnstile"

	turnstileSiteverifyURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	turnstileTimeout       = 2500 * time.Millisecond
)

type captchaOutcome int

const (
	captchaPassed captchaOutcome = iota
	captchaRejected
	captchaUnavailable
)

type captchaVerifier interface {
	Verify(ctx context.Context, secret, token, remoteIP string) captchaOutcome
}

type turnstileVerifier struct {
	client   *http.Client
	endpoint string
}

func newTurnstileVerifier(endpoint string, timeout time.Duration) *turnstileVerifier {
	return &turnstileVerifier{
		client:   &http.Client{Timeout: timeout},
		endpoint: endpoint,
	}
}

type turnstileSiteverifyResponse struct {
	Success    bool     `json:"success"`
	ErrorCodes []string `json:"error-codes"`
}

var turnstileRejectedErrorCodes = map[string]bool{
	"missing-input-response": true,
	"invalid-input-response": true,
	"timeout-or-duplicate":   true,
	"invalid-input-secret":   true,
}

func (v *turnstileVerifier) Verify(ctx context.Context, secret, token, remoteIP string) captchaOutcome {
	form := url.Values{"secret": {secret}, "response": {token}}
	if remoteIP != "" && remoteIP != "unknown" {
		form.Set("remoteip", remoteIP)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, v.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return captchaUnavailable
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := v.client.Do(request)
	if err != nil {
		return captchaUnavailable
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return captchaUnavailable
	}

	var payload turnstileSiteverifyResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return captchaUnavailable
	}
	if payload.Success {
		return captchaPassed
	}
	for _, code := range payload.ErrorCodes {
		if turnstileRejectedErrorCodes[code] {
			return captchaRejected
		}
	}

	return captchaUnavailable
}

type unavailableCaptchaVerifier struct{}

func (unavailableCaptchaVerifier) Verify(context.Context, string, string, string) captchaOutcome {
	return captchaUnavailable
}
