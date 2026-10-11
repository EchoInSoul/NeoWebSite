package access

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
)

func newAccessTestApp(t *testing.T, db *database.Database) *httptest.Server {
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

	module := New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.LoadIdentity(module.Service, sm))
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

func decodeErrorCode(t *testing.T, resp *http.Response) string {
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
	code, _ := errObj["code"].(string)
	return code
}

func decodeItems[T any](t *testing.T, resp *http.Response) []T {
	t.Helper()
	defer resp.Body.Close()

	var body itemList[T]
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode items: %v", err)
	}
	return body.Items
}

func decodeValue[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()

	var body T
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode value: %v", err)
	}
	return body
}

func expectStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
}

func TestAdminRequiresAuthAndPermission(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)

	anonymous, err := http.Get(ts.URL + "/api/v1/admin/groups")
	if err != nil {
		t.Fatalf("anonymous get: %v", err)
	}
	expectStatus(t, anonymous, http.StatusUnauthorized)
	if code := decodeErrorCode(t, anonymous); code != "unauthorized" {
		t.Fatalf("anonymous error = %q, want unauthorized", code)
	}

	plainID := createTestUser(t, db, rbac.GroupIDUser)
	plain := seedClient(t, ts, plainID)
	forbidden := doJSON(t, plain, http.MethodGet, ts.URL+"/api/v1/admin/groups", nil)
	expectStatus(t, forbidden, http.StatusForbidden)
	if code := decodeErrorCode(t, forbidden); code != "forbidden" {
		t.Fatalf("plain error = %q, want forbidden", code)
	}
}

func TestPermissionsEndpoint(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)

	resp := doJSON(t, seedClient(t, ts, ownerID), http.MethodGet, ts.URL+"/api/v1/admin/permissions", nil)
	expectStatus(t, resp, http.StatusOK)
	items := decodeItems[map[string]any](t, resp)
	if len(items) != len(rbac.Catalog()) {
		t.Fatalf("permissions = %d items, want %d", len(items), len(rbac.Catalog()))
	}
	if items[0]["key"] != rbac.PermissionUsersView || items[0]["name"] != "查看用户" {
		t.Fatalf("first permission = %+v, want key+name in the documented shape", items[0])
	}
}

func TestGroupsList(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)
	groupID := createTestGroup(t, db, "g-"+uniqueTag(t), []string{rbac.PermissionUsersView})
	createTestUser(t, db, groupID)
	createTestUser(t, db, rbac.GroupIDUser)

	resp := doJSON(t, seedClient(t, ts, ownerID), http.MethodGet, ts.URL+"/api/v1/admin/groups", nil)
	expectStatus(t, resp, http.StatusOK)
	groups := decodeItems[Group](t, resp)

	byKey := map[string]Group{}
	var custom *Group
	for i := range groups {
		if groups[i].SystemKey != nil {
			byKey[*groups[i].SystemKey] = groups[i]
		}
		if groups[i].ID == groupID.String() {
			custom = &groups[i]
		}
	}
	if len(byKey) != 3 {
		t.Fatalf("preset groups = %d, want 3", len(byKey))
	}
	if !byKey[rbac.GroupKeyOwner].IsPreset || !reflect.DeepEqual(byKey[rbac.GroupKeyOwner].Permissions, rbac.All()) {
		t.Fatalf("owner group = %+v, want all permissions", byKey[rbac.GroupKeyOwner])
	}
	if !reflect.DeepEqual(byKey[rbac.GroupKeyAdmin].Permissions, rbac.All()) {
		t.Fatalf("admin permissions = %v, want all", byKey[rbac.GroupKeyAdmin].Permissions)
	}
	if len(byKey[rbac.GroupKeyUser].Permissions) != 0 {
		t.Fatalf("plain user permissions = %v, want empty", byKey[rbac.GroupKeyUser].Permissions)
	}
	if byKey[rbac.GroupKeyUser].MemberCount < 1 {
		t.Fatalf("plain user member count = %d, want >= 1", byKey[rbac.GroupKeyUser].MemberCount)
	}
	if custom == nil {
		t.Fatalf("custom group %s missing", groupID)
	}
	if custom.SystemKey != nil || custom.IsPreset || custom.MemberCount != 1 {
		t.Fatalf("custom group = %+v, want custom with 1 member", custom)
	}
	if !reflect.DeepEqual(custom.Permissions, []string{rbac.PermissionUsersView}) {
		t.Fatalf("custom permissions = %v, want [users.view]", custom.Permissions)
	}
}

func TestGroupCreate(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)
	owner := seedClient(t, ts, ownerID)
	tag := uniqueTag(t)

	created := doJSON(t, owner, http.MethodPost, ts.URL+"/api/v1/admin/groups", map[string]any{
		"name":        "编辑-" + tag,
		"permissions": []string{rbac.PermissionUsersEdit},
	})
	expectStatus(t, created, http.StatusCreated)
	group := decodeValue[Group](t, created)
	t.Cleanup(func() {
		_ = db.Client.Group.DeleteOneID(uuid.MustParse(group.ID)).Exec(context.Background())
	})
	want := []string{rbac.PermissionUsersEdit, rbac.PermissionUsersView}
	if !reflect.DeepEqual(group.Permissions, want) {
		t.Fatalf("created permissions = %v, want %v (expanded)", group.Permissions, want)
	}

	duplicate := doJSON(t, owner, http.MethodPost, ts.URL+"/api/v1/admin/groups", map[string]any{"name": "管理员"})
	expectStatus(t, duplicate, http.StatusConflict)
	if code := decodeErrorCode(t, duplicate); code != "group_name_taken" {
		t.Fatalf("duplicate error = %q, want group_name_taken", code)
	}

	unknown := doJSON(t, owner, http.MethodPost, ts.URL+"/api/v1/admin/groups", map[string]any{
		"name":        "x-" + tag,
		"permissions": []string{"content.posts.edit"},
	})
	expectStatus(t, unknown, http.StatusBadRequest)
	if code := decodeErrorCode(t, unknown); code != "invalid_permission" {
		t.Fatalf("unknown key error = %q, want invalid_permission", code)
	}

	blank := doJSON(t, owner, http.MethodPost, ts.URL+"/api/v1/admin/groups", map[string]any{"name": "   "})
	expectStatus(t, blank, http.StatusBadRequest)
	if code := decodeErrorCode(t, blank); code != "invalid_request" {
		t.Fatalf("blank name error = %q, want invalid_request", code)
	}

	viewerGroupID := createTestGroup(t, db, "viewer-"+tag, []string{rbac.PermissionGroupsView})
	viewerID := createTestUser(t, db, viewerGroupID)
	limited := doJSON(t, seedClient(t, ts, viewerID), http.MethodPost, ts.URL+"/api/v1/admin/groups", map[string]any{"name": "y-" + tag})
	expectStatus(t, limited, http.StatusForbidden)
	if code := decodeErrorCode(t, limited); code != "forbidden" {
		t.Fatalf("limited error = %q, want forbidden", code)
	}
}

func TestGroupUpdateAndDelete(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)
	owner := seedClient(t, ts, ownerID)
	tag := uniqueTag(t)

	locked := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/groups/"+rbac.GroupIDAdmin.String(), map[string]any{"name": "x"})
	expectStatus(t, locked, http.StatusConflict)
	if code := decodeErrorCode(t, locked); code != "preset_group_locked" {
		t.Fatalf("preset update error = %q, want preset_group_locked", code)
	}

	groupID := createTestGroup(t, db, "g-"+tag, []string{rbac.PermissionUsersView})
	updated := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/groups/"+groupID.String(), map[string]any{
		"name":        "新名-" + tag,
		"permissions": []string{rbac.PermissionGroupsEdit},
	})
	expectStatus(t, updated, http.StatusOK)
	group := decodeValue[Group](t, updated)
	if group.Name != "新名-"+tag || !reflect.DeepEqual(group.Permissions, []string{rbac.PermissionGroupsEdit, rbac.PermissionGroupsView}) {
		t.Fatalf("updated group = %+v", group)
	}

	empty := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/groups/"+groupID.String(), map[string]any{})
	expectStatus(t, empty, http.StatusBadRequest)
	if code := decodeErrorCode(t, empty); code != "invalid_request" {
		t.Fatalf("empty patch error = %q, want invalid_request", code)
	}

	createTestUser(t, db, groupID)
	notEmpty := doJSON(t, owner, http.MethodDelete, ts.URL+"/api/v1/admin/groups/"+groupID.String(), nil)
	expectStatus(t, notEmpty, http.StatusConflict)
	if code := decodeErrorCode(t, notEmpty); code != "group_not_empty" {
		t.Fatalf("non-empty delete error = %q, want group_not_empty", code)
	}

	emptyGroupID := createTestGroup(t, db, "e-"+tag, nil)
	deleted := doJSON(t, owner, http.MethodDelete, ts.URL+"/api/v1/admin/groups/"+emptyGroupID.String(), nil)
	expectStatus(t, deleted, http.StatusOK)
	var confirmation struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.NewDecoder(deleted.Body).Decode(&confirmation); err != nil {
		t.Fatalf("decode delete confirmation: %v", err)
	}
	deleted.Body.Close()
	if confirmation.ID != emptyGroupID.String() || !confirmation.Deleted {
		t.Fatalf("delete confirmation = %+v, want id %s deleted true", confirmation, emptyGroupID)
	}

	missing := doJSON(t, owner, http.MethodDelete, ts.URL+"/api/v1/admin/groups/"+uuid.New().String(), nil)
	expectStatus(t, missing, http.StatusNotFound)
	if code := decodeErrorCode(t, missing); code != "not_found" {
		t.Fatalf("missing delete error = %q, want not_found", code)
	}
}

func TestUsersEndpointsAndRules(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)
	owner := seedClient(t, ts, ownerID)
	tag := uniqueTag(t)

	groupID := createTestGroup(t, db, "g-"+tag, []string{rbac.PermissionGroupsEdit})
	userID := createTestUser(t, db, groupID)
	addUserPermissions(t, db, userID, []string{rbac.PermissionUsersView})

	list := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/users", nil)
	expectStatus(t, list, http.StatusOK)
	users := decodeItems[UserSummary](t, list)
	var found *UserSummary
	for i := range users {
		if users[i].ID == userID.String() {
			found = &users[i]
		}
	}
	if found == nil {
		t.Fatalf("user %s missing from list", userID)
	}
	if found.Group.SystemKey != nil || !reflect.DeepEqual(found.ExtraPermissions, []string{rbac.PermissionUsersView}) {
		t.Fatalf("listed user = %+v", found)
	}

	detail := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/users/"+userID.String(), nil)
	expectStatus(t, detail, http.StatusOK)
	user := decodeValue[UserDetail](t, detail)
	want := []string{rbac.PermissionGroupsEdit, rbac.PermissionUsersView, rbac.PermissionGroupsView}
	if !reflect.DeepEqual(user.Permissions, want) {
		t.Fatalf("detail permissions = %v, want %v", user.Permissions, want)
	}

	missing := doJSON(t, owner, http.MethodGet, ts.URL+"/api/v1/admin/users/"+uuid.New().String(), nil)
	expectStatus(t, missing, http.StatusNotFound)
	if code := decodeErrorCode(t, missing); code != "not_found" {
		t.Fatalf("missing user error = %q, want not_found", code)
	}

	plainTargetID := createTestUser(t, db, rbac.GroupIDUser)
	promoted := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+plainTargetID.String(), map[string]any{
		"group_id": rbac.GroupIDAdmin.String(),
	})
	expectStatus(t, promoted, http.StatusOK)
	promotedUser := decodeValue[UserDetail](t, promoted)
	if promotedUser.Group.SystemKey == nil || *promotedUser.Group.SystemKey != rbac.GroupKeyAdmin {
		t.Fatalf("promoted group = %+v, want admin", promotedUser.Group)
	}

	self := doJSON(t, owner, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+ownerID.String(), map[string]any{
		"extra_permissions": []string{rbac.PermissionUsersView},
	})
	expectStatus(t, self, http.StatusForbidden)
	if code := decodeErrorCode(t, self); code != "cannot_modify_self" {
		t.Fatalf("self error = %q, want cannot_modify_self", code)
	}

	managerGroupID := createTestGroup(t, db, "mgr-"+tag, []string{rbac.PermissionUsersEdit})
	managerID := createTestUser(t, db, managerGroupID)
	manager := seedClient(t, ts, managerID)

	protected := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+plainTargetID.String(), map[string]any{
		"group_id": rbac.GroupIDOwner.String(),
	})
	expectStatus(t, protected, http.StatusForbidden)
	if code := decodeErrorCode(t, protected); code != "owner_protected" {
		t.Fatalf("promote to owner error = %q, want owner_protected", code)
	}

	richGroupID := createTestGroup(t, db, "rich-"+tag, []string{rbac.PermissionGroupsEdit})
	grantForbidden := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+plainTargetID.String(), map[string]any{
		"group_id": richGroupID.String(),
	})
	expectStatus(t, grantForbidden, http.StatusForbidden)
	if code := decodeErrorCode(t, grantForbidden); code != "owner_protected" {
		t.Fatalf("moving admin member error = %q, want owner_protected", code)
	}

	freeTargetID := createTestUser(t, db, rbac.GroupIDUser)
	grantForbidden = doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+freeTargetID.String(), map[string]any{
		"group_id": richGroupID.String(),
	})
	expectStatus(t, grantForbidden, http.StatusForbidden)
	if code := decodeErrorCode(t, grantForbidden); code != "grant_forbidden" {
		t.Fatalf("group transfer error = %q, want grant_forbidden", code)
	}

	granted := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+freeTargetID.String(), map[string]any{
		"extra_permissions": []string{rbac.PermissionUsersView},
	})
	expectStatus(t, granted, http.StatusOK)
	grantedUser := decodeValue[UserDetail](t, granted)
	if !reflect.DeepEqual(grantedUser.ExtraPermissions, []string{rbac.PermissionUsersView}) {
		t.Fatalf("extra permissions = %v, want [users.view]", grantedUser.ExtraPermissions)
	}

	cleared := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+freeTargetID.String(), map[string]any{
		"extra_permissions": []string{},
	})
	expectStatus(t, cleared, http.StatusOK)
	clearedUser := decodeValue[UserDetail](t, cleared)
	if len(clearedUser.ExtraPermissions) != 0 {
		t.Fatalf("cleared extra permissions = %v, want empty", clearedUser.ExtraPermissions)
	}

	empty := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+freeTargetID.String(), map[string]any{})
	expectStatus(t, empty, http.StatusBadRequest)
	if code := decodeErrorCode(t, empty); code != "invalid_request" {
		t.Fatalf("empty patch error = %q, want invalid_request", code)
	}

	blankGroup := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+freeTargetID.String(), map[string]any{
		"group_id": "",
	})
	expectStatus(t, blankGroup, http.StatusBadRequest)
	if code := decodeErrorCode(t, blankGroup); code != "invalid_request" {
		t.Fatalf("blank group error = %q, want invalid_request", code)
	}

	unknownKey := doJSON(t, manager, http.MethodPatch, ts.URL+"/api/v1/admin/users/"+freeTargetID.String(), map[string]any{
		"extra_permissions": []string{"content.posts.edit"},
	})
	expectStatus(t, unknownKey, http.StatusBadRequest)
	if code := decodeErrorCode(t, unknownKey); code != "invalid_permission" {
		t.Fatalf("unknown key error = %q, want invalid_permission", code)
	}
}

func TestEditImpliesViewOnEndpoints(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	tag := uniqueTag(t)

	groupID := createTestGroup(t, db, "editor-"+tag, []string{rbac.PermissionUsersEdit})
	userID := createTestUser(t, db, groupID)

	list := doJSON(t, seedClient(t, ts, userID), http.MethodGet, ts.URL+"/api/v1/admin/users", nil)
	expectStatus(t, list, http.StatusOK)
	items := decodeItems[UserSummary](t, list)
	if len(items) == 0 {
		t.Fatal("users list is empty, want at least the seeded users")
	}
}

func TestConcurrentReplaceKeepsBothWritesServing(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)
	owner := seedClient(t, ts, ownerID)
	tag := uniqueTag(t)
	groupID := createTestGroup(t, db, "race-"+tag, []string{rbac.PermissionUsersView})
	userID := createTestUser(t, db, rbac.GroupIDUser)

	patch := func(endpoint string, payload map[string]any) int {
		body, err := json.Marshal(payload)
		if err != nil {
			return -1
		}
		request, err := http.NewRequest(http.MethodPatch, endpoint, bytes.NewReader(body))
		if err != nil {
			return -2
		}
		request.Header.Set("Content-Type", "application/json")
		resp, err := owner.Do(request)
		if err != nil {
			return -3
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	for round := 0; round < 5; round++ {
		statuses := make([]int, 2)
		var waitGroup sync.WaitGroup
		waitGroup.Add(2)
		go func() {
			defer waitGroup.Done()
			statuses[0] = patch(ts.URL+"/api/v1/admin/groups/"+groupID.String(), map[string]any{
				"permissions": []string{rbac.PermissionGroupsEdit},
			})
		}()
		go func() {
			defer waitGroup.Done()
			statuses[1] = patch(ts.URL+"/api/v1/admin/groups/"+groupID.String(), map[string]any{
				"permissions": []string{rbac.PermissionGroupsEdit, rbac.PermissionUsersView},
			})
		}()
		waitGroup.Wait()
		for _, status := range statuses {
			if status != http.StatusOK {
				t.Fatalf("round %d: concurrent group replace status = %d, want 200 (all: %v)", round, status, statuses)
			}
		}

		userStatuses := make([]int, 2)
		var userWaitGroup sync.WaitGroup
		userWaitGroup.Add(2)
		go func() {
			defer userWaitGroup.Done()
			userStatuses[0] = patch(ts.URL+"/api/v1/admin/users/"+userID.String(), map[string]any{
				"extra_permissions": []string{rbac.PermissionUsersView},
			})
		}()
		go func() {
			defer userWaitGroup.Done()
			userStatuses[1] = patch(ts.URL+"/api/v1/admin/users/"+userID.String(), map[string]any{
				"extra_permissions": []string{rbac.PermissionUsersView, rbac.PermissionGroupsView},
			})
		}()
		userWaitGroup.Wait()
		for _, status := range userStatuses {
			if status != http.StatusOK {
				t.Fatalf("round %d: concurrent user replace status = %d, want 200 (all: %v)", round, status, userStatuses)
			}
		}
	}
}

func TestGroupCreateConcurrentSameName(t *testing.T) {
	db := testDB(t)
	ts := newAccessTestApp(t, db)
	_, ownerID := createOwnerIdentity(t, db)
	owner := seedClient(t, ts, ownerID)
	name := "race-" + uniqueTag(t)

	statuses := make([]int, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for i := range statuses {
		go func(index int) {
			defer waitGroup.Done()
			resp := doJSON(t, owner, http.MethodPost, ts.URL+"/api/v1/admin/groups", map[string]any{"name": name})
			statuses[index] = resp.StatusCode
			if resp.StatusCode == http.StatusCreated {
				var body struct {
					ID string `json:"id"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&body); err == nil && body.ID != "" {
					t.Cleanup(func() {
						_ = db.Client.Group.DeleteOneID(uuid.MustParse(body.ID)).Exec(context.Background())
					})
				}
			}
			resp.Body.Close()
		}(i)
	}
	waitGroup.Wait()

	created, conflicted := 0, 0
	for _, status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			t.Fatalf("unexpected status %d (all: %v)", status, statuses)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("created/conflicted = %d/%d, want 1/1", created, conflicted)
	}
}
