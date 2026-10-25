package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"website/backend/internal/modules/mail"
)

type fakeMailer struct {
	mu            sync.Mutex
	configured    bool
	configuredErr error
	sendSyncErr   error
	messages      []mail.Message
}

func (f *fakeMailer) Configured(context.Context) (bool, error) {
	return f.configured, f.configuredErr
}

func (f *fakeMailer) Send(context.Context, mail.Message) error {
	return nil
}

func (f *fakeMailer) SendSync(_ context.Context, msg mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendSyncErr != nil {
		return f.sendSyncErr
	}
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeMailer) sent() []mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mail.Message(nil), f.messages...)
}

func (f *fakeMailer) Close(context.Context) error {
	return nil
}

func postJSONWithHeaders(t *testing.T, client *http.Client, endpoint string, body any, headers map[string]string) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", endpoint, err)
	}
	return resp
}

func doGET(t *testing.T, client *http.Client, endpoint string) *http.Response {
	t.Helper()

	resp, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("get %s: %v", endpoint, err)
	}
	return resp
}

func doDELETE(t *testing.T, client *http.Client, endpoint string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodDelete, endpoint, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("delete %s: %v", endpoint, err)
	}
	return resp
}

func loginAs(t *testing.T, tsURL, email, password string) *http.Client {
	t.Helper()

	client := testClient(t)
	resp := postJSON(t, client, tsURL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: password})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	return client
}

func sessionRow(t *testing.T, sm *scs.SessionManager, tsURL string, client *http.Client) (string, time.Time, map[string]any) {
	t.Helper()

	token := sessionCookie(t, client.Jar, tsURL)
	if token == "" {
		t.Fatal("session cookie missing")
	}
	data, exists, err := sm.Store.Find(token)
	if err != nil {
		t.Fatalf("find session row: %v", err)
	}
	if !exists {
		t.Fatalf("session row for token %q not found", token)
	}
	deadline, values, err := sm.Codec.Decode(data)
	if err != nil {
		t.Fatalf("decode session row: %v", err)
	}
	return token, deadline, values
}
