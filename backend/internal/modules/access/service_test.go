package access

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/userpermission"
)

func ptr[T any](v T) *T {
	return &v
}

func createTestUser(t *testing.T, db *database.Database, groupID uuid.UUID) uuid.UUID {
	t.Helper()

	tag := uniqueTag(t)
	created, err := db.Client.User.Create().
		SetEmail("u+" + tag + "@test.local").
		SetNickname("nick" + tag).
		SetPasswordHash("hash-" + tag).
		SetEmailVerificationStatus("not_required").
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

func createTestGroup(t *testing.T, db *database.Database, name string, permissions []string) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	created, err := db.Client.Group.Create().SetName(name).Save(ctx)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.Group.DeleteOneID(created.ID).Exec(context.Background())
	})
	addGroupPermissions(t, db, created.ID, permissions)
	return created.ID
}

func addGroupPermissions(t *testing.T, db *database.Database, groupID uuid.UUID, permissions []string) {
	t.Helper()

	ctx := context.Background()
	for _, permission := range permissions {
		if _, err := db.Client.GroupPermission.Create().
			SetGroupID(groupID).
			SetPermission(permission).
			Save(ctx); err != nil {
			t.Fatalf("create group permission: %v", err)
		}
	}
}

func addUserPermissions(t *testing.T, db *database.Database, userID uuid.UUID, permissions []string) {
	t.Helper()

	ctx := context.Background()
	for _, permission := range permissions {
		if _, err := db.Client.UserPermission.Create().
			SetUserID(userID).
			SetPermission(permission).
			Save(ctx); err != nil {
			t.Fatalf("create user permission: %v", err)
		}
	}
}

func identityFor(t *testing.T, db *database.Database, userID uuid.UUID) guard.Identity {
	t.Helper()

	identity, err := newService(db).IdentityByUserID(context.Background(), userID.String())
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	return identity
}

func createOwnerIdentity(t *testing.T, db *database.Database) (guard.Identity, uuid.UUID) {
	t.Helper()

	ownerID := createTestUser(t, db, rbac.GroupIDOwner)
	return identityFor(t, db, ownerID), ownerID
}

func TestIdentityByUserID(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()

	ownerID := createTestUser(t, db, rbac.GroupIDOwner)
	ownerIdentity := identityFor(t, db, ownerID)
	if ownerIdentity.GroupKey != rbac.GroupKeyOwner {
		t.Fatalf("owner group key = %q, want %q", ownerIdentity.GroupKey, rbac.GroupKeyOwner)
	}
	if !reflect.DeepEqual(ownerIdentity.Permissions, rbac.All()) {
		t.Fatalf("owner permissions = %v, want %v", ownerIdentity.Permissions, rbac.All())
	}

	groupID := createTestGroup(t, db, "g-"+uniqueTag(t), []string{rbac.PermissionUsersEdit})
	memberID := createTestUser(t, db, groupID)
	memberIdentity := identityFor(t, db, memberID)
	if memberIdentity.GroupKey != "" {
		t.Fatalf("custom group key = %q, want empty", memberIdentity.GroupKey)
	}
	if !reflect.DeepEqual(memberIdentity.Permissions, []string{rbac.PermissionUsersEdit, rbac.PermissionUsersView}) {
		t.Fatalf("member permissions = %v, want edit+view", memberIdentity.Permissions)
	}

	addUserPermissions(t, db, memberID, []string{rbac.PermissionGroupsView})
	unionIdentity := identityFor(t, db, memberID)
	if !reflect.DeepEqual(unionIdentity.Permissions, []string{rbac.PermissionUsersEdit, rbac.PermissionGroupsView, rbac.PermissionUsersView}) {
		t.Fatalf("union permissions = %v, want edit+groups.view+users.view", unionIdentity.Permissions)
	}

	if _, err := service.IdentityByUserID(ctx, uuid.New().String()); !errors.Is(err, guard.ErrUserNotFound) {
		t.Fatalf("missing user error = %v, want ErrUserNotFound", err)
	}
	if _, err := service.IdentityByUserID(ctx, "not-a-uuid"); !errors.Is(err, guard.ErrUserNotFound) {
		t.Fatalf("invalid id error = %v, want ErrUserNotFound", err)
	}
}

func TestCreateGroup(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	owner, _ := createOwnerIdentity(t, db)
	tag := uniqueTag(t)

	created, err := service.CreateGroup(ctx, owner, "编辑-"+tag, []string{rbac.PermissionUsersView})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.Group.DeleteOneID(uuid.MustParse(created.ID)).Exec(context.Background())
	})
	if created.IsPreset || created.SystemKey != nil {
		t.Fatalf("created group = %+v, want custom group without system key", created)
	}
	if !reflect.DeepEqual(created.Permissions, []string{rbac.PermissionUsersView}) {
		t.Fatalf("created permissions = %v, want [users.view]", created.Permissions)
	}

	if _, err := service.CreateGroup(ctx, owner, "x-"+tag, []string{"content.posts.edit"}); !errors.Is(err, apierror.ErrInvalidPermission) {
		t.Fatalf("unknown key error = %v, want apierror.ErrInvalidPermission", err)
	}
	if _, err := service.CreateGroup(ctx, owner, "管理员", nil); !errors.Is(err, apierror.ErrGroupNameTaken) {
		t.Fatalf("preset name collision error = %v, want apierror.ErrGroupNameTaken", err)
	}
	if _, err := service.CreateGroup(ctx, owner, "   ", nil); !errors.Is(err, apierror.ErrInvalidRequest) {
		t.Fatalf("blank name error = %v, want apierror.ErrInvalidRequest", err)
	}
	if _, err := service.CreateGroup(ctx, owner, strings.Repeat("x", 65), nil); !errors.Is(err, apierror.ErrInvalidRequest) {
		t.Fatalf("long name error = %v, want apierror.ErrInvalidRequest", err)
	}

	limitedGroupID := createTestGroup(t, db, "lim-"+tag, []string{rbac.PermissionUsersView})
	limitedUserID := createTestUser(t, db, limitedGroupID)
	limited := identityFor(t, db, limitedUserID)
	if _, err := service.CreateGroup(ctx, limited, "y-"+tag, []string{rbac.PermissionUsersEdit}); !errors.Is(err, apierror.ErrGrantForbidden) {
		t.Fatalf("grant error = %v, want apierror.ErrGrantForbidden", err)
	}
}

func TestUpdateGroup(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	owner, _ := createOwnerIdentity(t, db)
	tag := uniqueTag(t)

	if _, err := service.UpdateGroup(ctx, owner, rbac.GroupIDAdmin.String(), UpdateGroupInput{Name: ptr("x")}); !errors.Is(err, apierror.ErrPresetGroupLocked) {
		t.Fatalf("preset locked error = %v, want apierror.ErrPresetGroupLocked", err)
	}
	if err := service.DeleteGroup(ctx, rbac.GroupIDUser.String()); !errors.Is(err, apierror.ErrPresetGroupLocked) {
		t.Fatalf("preset delete error = %v, want apierror.ErrPresetGroupLocked", err)
	}

	groupID := createTestGroup(t, db, "g-"+tag, []string{rbac.PermissionUsersView})
	updated, err := service.UpdateGroup(ctx, owner, groupID.String(), UpdateGroupInput{
		Name:        ptr("新名-" + tag),
		Permissions: ptr([]string{rbac.PermissionGroupsEdit}),
	})
	if err != nil {
		t.Fatalf("update group: %v", err)
	}
	if updated.Name != "新名-"+tag {
		t.Fatalf("name = %q, want 新名-%s", updated.Name, tag)
	}
	if !reflect.DeepEqual(updated.Permissions, []string{rbac.PermissionGroupsEdit, rbac.PermissionGroupsView}) {
		t.Fatalf("permissions = %v, want groups.edit+groups.view", updated.Permissions)
	}

	memberID := createTestUser(t, db, groupID)
	memberIdentity := identityFor(t, db, memberID)
	if !reflect.DeepEqual(memberIdentity.Permissions, []string{rbac.PermissionGroupsEdit, rbac.PermissionGroupsView}) {
		t.Fatalf("member permissions after group update = %v, want groups.edit+groups.view", memberIdentity.Permissions)
	}

	limitedGroupID := createTestGroup(t, db, "lim-"+tag, []string{rbac.PermissionGroupsEdit})
	limitedUserID := createTestUser(t, db, limitedGroupID)
	limited := identityFor(t, db, limitedUserID)

	if _, err := service.UpdateGroup(ctx, limited, groupID.String(), UpdateGroupInput{
		Permissions: ptr([]string{rbac.PermissionGroupsEdit, rbac.PermissionUsersEdit}),
	}); !errors.Is(err, apierror.ErrGrantForbidden) {
		t.Fatalf("grant error = %v, want apierror.ErrGrantForbidden", err)
	}

	removed, err := service.UpdateGroup(ctx, limited, groupID.String(), UpdateGroupInput{
		Permissions: ptr([]string{rbac.PermissionGroupsEdit}),
	})
	if err != nil {
		t.Fatalf("remove without holding mirrored permission: %v", err)
	}
	if !reflect.DeepEqual(removed.Permissions, []string{rbac.PermissionGroupsEdit, rbac.PermissionGroupsView}) {
		t.Fatalf("permissions after removal = %v, want groups.edit+groups.view", removed.Permissions)
	}

	ownUpdate, err := service.UpdateGroup(ctx, limited, limitedGroupID.String(), UpdateGroupInput{
		Permissions: ptr([]string{}),
	})
	if err != nil {
		t.Fatalf("edit own group: %v", err)
	}
	if len(ownUpdate.Permissions) != 0 {
		t.Fatalf("own group permissions = %v, want empty", ownUpdate.Permissions)
	}
}

func TestDeleteGroup(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	tag := uniqueTag(t)

	groupID := createTestGroup(t, db, "g-"+tag, []string{rbac.PermissionUsersView})
	createTestUser(t, db, groupID)
	if err := service.DeleteGroup(ctx, groupID.String()); !errors.Is(err, apierror.ErrGroupNotEmpty) {
		t.Fatalf("non-empty delete error = %v, want apierror.ErrGroupNotEmpty", err)
	}

	emptyGroupID := createTestGroup(t, db, "e-"+tag, []string{rbac.PermissionUsersView})
	if err := service.DeleteGroup(ctx, emptyGroupID.String()); err != nil {
		t.Fatalf("empty delete: %v", err)
	}

	if err := service.DeleteGroup(ctx, uuid.New().String()); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("missing group delete error = %v, want apierror.ErrNotFound", err)
	}
	if err := service.DeleteGroup(ctx, "not-a-uuid"); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("invalid id delete error = %v, want apierror.ErrNotFound", err)
	}
}

func TestDeleteUser(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	owner, ownerID := createOwnerIdentity(t, db)

	if err := service.DeleteUser(ctx, owner, ownerID.String()); !errors.Is(err, apierror.ErrCannotModifySelf) {
		t.Fatalf("self delete error = %v, want apierror.ErrCannotModifySelf", err)
	}

	targetID := createTestUser(t, db, rbac.GroupIDUser)
	addUserPermissions(t, db, targetID, []string{rbac.PermissionUsersView})
	if err := service.DeleteUser(ctx, owner, targetID.String()); err != nil {
		t.Fatalf("delete plain user: %v", err)
	}
	if _, err := service.GetUser(ctx, targetID.String()); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("deleted user lookup error = %v, want apierror.ErrNotFound", err)
	}
	orphans, err := db.Client.UserPermission.Query().
		Where(userpermission.UserIDEQ(targetID)).
		Count(ctx)
	if err != nil {
		t.Fatalf("count orphan permissions: %v", err)
	}
	if orphans != 0 {
		t.Fatalf("orphan permission rows = %d, want 0", orphans)
	}

	managerGroupID := createTestGroup(t, db, "mgr-"+uniqueTag(t), []string{rbac.PermissionUsersEdit})
	managerID := createTestUser(t, db, managerGroupID)
	manager := identityFor(t, db, managerID)

	plainID := createTestUser(t, db, rbac.GroupIDUser)
	if err := service.DeleteUser(ctx, manager, plainID.String()); err != nil {
		t.Fatalf("manager deleting plain user: %v", err)
	}

	adminID := createTestUser(t, db, rbac.GroupIDAdmin)
	if err := service.DeleteUser(ctx, manager, adminID.String()); !errors.Is(err, apierror.ErrOwnerProtected) {
		t.Fatalf("non-owner deleting admin error = %v, want apierror.ErrOwnerProtected", err)
	}
	if err := service.DeleteUser(ctx, manager, ownerID.String()); !errors.Is(err, apierror.ErrOwnerProtected) {
		t.Fatalf("non-owner deleting owner error = %v, want apierror.ErrOwnerProtected", err)
	}
	if err := service.DeleteUser(ctx, owner, uuid.New().String()); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("missing user delete error = %v, want apierror.ErrNotFound", err)
	}
	if err := service.DeleteUser(ctx, owner, "not-a-uuid"); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("invalid id delete error = %v, want apierror.ErrNotFound", err)
	}
}

func TestDeleteUsers(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	owner, ownerID := createOwnerIdentity(t, db)

	firstID := createTestUser(t, db, rbac.GroupIDUser)
	secondID := createTestUser(t, db, rbac.GroupIDUser)
	addUserPermissions(t, db, firstID, []string{rbac.PermissionUsersView})
	if err := service.DeleteUsers(ctx, owner, []string{firstID.String(), secondID.String(), firstID.String()}); err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	for _, id := range []uuid.UUID{firstID, secondID} {
		if _, err := service.GetUser(ctx, id.String()); !errors.Is(err, apierror.ErrNotFound) {
			t.Fatalf("deleted user %s lookup error = %v, want apierror.ErrNotFound", id, err)
		}
	}

	keepID := createTestUser(t, db, rbac.GroupIDUser)
	if err := service.DeleteUsers(ctx, owner, []string{keepID.String(), ownerID.String()}); !errors.Is(err, apierror.ErrCannotModifySelf) {
		t.Fatalf("self batch error = %v, want apierror.ErrCannotModifySelf", err)
	}
	if _, err := service.GetUser(ctx, keepID.String()); err != nil {
		t.Fatalf("batch containing self deleted a bystander: %v", err)
	}

	if err := service.DeleteUsers(ctx, owner, []string{keepID.String(), uuid.New().String()}); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("missing member error = %v, want apierror.ErrNotFound", err)
	}
	if _, err := service.GetUser(ctx, keepID.String()); err != nil {
		t.Fatalf("batch containing a missing id deleted an existing user: %v", err)
	}

	if err := service.DeleteUsers(ctx, owner, nil); !errors.Is(err, apierror.ErrInvalidRequest) {
		t.Fatalf("empty batch error = %v, want apierror.ErrInvalidRequest", err)
	}

	managerGroupID := createTestGroup(t, db, "mgr-"+uniqueTag(t), []string{rbac.PermissionUsersEdit})
	managerID := createTestUser(t, db, managerGroupID)
	manager := identityFor(t, db, managerID)
	adminID := createTestUser(t, db, rbac.GroupIDAdmin)
	if err := service.DeleteUsers(ctx, manager, []string{keepID.String(), adminID.String()}); !errors.Is(err, apierror.ErrOwnerProtected) {
		t.Fatalf("privileged batch error = %v, want apierror.ErrOwnerProtected", err)
	}
	if _, err := service.GetUser(ctx, keepID.String()); err != nil {
		t.Fatalf("privileged batch deleted a plain user: %v", err)
	}
	if err := service.DeleteUsers(ctx, manager, []string{keepID.String()}); err != nil {
		t.Fatalf("manager batch delete: %v", err)
	}
}

func TestUpdateUser(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	owner, ownerID := createOwnerIdentity(t, db)
	tag := uniqueTag(t)

	if _, err := service.UpdateUser(ctx, owner, ownerID.String(), UpdateUserInput{GroupID: ptr(rbac.GroupIDUser.String())}); !errors.Is(err, apierror.ErrCannotModifySelf) {
		t.Fatalf("self modification error = %v, want apierror.ErrCannotModifySelf", err)
	}

	targetID := createTestUser(t, db, rbac.GroupIDUser)
	updated, err := service.UpdateUser(ctx, owner, targetID.String(), UpdateUserInput{
		GroupID:          ptr(rbac.GroupIDAdmin.String()),
		ExtraPermissions: ptr([]string{rbac.PermissionUsersView}),
	})
	if err != nil {
		t.Fatalf("owner moves user to admin group: %v", err)
	}
	if updated.Group.SystemKey == nil || *updated.Group.SystemKey != rbac.GroupKeyAdmin {
		t.Fatalf("group = %+v, want admin preset", updated.Group)
	}

	if _, err := service.UpdateUser(ctx, owner, ownerID.String(), UpdateUserInput{ExtraPermissions: ptr([]string{rbac.PermissionUsersView})}); !errors.Is(err, apierror.ErrCannotModifySelf) {
		t.Fatalf("self extra error = %v, want apierror.ErrCannotModifySelf", err)
	}

	managerGroupID := createTestGroup(t, db, "mgr-"+tag, []string{rbac.PermissionUsersEdit})
	managerID := createTestUser(t, db, managerGroupID)
	manager := identityFor(t, db, managerID)

	if _, err := service.UpdateUser(ctx, manager, ownerID.String(), UpdateUserInput{ExtraPermissions: ptr([]string{rbac.PermissionUsersView})}); !errors.Is(err, apierror.ErrOwnerProtected) {
		t.Fatalf("non-owner touching owner error = %v, want apierror.ErrOwnerProtected", err)
	}
	if _, err := service.UpdateUser(ctx, manager, targetID.String(), UpdateUserInput{GroupID: ptr(rbac.GroupIDOwner.String())}); !errors.Is(err, apierror.ErrOwnerProtected) {
		t.Fatalf("non-owner promoting to owner error = %v, want apierror.ErrOwnerProtected", err)
	}

	richGroupID := createTestGroup(t, db, "rich-"+tag, []string{rbac.PermissionGroupsEdit})
	transferTargetID := createTestUser(t, db, rbac.GroupIDUser)
	if _, err := service.UpdateUser(ctx, manager, transferTargetID.String(), UpdateUserInput{GroupID: ptr(richGroupID.String())}); !errors.Is(err, apierror.ErrGrantForbidden) {
		t.Fatalf("group transfer grant error = %v, want apierror.ErrGrantForbidden", err)
	}
	if _, err := service.UpdateUser(ctx, manager, transferTargetID.String(), UpdateUserInput{
		GroupID:          ptr(richGroupID.String()),
		ExtraPermissions: ptr([]string{rbac.PermissionUsersView}),
	}); !errors.Is(err, apierror.ErrGrantForbidden) {
		t.Fatalf("combined grant error = %v, want apierror.ErrGrantForbidden", err)
	}

	limitedGroupID := createTestGroup(t, db, "part-"+tag, []string{rbac.PermissionUsersView})
	limitedID := createTestUser(t, db, limitedGroupID)
	if _, err := service.UpdateUser(ctx, manager, limitedID.String(), UpdateUserInput{
		ExtraPermissions: ptr([]string{rbac.PermissionUsersView}),
	}); err != nil {
		t.Fatalf("granted extra permission: %v", err)
	}
	removed, err := service.UpdateUser(ctx, manager, limitedID.String(), UpdateUserInput{
		ExtraPermissions: ptr([]string{}),
	})
	if err != nil {
		t.Fatalf("remove extra permission: %v", err)
	}
	if len(removed.ExtraPermissions) != 0 {
		t.Fatalf("extra permissions = %v, want empty", removed.ExtraPermissions)
	}

	if _, err := service.UpdateUser(ctx, manager, limitedID.String(), UpdateUserInput{
		ExtraPermissions: ptr([]string{"content.posts.edit"}),
	}); !errors.Is(err, apierror.ErrInvalidPermission) {
		t.Fatalf("unknown key error = %v, want apierror.ErrInvalidPermission", err)
	}
	if _, err := service.UpdateUser(ctx, owner, limitedID.String(), UpdateUserInput{
		GroupID: ptr(uuid.New().String()),
	}); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("missing group error = %v, want apierror.ErrNotFound", err)
	}
	if _, err := service.UpdateUser(ctx, owner, uuid.New().String(), UpdateUserInput{
		ExtraPermissions: ptr([]string{}),
	}); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("missing user error = %v, want apierror.ErrNotFound", err)
	}
}

func TestLastOwnerConcurrent(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()

	firstID := createTestUser(t, db, rbac.GroupIDOwner)
	secondID := createTestUser(t, db, rbac.GroupIDOwner)
	first := identityFor(t, db, firstID)
	second := identityFor(t, db, secondID)

	results := make([]error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		_, results[0] = service.UpdateUser(ctx, second, firstID.String(), UpdateUserInput{GroupID: ptr(rbac.GroupIDUser.String())})
	}()
	go func() {
		defer waitGroup.Done()
		_, results[1] = service.UpdateUser(ctx, first, secondID.String(), UpdateUserInput{GroupID: ptr(rbac.GroupIDUser.String())})
	}()
	waitGroup.Wait()

	lastOwnerErrors := 0
	for _, err := range results {
		switch {
		case err == nil:
		case errors.Is(err, apierror.ErrLastOwner):
			lastOwnerErrors++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if lastOwnerErrors != 1 {
		t.Fatalf("last owner errors = %d, want exactly 1 (results: %v)", lastOwnerErrors, results)
	}
}

func TestDeleteLastOwnerConcurrent(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()

	firstID := createTestUser(t, db, rbac.GroupIDOwner)
	secondID := createTestUser(t, db, rbac.GroupIDOwner)
	first := identityFor(t, db, firstID)
	second := identityFor(t, db, secondID)

	results := make([]error, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		results[0] = service.DeleteUser(ctx, second, firstID.String())
	}()
	go func() {
		defer waitGroup.Done()
		results[1] = service.DeleteUser(ctx, first, secondID.String())
	}()
	waitGroup.Wait()

	lastOwnerErrors := 0
	for _, err := range results {
		switch {
		case err == nil:
		case errors.Is(err, apierror.ErrLastOwner):
			lastOwnerErrors++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if lastOwnerErrors != 1 {
		t.Fatalf("last owner errors = %d, want exactly 1 (results: %v)", lastOwnerErrors, results)
	}
}

func TestEffectiveUnionRevoke(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()
	owner, _ := createOwnerIdentity(t, db)

	groupID := createTestGroup(t, db, "g-"+uniqueTag(t), []string{rbac.PermissionUsersView})
	userID := createTestUser(t, db, groupID)
	addUserPermissions(t, db, userID, []string{rbac.PermissionUsersView})

	before := identityFor(t, db, userID).Permissions
	if _, err := service.UpdateUser(ctx, owner, userID.String(), UpdateUserInput{ExtraPermissions: ptr([]string{})}); err != nil {
		t.Fatalf("clear extra permissions: %v", err)
	}
	after := identityFor(t, db, userID).Permissions
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("effective permissions changed from %v to %v after removing a group-granted extra", before, after)
	}
}

func TestListUsersAndGetUser(t *testing.T) {
	db := testDB(t)
	service := newService(db)
	ctx := context.Background()

	groupID := createTestGroup(t, db, "g-"+uniqueTag(t), []string{rbac.PermissionGroupsEdit})
	userID := createTestUser(t, db, groupID)
	addUserPermissions(t, db, userID, []string{rbac.PermissionUsersView})

	users, err := service.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	var found *UserSummary
	for i := range users {
		if users[i].ID == userID.String() {
			found = &users[i]
		}
	}
	if found == nil {
		t.Fatalf("user %s missing from list", userID)
	}
	if found.Group.ID != groupID.String() || found.Group.SystemKey != nil {
		t.Fatalf("group = %+v, want custom group %s without system key", found.Group, groupID)
	}
	if !reflect.DeepEqual(found.ExtraPermissions, []string{rbac.PermissionUsersView}) {
		t.Fatalf("extra permissions = %v, want [users.view]", found.ExtraPermissions)
	}
	if found.EmailVerificationStatus != "not_required" {
		t.Fatalf("email verification status = %q, want not_required", found.EmailVerificationStatus)
	}
	for i := 1; i < len(users); i++ {
		if users[i-1].Email == users[i].Email {
			t.Fatalf("duplicate user rows at %d", i)
		}
	}

	detail, err := service.GetUser(ctx, userID.String())
	if err != nil {
		t.Fatalf("get user: %v", err)
	}
	want := []string{rbac.PermissionGroupsEdit, rbac.PermissionUsersView, rbac.PermissionGroupsView}
	if !reflect.DeepEqual(detail.Permissions, want) {
		t.Fatalf("effective permissions = %v, want %v", detail.Permissions, want)
	}
	if detail.EmailVerificationStatus != "not_required" {
		t.Fatalf("detail email verification status = %q, want not_required", detail.EmailVerificationStatus)
	}
	if _, err := service.GetUser(ctx, uuid.New().String()); !errors.Is(err, apierror.ErrNotFound) {
		t.Fatalf("missing user error = %v, want apierror.ErrNotFound", err)
	}
}
