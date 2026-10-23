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

func assertRegisterEnabled(t *testing.T, db *database.Database, want bool) {
	t.Helper()

	row, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("register"), settingvalue.NameEQ("enabled")).
		Only(context.Background())
	if err != nil {
		t.Fatalf("query register.enabled: %v", err)
	}
	wantValue := "false"
	if want {
		wantValue = "true"
	}
	if string(row.Value) != wantValue {
		t.Fatalf("register.enabled = %s, want %s", row.Value, wantValue)
	}
}

func builtinGroup(name string) (Group, bool) {
	for _, group := range builtinRegistry {
		if group.Name == name {
			return group, true
		}
	}
	return Group{}, false
}

func resetBuiltin(t *testing.T, db *database.Database, names ...string) {
	t.Helper()

	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	groups := make([]Group, 0, len(names))
	for _, name := range names {
		group, ok := builtinGroup(name)
		if !ok {
			t.Fatalf("unknown builtin group %q", name)
		}
		groups = append(groups, group)
	}

	deleteGroups(t, db, names...)
	seedGroupDefaults(t, module.Service, groups)
	t.Cleanup(func() {
		deleteGroups(t, db, names...)
		if err := writeGroupDefaults(module.Service, groups); err != nil {
			t.Errorf("reseed %v: %v", names, err)
		}
	})
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

func createGroupWithPermissions(t *testing.T, db *database.Database, label string, permissions []string) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	tag := fmt.Sprintf("%s-%d-%d", label, os.Getpid(), time.Now().UnixNano())
	created, err := db.Client.Group.Create().SetName(tag).Save(ctx)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.Group.DeleteOneID(created.ID).Exec(context.Background())
	})
	for _, permission := range permissions {
		if _, err := db.Client.GroupPermission.Create().
			SetGroupID(created.ID).
			SetPermission(permission).
			Save(ctx); err != nil {
			t.Fatalf("create group permission: %v", err)
		}
	}
	return created.ID
}

func TestSettingsGetAsOwner(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
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
	if count := registerRowCount(t, db); count != 5 {
		t.Fatalf("register rows = %d, want the five seeded rows", count)
	}
	assertRegisterEnabled(t, db, true)
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

func TestSettingsGroupPermissionsSplitByGroup(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register", "site", "auth")
	ts := newSettingsTestApp(t, db)

	registerGroupID := createGroupWithPermissions(t, db, "reg", []string{rbac.PermissionRegisterView, rbac.PermissionRegisterEdit})
	settingsGroupID := createGroupWithPermissions(t, db, "nav", []string{rbac.PermissionSettingsView, rbac.PermissionSettingsEdit})

	registerUser := seedClient(t, ts, createUserInGroup(t, db, registerGroupID))
	settingsUser := seedClient(t, ts, createUserInGroup(t, db, settingsGroupID))

	resp := doJSON(t, registerUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, registerUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, registerUser, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, registerUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/site", nil)
	expectStatus(t, resp, http.StatusForbidden)
	if code := decodeErrorCode(t, resp); code != "forbidden" {
		t.Fatalf("site as register user: code = %q, want forbidden", code)
	}

	resp = doJSON(t, settingsUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/site", nil)
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, settingsUser, http.MethodPatch, ts.URL+"/api/v1/admin/settings/site", map[string]any{"title": "拆分测试"})
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, settingsUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusForbidden)
	if code := decodeErrorCode(t, resp); code != "forbidden" {
		t.Fatalf("register as settings user: code = %q, want forbidden", code)
	}

	resp = doJSON(t, settingsUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	resp = doJSON(t, settingsUser, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	resp = doJSON(t, registerUser, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"password_min_length": 10})
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = doJSON(t, registerUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/nope", nil)
	expectStatus(t, resp, http.StatusForbidden)
	resp.Body.Close()

	resp = doJSON(t, settingsUser, http.MethodGet, ts.URL+"/api/v1/admin/settings/nope", nil)
	expectStatus(t, resp, http.StatusNotFound)
	resp.Body.Close()
}

func TestSettingsPublicGetAnonymously(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth", "register", "site")
	ts := newSettingsTestApp(t, db)

	resp := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/public/settings", nil)
	expectStatus(t, resp, http.StatusOK)

	items := decodePublicItems(t, resp)
	if len(items) != 3 {
		t.Fatalf("public items = %v, want the site, register and auth groups", items)
	}
	if items[0]["group"] != "site" {
		t.Fatalf("group = %v, want site first in registry order", items[0]["group"])
	}
	siteValues, ok := items[0]["values"].(map[string]any)
	if !ok {
		t.Fatalf("site values = %v, want an object", items[0]["values"])
	}
	if len(siteValues) != 6 {
		t.Fatalf("site values = %v, want exactly the six defaults", items[0]["values"])
	}
	for _, name := range []string{"title", "subtitle", "description", "keywords", "url"} {
		if siteValues[name] != "" {
			t.Fatalf("site.%s = %v, want the empty default", name, siteValues[name])
		}
	}
	if siteValues["block_search_engines"] != false {
		t.Fatalf("site.block_search_engines = %v, want the false default", siteValues["block_search_engines"])
	}
	if items[1]["group"] != "register" {
		t.Fatalf("group = %v, want register", items[1]["group"])
	}
	values, ok := items[1]["values"].(map[string]any)
	if !ok || values["enabled"] != true {
		t.Fatalf("values = %v, want enabled true merged from default", items[1]["values"])
	}
	if items[2]["group"] != "auth" {
		t.Fatalf("group = %v, want auth", items[2]["group"])
	}
	authValues, ok := items[2]["values"].(map[string]any)
	if !ok || len(authValues) != 4 {
		t.Fatalf("public auth values = %v, want totp_enabled plus the three captcha keys", items[2]["values"])
	}
	if authValues["totp_enabled"] != true {
		t.Fatalf("public auth.totp_enabled = %v, want true", authValues["totp_enabled"])
	}
	if authValues["captcha_enabled"] != false || authValues["captcha_provider"] != "" || authValues["turnstile_site_key"] != "" {
		t.Fatalf("public auth captcha values = %v, want the empty defaults", authValues)
	}
}

func TestSettingsPatchSiteReadsBackPublicly(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth", "register", "site")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/site", map[string]any{"title": "我的博客", "block_search_engines": true})
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "site" {
		t.Fatalf("group = %q, want site", group)
	}
	if values["title"] != "我的博客" {
		t.Fatalf("site.title = %v, want the written title", values["title"])
	}
	if values["block_search_engines"] != true {
		t.Fatalf("site.block_search_engines = %v, want the written true", values["block_search_engines"])
	}
	if values["subtitle"] != "" {
		t.Fatalf("site.subtitle = %v, want the untouched empty default", values["subtitle"])
	}

	public := doJSON(t, &http.Client{}, http.MethodGet, ts.URL+"/api/v1/public/settings", nil)
	expectStatus(t, public, http.StatusOK)
	items := decodePublicItems(t, public)
	if len(items) != 3 || items[0]["group"] != "site" {
		t.Fatalf("public items = %v, want site first then register and auth", items)
	}
	siteValues, ok := items[0]["values"].(map[string]any)
	if !ok || siteValues["title"] != "我的博客" || siteValues["block_search_engines"] != true {
		t.Fatalf("public site values = %v, want the written title and bool", items[0]["values"])
	}
}

func TestSettingsAdminReadsRegisterDefaults(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/register", nil)
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "register" {
		t.Fatalf("group = %q, want register", group)
	}
	if len(values) != 5 {
		t.Fatalf("register values = %v, want the five defaults", values)
	}
	if values["enabled"] != true {
		t.Fatalf("register.enabled = %v, want true", values["enabled"])
	}
	if want := rbac.PresetGroupNameByKey(rbac.GroupKeyUser); values["default_group"] != want {
		t.Fatalf("register.default_group = %v, want %q", values["default_group"], want)
	}
	if values["email_domain_control_enabled"] != false || values["email_verification_enabled"] != false {
		t.Fatalf("register flags = %v, want false", values)
	}
	if values["allowed_email_domains"] != "" {
		t.Fatalf("register.allowed_email_domains = %v, want empty", values["allowed_email_domains"])
	}
}

func TestSettingsAdminPatchAuthRoundtrip(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, resp, http.StatusOK)
	group, values := decodeGroup(t, resp)
	if group != "auth" {
		t.Fatalf("group = %q, want auth", group)
	}
	if len(values) != 11 {
		t.Fatalf("auth values = %v, want the eleven defaults", values)
	}
	if values["session_max_age_days"] != float64(7) {
		t.Fatalf("auth.session_max_age_days = %v, want 7", values["session_max_age_days"])
	}
	if values["password_min_length"] != float64(8) {
		t.Fatalf("auth.password_min_length = %v, want 8", values["password_min_length"])
	}
	if values["captcha_provider"] != "" {
		t.Fatalf("auth.captcha_provider = %v, want empty", values["captcha_provider"])
	}
	if values["turnstile_site_key"] != "" || values["turnstile_secret_key"] != "" {
		t.Fatalf("turnstile keys = %v / %v, want empty", values["turnstile_site_key"], values["turnstile_secret_key"])
	}
	if values["totp_enabled"] != true {
		t.Fatalf("auth.totp_enabled = %v, want true", values["totp_enabled"])
	}

	patch := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"session_max_age_days": 14})
	expectStatus(t, patch, http.StatusOK)
	_, values = decodeGroup(t, patch)
	if values["session_max_age_days"] != float64(14) {
		t.Fatalf("patched session_max_age_days = %v, want 14", values["session_max_age_days"])
	}
	if values["password_min_length"] != float64(8) {
		t.Fatalf("untouched password_min_length = %v, want 8", values["password_min_length"])
	}

	after := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, after, http.StatusOK)
	_, values = decodeGroup(t, after)
	if values["session_max_age_days"] != float64(14) {
		t.Fatalf("readback session_max_age_days = %v, want 14", values["session_max_age_days"])
	}
}

func TestSettingsPatchTurnstileKeysRoundtrip(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	patch := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{
		"captcha_enabled":      true,
		"captcha_provider":     "turnstile",
		"turnstile_site_key":   "0x4AAAAAAAsite",
		"turnstile_secret_key": "0x4AAAAAAAsecret",
	})
	expectStatus(t, patch, http.StatusOK)

	after := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/settings/auth", nil)
	expectStatus(t, after, http.StatusOK)
	_, values := decodeGroup(t, after)
	if values["captcha_enabled"] != true || values["captcha_provider"] != "turnstile" {
		t.Fatalf("captcha gate = %v / %v, want true / turnstile", values["captcha_enabled"], values["captcha_provider"])
	}
	if values["turnstile_site_key"] != "0x4AAAAAAAsite" {
		t.Fatalf("readback turnstile_site_key = %v", values["turnstile_site_key"])
	}
	if values["turnstile_secret_key"] != "0x4AAAAAAAsecret" {
		t.Fatalf("readback turnstile_secret_key = %v", values["turnstile_secret_key"])
	}
}

func TestSettingsPatchTurnsRegistrationOff(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
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
	if count := registerRowCount(t, db); count != 5 {
		t.Fatalf("register rows = %d, want the five seeded rows", count)
	}
	assertRegisterEnabled(t, db, false)
}

func TestSettingsPatchUnknownItem(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
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
	assertRegisterEnabled(t, db, true)
}

func TestSettingsPatchTypeMismatch(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": "no"})
	expectStatus(t, resp, http.StatusBadRequest)
	errObj := decodeError(t, resp)
	if errObj["code"] != "invalid_request" || errObj["param"] != "enabled" {
		t.Fatalf("error = %v, want invalid_request with param enabled", errObj)
	}
	assertRegisterEnabled(t, db, true)
}

func TestSettingsPatchEmptyBody(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
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
	resetBuiltin(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	for _, payload := range []string{"{not json", `["enabled"]`, `"enabled"`, ""} {
		resp := doRaw(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", payload)
		expectStatus(t, resp, http.StatusBadRequest)
		if code := decodeErrorCode(t, resp); code != "invalid_request" {
			t.Fatalf("payload %q: code = %q, want invalid_request", payload, code)
		}
	}
	assertRegisterEnabled(t, db, true)
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
	resetBuiltin(t, db, "register")
	ts := newSettingsTestApp(t, db)
	plain := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDUser))
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	denied := doJSON(t, plain, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, denied, http.StatusForbidden)
	if code := decodeErrorCode(t, denied); code != "forbidden" {
		t.Fatalf("code = %q, want forbidden", code)
	}
	assertRegisterEnabled(t, db, true)

	allowed := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, allowed, http.StatusOK)
	allowed.Body.Close()
}

func TestSettingsPatchRegisterDomainControlAllowsEmptyList(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	enable := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"email_domain_control_enabled": true})
	expectStatus(t, enable, http.StatusOK)
	enable.Body.Close()

	set := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"allowed_email_domains": "example.com"})
	expectStatus(t, set, http.StatusOK)
	set.Body.Close()

	for _, raw := range []string{"", " , ，"} {
		clear := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"allowed_email_domains": raw})
		expectStatus(t, clear, http.StatusOK)
		_, values := decodeGroup(t, clear)
		if values["allowed_email_domains"] != raw {
			t.Fatalf("allowed_email_domains = %v, want %q", values["allowed_email_domains"], raw)
		}
		if values["email_domain_control_enabled"] != true {
			t.Fatalf("email_domain_control_enabled = %v, want true", values["email_domain_control_enabled"])
		}
	}

	blanks := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"email_domain_control_enabled": true, "allowed_email_domains": " , ，"})
	expectStatus(t, blanks, http.StatusOK)
	blanks.Body.Close()
}

func TestSettingsPatchAuthMinLengthRange(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	for _, length := range []int{3, 65} {
		resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"password_min_length": length})
		expectStatus(t, resp, http.StatusConflict)
		body := decodeError(t, resp)
		if body["code"] != "setting_conflict" || body["param"] != "password_min_length" {
			t.Fatalf("length %d: conflict body = %v", length, body)
		}
	}

	ok := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"password_min_length": 16})
	expectStatus(t, ok, http.StatusOK)
	_, values := decodeGroup(t, ok)
	if values["password_min_length"] != float64(16) {
		t.Fatalf("password_min_length = %v, want 16", values["password_min_length"])
	}
}

func TestSettingsPatchAuthSessionMaxAgeRange(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	for _, days := range []int{0, 31} {
		resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"session_max_age_days": days})
		expectStatus(t, resp, http.StatusConflict)
		body := decodeError(t, resp)
		if body["code"] != "setting_conflict" || body["param"] != "session_max_age_days" {
			t.Fatalf("days %d: conflict body = %v", days, body)
		}
	}

	ok := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"session_max_age_enabled": true, "session_max_age_days": 30})
	expectStatus(t, ok, http.StatusOK)
	_, values := decodeGroup(t, ok)
	if values["session_max_age_days"] != float64(30) || values["session_max_age_enabled"] != true {
		t.Fatalf("session max age = %v, want enabled with 30 days", values)
	}
}

func TestSettingsPatchAuthSessionMaxAgeRejectsDirtyDaysOnEnable(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	if err := module.Service.SetNumber(context.Background(), "auth", "session_max_age_days", 0); err != nil {
		t.Fatalf("seed dirty days: %v", err)
	}

	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))
	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"session_max_age_enabled": true})
	expectStatus(t, resp, http.StatusConflict)
	body := decodeError(t, resp)
	if body["code"] != "setting_conflict" || body["param"] != "session_max_age_days" {
		t.Fatalf("conflict body = %v", body)
	}
}

func TestSettingsPatchAuthSessionMaxAgeAllowsDisableWithDirtyDays(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	ctx := context.Background()
	if err := module.Service.SetNumber(ctx, "auth", "session_max_age_days", 0); err != nil {
		t.Fatalf("seed dirty days: %v", err)
	}
	if err := module.Service.SetBool(ctx, "auth", "session_max_age_enabled", true); err != nil {
		t.Fatalf("seed enabled switch: %v", err)
	}

	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))
	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"session_max_age_enabled": false})
	expectStatus(t, resp, http.StatusOK)
	_, values := decodeGroup(t, resp)
	if values["session_max_age_enabled"] != false {
		t.Fatalf("session_max_age_enabled = %v, want false", values["session_max_age_enabled"])
	}
}

func TestSettingsPatchAuthMinLengthIgnoresDirtySessionDays(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "auth")
	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	if err := module.Service.SetNumber(context.Background(), "auth", "session_max_age_days", 0); err != nil {
		t.Fatalf("seed dirty days: %v", err)
	}

	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))
	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/auth", map[string]any{"password_min_length": 12})
	expectStatus(t, resp, http.StatusOK)
	_, values := decodeGroup(t, resp)
	if values["password_min_length"] != float64(12) {
		t.Fatalf("password_min_length = %v, want 12", values["password_min_length"])
	}
}

func TestSettingsPatchIgnoresDirtyStateOutsideValidateOn(t *testing.T) {
	db := testDB(t)
	resetBuiltin(t, db, "register")
	module, err := New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	if err := module.Service.SetBool(context.Background(), "register", "email_domain_control_enabled", true); err != nil {
		t.Fatalf("seed dirty state: %v", err)
	}
	ts := newSettingsTestApp(t, db)
	owner := seedClient(t, ts, createUserInGroup(t, db, rbac.GroupIDOwner))

	resp := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"enabled": false})
	expectStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	touch := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/settings/register", map[string]any{"email_domain_control_enabled": false})
	expectStatus(t, touch, http.StatusOK)
	touch.Body.Close()
}
