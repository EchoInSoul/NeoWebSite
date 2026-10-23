package guard

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
)

func newActivityTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/seed", func(c *gin.Context) {
		ctx := c.Request.Context()
		if c.Query("anonymous") != "1" {
			sm.Put(ctx, SessionUserIDKey, "u1")
		}
		sm.Put(ctx, SessionLastSeenAtKey, c.Query("last_seen"))
		c.Status(http.StatusNoContent)
	})
	r.GET("/probe", TrackSessionActivity(sm), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"last_seen": sm.GetString(c.Request.Context(), SessionLastSeenAtKey)})
	})

	ts := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(ts.Close)
	return ts
}

func activityClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func seedActivitySession(t *testing.T, client *http.Client, tsURL string, query url.Values) {
	t.Helper()

	resp, err := client.Get(tsURL + "/seed?" + query.Encode())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp.Body.Close()
}

func probeActivity(t *testing.T, client *http.Client, tsURL string) string {
	t.Helper()

	resp, err := client.Get(tsURL + "/probe")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		LastSeen string `json:"last_seen"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.LastSeen
}

func TestTrackSessionActivityWritesWhenStale(t *testing.T) {
	ts := newActivityTestServer(t)
	client := activityClient(t)
	stale := time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339)
	seedActivitySession(t, client, ts.URL, url.Values{"last_seen": {stale}})

	got := probeActivity(t, client, ts.URL)
	if got == "" || got == stale {
		t.Fatalf("last_seen = %q, want a fresh timestamp", got)
	}
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("last_seen %q is not RFC3339: %v", got, err)
	}
	if time.Since(parsed) > time.Minute {
		t.Fatalf("last_seen = %q, want within a minute of now", got)
	}
}

func TestTrackSessionActivitySkipsWhenFresh(t *testing.T) {
	ts := newActivityTestServer(t)
	client := activityClient(t)
	fresh := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	seedActivitySession(t, client, ts.URL, url.Values{"last_seen": {fresh}})

	if got := probeActivity(t, client, ts.URL); got != fresh {
		t.Fatalf("last_seen = %q, want untouched %q", got, fresh)
	}
}

func TestTrackSessionActivityRejectsMissingTimestamp(t *testing.T) {
	ts := newActivityTestServer(t)
	client := activityClient(t)
	seedActivitySession(t, client, ts.URL, url.Values{"last_seen": {""}})

	resp, err := client.Get(ts.URL + "/probe")
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "unauthorized" {
		t.Fatalf("code = %q, want unauthorized", body.Error.Code)
	}

	cleared := false
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session" && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("session cookie was not cleared")
	}
}

func TestTrackSessionActivityIgnoresAnonymous(t *testing.T) {
	ts := newActivityTestServer(t)
	client := activityClient(t)
	stale := time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339)
	seedActivitySession(t, client, ts.URL, url.Values{"anonymous": {"1"}, "last_seen": {stale}})

	if got := probeActivity(t, client, ts.URL); got != stale {
		t.Fatalf("last_seen = %q, want untouched %q for an anonymous session", got, stale)
	}
}
