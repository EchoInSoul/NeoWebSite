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

func decodeValues(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()

	var values map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&values); err != nil {
		t.Fatalf("decode values: %v", err)
	}
	return values
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
	values := decodeValues(t, resp)
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
