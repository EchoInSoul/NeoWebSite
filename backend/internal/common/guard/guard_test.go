package guard

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/seed", func(c *gin.Context) {
		sm.Put(c.Request.Context(), SessionUserIDKey, "u1")
		c.Status(http.StatusNoContent)
	})
	r.GET("/protected", RequireAuth(sm), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user": c.GetString(SessionUserIDKey)})
	})

	ts := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(ts.Close)
	return ts
}

func TestRequireAuthRejectsAnonymous(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/protected")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", body["error"])
	}
}

func TestRequireAuthAllowsSessionUser(t *testing.T) {
	ts := newTestServer(t)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	seed, err := client.Get(ts.URL + "/seed")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	seed.Body.Close()

	resp, err := client.Get(ts.URL + "/protected")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["user"] != "u1" {
		t.Fatalf("user = %q, want u1", body["user"])
	}
}
