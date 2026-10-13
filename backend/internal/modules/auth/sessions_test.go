package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"

	"website/backend/internal/common/guard"
)

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

func rewriteSessionRow(t *testing.T, sm *scs.SessionManager, token string, deadline time.Time, values map[string]any) {
	t.Helper()

	data, err := sm.Codec.Encode(deadline, values)
	if err != nil {
		t.Fatalf("encode session row: %v", err)
	}
	if err := sm.Store.Commit(token, data, deadline); err != nil {
		t.Fatalf("commit session row: %v", err)
	}
}

func setSessionCookie(t *testing.T, client *http.Client, tsURL, token string) {
	t.Helper()

	parsed, err := url.Parse(tsURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	client.Jar.SetCookies(parsed, []*http.Cookie{{Name: "session", Value: token}})
}

func TestLoginStoresSessionMetadata(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password}, map[string]string{
		"User-Agent":      "probe/1.0",
		"X-Forwarded-For": "203.0.113.7, 10.0.0.1",
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	_, _, values := sessionRow(t, sm, ts.URL, client)
	if got, _ := values[guard.SessionUserIDKey].(string); got != u.ID {
		t.Fatalf("user_id = %v, want %s", values[guard.SessionUserIDKey], u.ID)
	}
	if got, _ := values[guard.SessionUserAgentKey].(string); got != "probe/1.0" {
		t.Fatalf("user_agent = %q, want probe/1.0", got)
	}
	if got, _ := values[guard.SessionIPKey].(string); got != "203.0.113.7" {
		t.Fatalf("ip = %q, want 203.0.113.7", got)
	}
	created, ok := values[guard.SessionCreatedAtKey].(string)
	if !ok {
		t.Fatalf("created_at missing: %v", values)
	}
	if _, err := time.Parse(time.RFC3339, created); err != nil {
		t.Fatalf("created_at %q is not RFC3339: %v", created, err)
	}
	if values[guard.SessionLastSeenAtKey] != created {
		t.Fatalf("last_seen_at = %v, want %q", values[guard.SessionLastSeenAtKey], created)
	}
}

func TestLoginTruncatesOversizedHeaders(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password}, map[string]string{
		"User-Agent":      strings.Repeat("a", 2000),
		"X-Forwarded-For": strings.Repeat("b", 200),
	})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	_, _, values := sessionRow(t, sm, ts.URL, client)
	gotUA, _ := values[guard.SessionUserAgentKey].(string)
	gotIP, _ := values[guard.SessionIPKey].(string)
	if len(gotUA) != 512 {
		t.Fatalf("user_agent length = %d, want 512", len(gotUA))
	}
	if len(gotIP) != 64 {
		t.Fatalf("ip length = %d, want 64", len(gotIP))
	}
}

func TestEstablishSessionClearsPendingKeys(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	token := "pending-token-" + tag
	deadline := time.Now().Add(time.Hour).UTC()
	data, err := sm.Codec.Encode(deadline, map[string]any{
		pendingUserIDKey:   "ghost-" + tag,
		pendingAttemptsKey: 3,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := sm.Store.Commit(token, data, deadline); err != nil {
		t.Fatalf("commit: %v", err)
	}
	setSessionCookie(t, client, ts.URL, token)

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	_, _, values := sessionRow(t, sm, ts.URL, client)
	if got, _ := values[guard.SessionUserIDKey].(string); got != u.ID {
		t.Fatalf("user_id = %v, want %s", values[guard.SessionUserIDKey], u.ID)
	}
	if _, ok := values[pendingUserIDKey]; ok {
		t.Fatalf("pending user key still present: %v", values)
	}
	if _, ok := values[pendingAttemptsKey]; ok {
		t.Fatalf("pending attempts key still present: %v", values)
	}
	if got, _ := values[guard.SessionLastSeenAtKey].(string); got == "" {
		t.Fatalf("last_seen_at missing after login: %v", values)
	}
}

func TestSessionActivityTouchThrottled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	token, deadline, values := sessionRow(t, sm, ts.URL, client)
	seeded, ok := values[guard.SessionLastSeenAtKey].(string)
	if !ok || seeded == "" {
		t.Fatalf("login did not write last_seen_at: %v", values)
	}

	me := doGET(t, client, ts.URL+"/api/v1/auth/me")
	me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.StatusCode)
	}
	_, _, afterFresh := sessionRow(t, sm, ts.URL, client)
	if afterFresh[guard.SessionLastSeenAtKey] != seeded {
		t.Fatalf("last_seen_at = %v, want untouched %q", afterFresh[guard.SessionLastSeenAtKey], seeded)
	}

	values[guard.SessionLastSeenAtKey] = time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339)
	rewriteSessionRow(t, sm, token, deadline, values)

	meStale := doGET(t, client, ts.URL+"/api/v1/auth/me")
	meStale.Body.Close()
	_, _, afterStale := sessionRow(t, sm, ts.URL, client)
	got, _ := afterStale[guard.SessionLastSeenAtKey].(string)
	parsed, err := time.Parse(time.RFC3339, got)
	if err != nil {
		t.Fatalf("last_seen_at %q is not RFC3339: %v", got, err)
	}
	if time.Since(parsed) > time.Minute {
		t.Fatalf("last_seen_at = %q, want refreshed within a minute of now", got)
	}
}

func fetchSessions(t *testing.T, client *http.Client, tsURL string) (int, []SessionResponse) {
	t.Helper()

	resp := doGET(t, client, tsURL+"/api/v1/users/me/sessions")
	defer resp.Body.Close()

	var body struct {
		Items []SessionResponse `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, body.Items
}

func TestSessionsListIncludesCurrentWithMetadata(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _ := newTestAppWithSessions(t, db)
	client := testClient(t)

	login := postJSONWithHeaders(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password}, map[string]string{
		"User-Agent":      "probe/1.0",
		"X-Forwarded-For": "203.0.113.7",
	})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	status, items := fetchSessions(t, client, ts.URL)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	item := items[0]
	if !item.Current {
		t.Fatal("current = false, want true for the requesting session")
	}
	if item.UserAgent != "probe/1.0" {
		t.Fatalf("user_agent = %q, want probe/1.0", item.UserAgent)
	}
	if item.IP != "203.0.113.7" {
		t.Fatalf("ip = %q, want 203.0.113.7", item.IP)
	}
	if item.ID == "" || item.ID == sessionCookie(t, client.Jar, ts.URL) {
		t.Fatalf("id = %q, want an opaque fingerprint different from the session token", item.ID)
	}
	for name, value := range map[string]string{"created_at": item.CreatedAt, "last_seen_at": item.LastSeenAt, "expires_at": item.ExpiresAt} {
		if _, err := time.Parse(time.RFC3339, value); err != nil {
			t.Fatalf("%s = %q is not RFC3339: %v", name, value, err)
		}
	}
}

func TestSessionsListIsolatedPerUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	first, firstPassword := createTestUser(t, db, tag+"-a")
	second, secondPassword := createTestUser(t, db, tag+"-b")
	ts, _ := newTestAppWithSessions(t, db)

	firstClient := testClient(t)
	firstLogin := postJSON(t, firstClient, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: first.Email, Password: firstPassword})
	firstLogin.Body.Close()
	if firstLogin.StatusCode != http.StatusOK {
		t.Fatalf("first login status = %d, want 200", firstLogin.StatusCode)
	}
	secondClient := testClient(t)
	secondLogin := postJSON(t, secondClient, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: second.Email, Password: secondPassword})
	secondLogin.Body.Close()
	if secondLogin.StatusCode != http.StatusOK {
		t.Fatalf("second login status = %d, want 200", secondLogin.StatusCode)
	}

	_, firstItems := fetchSessions(t, firstClient, ts.URL)
	_, secondItems := fetchSessions(t, secondClient, ts.URL)
	if len(firstItems) != 1 || len(secondItems) != 1 {
		t.Fatalf("items = %d/%d, want 1/1", len(firstItems), len(secondItems))
	}
	if firstItems[0].ID == secondItems[0].ID {
		t.Fatalf("both users see the same session id %q", firstItems[0].ID)
	}
}

func TestSessionsListExcludesPendingSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	deadline := time.Now().Add(time.Hour).UTC()
	data, err := sm.Codec.Encode(deadline, map[string]any{pendingUserIDKey: u.ID})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := sm.Store.Commit("pending-row-"+tag, data, deadline); err != nil {
		t.Fatalf("commit: %v", err)
	}

	_, items := fetchSessions(t, client, ts.URL)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1 (pending sessions must not be listed)", len(items))
	}
	if !items[0].Current {
		t.Fatal("the single listed item should be the current session")
	}
}

func TestSessionsListRequiresAuth(t *testing.T) {
	db := testDB(t)
	ts := newTestApp(t, db)

	resp := doGET(t, testClient(t), ts.URL+"/api/v1/users/me/sessions")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", code)
	}
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
	resp := postJSON(t, client, tsURL+"/api/v1/auth/login", LoginRequest{Identifier: email, Password: password})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	return client
}

func TestSessionsRevokeAnotherDevice(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _ := newTestAppWithSessions(t, db)

	keeper := loginAs(t, ts.URL, u.Email, password)
	doomed := loginAs(t, ts.URL, u.Email, password)

	_, items := fetchSessions(t, keeper, ts.URL)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	doomedID := ""
	for _, item := range items {
		if !item.Current {
			doomedID = item.ID
		}
	}
	if doomedID == "" {
		t.Fatal("no non-current session found")
	}

	resp := doDELETE(t, keeper, ts.URL+"/api/v1/users/me/sessions/"+doomedID)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != doomedID || !body.Deleted {
		t.Fatalf("body = %+v, want id %q with deleted true", body, doomedID)
	}

	revoked := doGET(t, doomed, ts.URL+"/api/v1/auth/me")
	revoked.Body.Close()
	if revoked.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked session status = %d, want 401", revoked.StatusCode)
	}

	stillAlive := doGET(t, keeper, ts.URL+"/api/v1/auth/me")
	stillAlive.Body.Close()
	if stillAlive.StatusCode != http.StatusOK {
		t.Fatalf("keeper status = %d, want 200", stillAlive.StatusCode)
	}
	_, after := fetchSessions(t, keeper, ts.URL)
	if len(after) != 1 {
		t.Fatalf("items = %d, want 1 after revoke", len(after))
	}
}

func TestSessionsRevokeUnknownOrForeignReturnsNotFound(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	owner, ownerPassword := createTestUser(t, db, tag+"-owner")
	other, otherPassword := createTestUser(t, db, tag+"-other")
	ts, _ := newTestAppWithSessions(t, db)

	ownerClient := loginAs(t, ts.URL, owner.Email, ownerPassword)
	otherClient := loginAs(t, ts.URL, other.Email, otherPassword)

	_, otherItems := fetchSessions(t, otherClient, ts.URL)
	if len(otherItems) != 1 {
		t.Fatalf("other items = %d, want 1", len(otherItems))
	}
	foreignID := otherItems[0].ID

	foreign := doDELETE(t, ownerClient, ts.URL+"/api/v1/users/me/sessions/"+foreignID)
	defer foreign.Body.Close()
	if foreign.StatusCode != http.StatusNotFound {
		t.Fatalf("foreign status = %d, want 404", foreign.StatusCode)
	}
	if code := decodeError(t, foreign); code != "not_found" {
		t.Fatalf("foreign error = %q, want not_found", code)
	}

	unknown := doDELETE(t, ownerClient, ts.URL+"/api/v1/users/me/sessions/nope-"+tag)
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown status = %d, want 404", unknown.StatusCode)
	}

	otherAlive := doGET(t, otherClient, ts.URL+"/api/v1/auth/me")
	otherAlive.Body.Close()
	if otherAlive.StatusCode != http.StatusOK {
		t.Fatalf("targeted session status = %d, want 200", otherAlive.StatusCode)
	}

	loginAs(t, ts.URL, owner.Email, ownerPassword)
	_, ownerItems := fetchSessions(t, ownerClient, ts.URL)
	if len(ownerItems) != 2 {
		t.Fatalf("owner items = %d, want 2", len(ownerItems))
	}
	spareID := ""
	for _, item := range ownerItems {
		if !item.Current {
			spareID = item.ID
		}
	}

	first := doDELETE(t, ownerClient, ts.URL+"/api/v1/users/me/sessions/"+spareID)
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first revoke status = %d, want 200", first.StatusCode)
	}
	again := doDELETE(t, ownerClient, ts.URL+"/api/v1/users/me/sessions/"+spareID)
	again.Body.Close()
	if again.StatusCode != http.StatusNotFound {
		t.Fatalf("second revoke status = %d, want 404", again.StatusCode)
	}
}

func TestSessionsRevokeCurrentEndsSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)

	client := loginAs(t, ts.URL, u.Email, password)

	token, deadline, values := sessionRow(t, sm, ts.URL, client)
	values[guard.SessionLastSeenAtKey] = time.Now().UTC().Add(-6 * time.Minute).Format(time.RFC3339)
	rewriteSessionRow(t, sm, token, deadline, values)

	_, items := fetchSessions(t, client, ts.URL)
	if len(items) != 1 || !items[0].Current {
		t.Fatalf("items = %+v, want exactly the current session", items)
	}

	resp := doDELETE(t, client, ts.URL+"/api/v1/users/me/sessions/"+items[0].ID)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	after := doGET(t, client, ts.URL+"/api/v1/auth/me")
	defer after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 after revoking the current session", after.StatusCode)
	}
}

func TestSessionsRevokeRequiresAuth(t *testing.T) {
	db := testDB(t)
	ts := newTestApp(t, db)

	resp := doDELETE(t, testClient(t), ts.URL+"/api/v1/users/me/sessions/whatever")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", code)
	}
}
