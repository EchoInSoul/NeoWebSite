package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestTurnstileVerifierClassifiesResponses(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   captchaOutcome
	}{
		{name: "success", status: http.StatusOK, body: `{"success":true}`, want: captchaPassed},
		{name: "missing response", status: http.StatusOK, body: `{"success":false,"error-codes":["missing-input-response"]}`, want: captchaRejected},
		{name: "invalid response", status: http.StatusOK, body: `{"success":false,"error-codes":["invalid-input-response"]}`, want: captchaRejected},
		{name: "duplicate token", status: http.StatusOK, body: `{"success":false,"error-codes":["timeout-or-duplicate"]}`, want: captchaRejected},
		{name: "invalid secret", status: http.StatusOK, body: `{"success":false,"error-codes":["invalid-input-secret"]}`, want: captchaUnavailable},
		{name: "no error codes", status: http.StatusOK, body: `{"success":false}`, want: captchaUnavailable},
		{name: "server error", status: http.StatusInternalServerError, body: `{"success":false,"error-codes":["internal-error"]}`, want: captchaUnavailable},
		{name: "unparseable body", status: http.StatusOK, body: `not-json`, want: captchaUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			t.Cleanup(server.Close)

			verifier := newTurnstileVerifier(server.URL, time.Second)
			if got := verifier.Verify(context.Background(), "secret", "token", "203.0.113.9"); got != tc.want {
				t.Fatalf("outcome = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestTurnstileVerifierSendsExpectedForm(t *testing.T) {
	var received url.Values
	var contentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		received = r.PostForm
		_, _ = io.WriteString(w, `{"success":true}`)
	}))
	t.Cleanup(server.Close)

	verifier := newTurnstileVerifier(server.URL, time.Second)
	if got := verifier.Verify(context.Background(), "the-secret", "the-token", "203.0.113.9"); got != captchaPassed {
		t.Fatalf("outcome = %d, want %d", got, captchaPassed)
	}

	if contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("content type = %q, want form encoding", contentType)
	}
	if received.Get("secret") != "the-secret" {
		t.Fatalf("secret = %q, want the configured secret", received.Get("secret"))
	}
	if received.Get("response") != "the-token" {
		t.Fatalf("response = %q, want the visitor token", received.Get("response"))
	}
	if received.Get("remoteip") != "203.0.113.9" {
		t.Fatalf("remoteip = %q, want the visitor address", received.Get("remoteip"))
	}
}

func TestTurnstileVerifierOmitsUnknownRemoteIP(t *testing.T) {
	for _, remoteIP := range []string{"", "unknown"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if _, ok := r.PostForm["remoteip"]; ok {
				t.Errorf("remoteip present for %q, want it omitted", remoteIP)
			}
			_, _ = io.WriteString(w, `{"success":true}`)
		}))

		verifier := newTurnstileVerifier(server.URL, time.Second)
		if got := verifier.Verify(context.Background(), "secret", "token", remoteIP); got != captchaPassed {
			t.Fatalf("remoteip %q: outcome = %d, want %d", remoteIP, got, captchaPassed)
		}
		server.Close()
	}
}

func TestTurnstileVerifierReportsTimeoutAsUnavailable(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	verifier := newTurnstileVerifier(server.URL, 50*time.Millisecond)
	if got := verifier.Verify(context.Background(), "secret", "token", "203.0.113.9"); got != captchaUnavailable {
		t.Fatalf("outcome = %d, want %d", got, captchaUnavailable)
	}
}
