package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/settingvalue"
	"website/backend/internal/modules/access"
)

func newSettingsTestApp(t *testing.T, db *database.Database) *httptest.Server {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/seed", func(c *gin.Context) {
		sm.Put(c.Request.Context(), guard.SessionUserIDKey, c.Query("user"))
		c.Status(http.StatusNoContent)
	})
	api := engine.Group("/api/v1")

	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts
}

func seedClient(t *testing.T, ts *httptest.Server, userID uuid.UUID) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp, err := client.Get(ts.URL + "/seed?user=" + userID.String())
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp.Body.Close()
	return client
}

func doJSON(t *testing.T, client *http.Client, method, endpoint string, body any) *http.Response {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	return resp
}

func doRaw(t *testing.T, client *http.Client, method, endpoint, payload string) *http.Response {
	t.Helper()

	request, err := http.NewRequest(method, endpoint, bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	return resp
}

func decodeError(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want nested object with code", body["error"])
	}
	return errObj
}

func decodeErrorCode(t *testing.T, resp *http.Response) string {
	t.Helper()

	code, _ := decodeError(t, resp)["code"].(string)
	return code
}

func decodeGroup(t *testing.T, resp *http.Response) (string, map[string]any) {
	t.Helper()
	defer resp.Body.Close()

	var view struct {
		Group  string         `json:"group"`
		Values map[string]any `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		t.Fatalf("decode group view: %v", err)
	}
	return view.Group, view.Values
}

func decodePublicItems(t *testing.T, resp *http.Response) []map[string]any {
	t.Helper()
	defer resp.Body.Close()

	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode public items: %v", err)
	}
	return body.Items
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()

	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
}

func registerRowCount(t *testing.T, db *database.Database) int {
	t.Helper()

	count, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("register")).
		Count(context.Background())
	if err != nil {
		t.Fatalf("count register rows: %v", err)
	}
	return count
}

func createUserInGroup(t *testing.T, db *database.Database, groupID uuid.UUID) uuid.UUID {
	t.Helper()

	tag := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	created, err := db.Client.User.Create().
		SetEmail("u+" + tag + "@test.local").
		SetNickname("nick" + tag).
		SetPasswordHash("hash-" + tag).
		SetGroupID(groupID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(created.ID).Exec(context.Background())
	})
	return created.ID
}

func TestSettingsGetAsOwner(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	ownerID := createUserInGroup(t, db, rbac.GroupIDOwner)

	resp := doJSON(t, seedClient(t, ts, ownerID), http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "register" {
		t.Fatalf("group = %q, want register", group)
	}
	if values["enabled"] != true {
		t.Fatalf("register.enabled = %v, want true merged from default", values["enabled"])
	}
	if count := registerRowCount(t, db); count != 0 {
		t.Fatalf("register rows = %d, want 0 before any write", count)
	}
}

func TestSettingsGetUnknownGroup(t *testing.T) {
	db := testDB(t)
	ts := newSettingsTestApp(t, db)
	ownerID := createUserInGroup(t, db, rbac.GroupIDOwner)

	resp := doJSON(t, seedClient(t, ts, ownerID), http.MethodGet, ts.URL+"/api/v1/admin/settings/nope", nil)
	expectStatus(t, resp, http.StatusNotFound)
	if code := decodeErrorCode(t, resp); code != "not_found" {
		t.Fatalf("code = %q, want not_found", code)
	}
}

func TestSettingsGetRequiresAuth(t *testing.T) {
	db := testDB(t)
	ts := newSettingsTestApp(t, db)

	resp := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusUnauthorized)
	if code := decodeErrorCode(t, resp); code != "unauthorized" {
		t.Fatalf("code = %q, want unauthorized", code)
	}
}

func TestSettingsGetForbiddenForPlainUser(t *testing.T) {
	db := testDB(t)
	ts := newSettingsTestApp(t, db)
	userID := createUserInGroup(t, db, rbac.GroupIDUser)

	resp := doJSON(t, seedClient(t, ts, userID), http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusForbidden)
	if code := decodeErrorCode(t, resp); code != "forbidden" {
		t.Fatalf("code = %q, want forbidden", code)
	}
}

func TestSettingsPublicGetAnonymously(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register", "site")
	ts := newSettingsTestApp(t, db)

	resp := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/public/settings", nil)
	expectStatus(t, resp, http.StatusOK)

	items := decodePublicItems(t, resp)
	if len(items) != 2 {
		t.Fatalf("public items = %v, want the site and register groups", items)
	}
	if items[0]["group"] != "site" {
		t.Fatalf("group = %v, want site first in registry order", items[0]["group"])
	}
	siteValues, ok := items[0]["values"].(map[string]any)
	if !ok {
		t.Fatalf("site values = %v, want an object", items[0]["values"])
	}
	if len(siteValues) != 5 {
		t.Fatalf("site values = %v, want exactly the five defaults", items[0]["values"])
	}
	for _, name := range []string{"title", "subtitle", "description", "keywords", "url"} {
		if siteValues[name] != "" {
			t.Fatalf("site.%s = %v, want the empty default", name, siteValues[name])
		}
	}
	if items[1]["group"] != "register" {
		t.Fatalf("group = %v, want register", items[1]["group"])
	}
	values, ok := items[1]["values"].(map[string]any)
	if !ok || values["enabled"] != true {
		t.Fatalf("values = %v, want enabled true merged from default", items[1]["values"])
	}
}

func TestSettingsPatchSiteTitleReadsBackPublicly(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register", "site")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/site", map[string]any{"title": "我的博客"})
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "site" {
		t.Fatalf("group = %q, want site", group)
	}
	if values["title"] != "我的博客" {
		t.Fatalf("site.title = %v, want the written title", values["title"])
	}
	if values["subtitle"] != "" {
		t.Fatalf("site.subtitle = %v, want the untouched empty default", values["subtitle"])
	}

	public := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/public/settings", nil)
	expectStatus(t, public, http.StatusOK)
	items := decodePublicItems(t, public)
	if len(items) != 2 || items[0]["group"] != "site" {
		t.Fatalf("public items = %v, want site first then register", items)
	}
	siteValues, ok := items[0]["values"].(map[string]any)
	if !ok || siteValues["title"] != "我的博客" {
		t.Fatalf("public site values = %v, want the written title", items[0]["values"])
	}
}

func TestSettingsPatchTurnsRegistrationOff(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "register" || values["enabled"] != false {
		t.Fatalf("patch response = %q %v, want register with enabled false", group, values)
	}

	after := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, after, http.StatusOK)
	group, values = decodeGroup(t, after)
	if group != "register" || values["enabled"] != false {
		t.Fatalf("get after patch = %q %v, want register with enabled false", group, values)
	}
	if count := registerRowCount(t, db); count != 1 {
		t.Fatalf("register rows = %d, want the single explicit write", count)
	}
}

func TestSettingsPatchUnknownItem(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false, "nope": 1})
	expectStatus(t, resp, http.StatusBadRequest)
	errObj := decodeError(t, resp)
	if errObj["code"] != "invalid_request" {
		t.Fatalf("code = %v, want invalid_request", errObj["code"])
	}
	if errObj["param"] != "nope" {
		t.Fatalf("param = %v, want nope", errObj["param"])
	}
	if count := registerRowCount(t, db); count != 0 {
		t.Fatalf("register rows = %d, want 0 — the legal item must not be half-written", count)
	}
}

func TestSettingsPatchTypeMismatch(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": "no"})
	expectStatus(t, resp, http.StatusBadRequest)
	errObj := decodeError(t, resp)
	if errObj["code"] != "invalid_request" || errObj["param"] != "enabled" {
		t.Fatalf("error = %v, want invalid_request with param enabled", errObj)
	}
	if count := registerRowCount(t, db); count != 0 {
		t.Fatalf("register rows = %d, want 0 after rejected write", count)
	}
}

func TestSettingsPatchEmptyBody(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{})
	expectStatus(t, resp, http.StatusBadRequest)
	errObj := decodeError(t, resp)
	if errObj["code"] != "invalid_request" {
		t.Fatalf("code = %v, want invalid_request", errObj["code"])
	}
	if errObj["param"] != nil {
		t.Fatalf("param = %v, want absent for a zero-item patch", errObj["param"])
	}
}

func TestSettingsPatchMalformedBody(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	for _, payload := range []string{"{not json", `["enabled"]`, `"enabled"`, ""} {
		resp := doRaw(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", payload)
		expectStatus(t, resp, http.StatusBadRequest)
		if code := decodeErrorCode(t, resp); code != "invalid_request" {
			t.Fatalf("payload %q: code = %q, want invalid_request", payload, code)
		}
	}
	if count := registerRowCount(t, db); count != 0 {
		t.Fatalf("register rows = %d, want 0 after malformed bodies", count)
	}
}

func TestSettingsPatchUnknownGroup(t *testing.T) {
	db := testDB(t)
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/nope", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusNotFound)
	if code := decodeErrorCode(t, resp); code != "not_found" {
		t.Fatalf("code = %q, want not_found", code)
	}
}

func TestSettingsPatchNeedsEditPermission(t *testing.T) {
	db := testDB(t)
	resetGroups(t, db, "register")
	ts := newSettingsTestApp(t, db)
	plain := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDUser))
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	denied := doJSON(t, plain, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, denied, http.StatusForbidden)
	if code := decodeErrorCode(t, denied); code != "forbidden" {
		t.Fatalf("code = %q, want forbidden", code)
	}
	if count := registerRowCount(t, db); count != 0 {
		t.Fatalf("register rows = %d, want 0 after forbidden patch", count)
	}

	allowed := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, allowed, http.StatusOK)
	allowed.Body.Close()
}
