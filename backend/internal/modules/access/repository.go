package access

import (
	"context"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/group"
	"website/backend/internal/database/ent/grouppermission"
	"website/backend/internal/database/ent/user"
	"website/backend/internal/database/ent/userpermission"
)

type groupRecord struct {
	ID        uuid.UUID
	SystemKey string
	Name      string
}

type userRecord struct {
	ID          uuid.UUID
	Email       string
	Nickname    string
	AuthVersion int
	Group       groupRecord
}

type updateUserParams struct {
	UserID         uuid.UUID
	GroupID        uuid.UUID
	GroupChanged   bool
	ReplaceExtra   bool
	Extra          []string
	LockOwnerCount bool
}

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
}

func toGroupRecord(row *ent.Group) groupRecord {
	return groupRecord{ID: row.ID, SystemKey: row.SystemKey, Name: row.Name}
}

func toUserRecord(row *ent.User) userRecord {
	record := userRecord{ID: row.ID, Email: row.Email, Nickname: row.Nickname, AuthVersion: row.AuthVersion}
	if row.Edges.Group != nil {
		record.Group = toGroupRecord(row.Edges.Group)
	}
	return record
}

func (r *repository) findUser(ctx context.Context, id uuid.UUID) (userRecord, error) {
	row, err := r.client.User.Query().Where(user.IDEQ(id)).WithGroup().Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return userRecord{}, apierror.ErrNotFound
		}
		return userRecord{}, err
	}
	return toUserRecord(row), nil
}

func (r *repository) findUsers(ctx context.Context, ids []uuid.UUID) ([]userRecord, error) {
	rows, err := r.client.User.Query().Where(user.IDIn(ids...)).WithGroup().All(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]userRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, toUserRecord(row))
	}
	return records, nil
}

func (r *repository) findGroup(ctx context.Context, id uuid.UUID) (groupRecord, error) {
	row, err := r.client.Group.Query().Where(group.IDEQ(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return groupRecord{}, apierror.ErrNotFound
		}
		return groupRecord{}, err
	}
	return toGroupRecord(row), nil
}

func (r *repository) groupPermissions(ctx context.Context, id uuid.UUID) ([]string, error) {
	rows, err := r.client.GroupPermission.Query().Where(grouppermission.GroupIDEQ(id)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Permission)
	}
	return out, nil
}

func (r *repository) userPermissions(ctx context.Context, id uuid.UUID) ([]string, error) {
	rows, err := r.client.UserPermission.Query().Where(userpermission.UserIDEQ(id)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Permission)
	}
	return out, nil
}

func (r *repository) listGroups(ctx context.Context) ([]groupRecord, map[uuid.UUID]int, map[uuid.UUID][]string, error) {
	rows, err := r.client.Group.Query().Order(ent.Asc(group.FieldCreatedAt)).All(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	records := make([]groupRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, toGroupRecord(row))
	}

	var counts []struct {
		GroupID uuid.UUID `json:"group_id"`
		Count   int       `json:"count"`
	}
	if err := r.client.User.Query().
		GroupBy(user.FieldGroupID).
		Aggregate(ent.Count()).
		Scan(ctx, &counts); err != nil {
		return nil, nil, nil, err
	}
	countByGroup := make(map[uuid.UUID]int, len(counts))
	for _, row := range counts {
		countByGroup[row.GroupID] = row.Count
	}

	permissionRows, err := r.client.GroupPermission.Query().All(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	permissionsByGroup := make(map[uuid.UUID][]string)
	for _, row := range permissionRows {
		permissionsByGroup[row.GroupID] = append(permissionsByGroup[row.GroupID], row.Permission)
	}
	return records, countByGroup, permissionsByGroup, nil
}

func (r *repository) listUsers(ctx context.Context) ([]userRecord, map[uuid.UUID][]string, error) {
	rows, err := r.client.User.Query().Order(ent.Desc(user.FieldCreatedAt)).WithGroup().All(ctx)
	if err != nil {
		return nil, nil, err
	}
	records := make([]userRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, toUserRecord(row))
	}

	permissionRows, err := r.client.UserPermission.Query().All(ctx)
	if err != nil {
		return nil, nil, err
	}
	permissionsByUser := make(map[uuid.UUID][]string)
	for _, row := range permissionRows {
		permissionsByUser[row.UserID] = append(permissionsByUser[row.UserID], row.Permission)
	}
	return records, permissionsByUser, nil
}

func (r *repository) memberCount(ctx context.Context, id uuid.UUID) (int, error) {
	return r.client.User.Query().Where(user.GroupIDEQ(id)).Count(ctx)
}

func (r *repository) createGroup(ctx context.Context, name string, permissions []string) (groupRecord, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return groupRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	created, err := tx.Group.Create().SetName(name).Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return groupRecord{}, apierror.ErrGroupNameTaken
		}
		return groupRecord{}, err
	}
	if len(permissions) > 0 {
		builders := make([]*ent.GroupPermissionCreate, 0, len(permissions))
		for _, permission := range permissions {
			builders = append(builders, tx.GroupPermission.Create().
				SetGroupID(created.ID).
				SetPermission(permission))
		}
		if _, err := tx.GroupPermission.CreateBulk(builders...).Save(ctx); err != nil {
			return groupRecord{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return groupRecord{}, err
	}
	return toGroupRecord(created), nil
}

func (r *repository) updateGroupName(ctx context.Context, id uuid.UUID, name string) error {
	if err := r.client.Group.UpdateOneID(id).SetName(name).Exec(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return apierror.ErrGroupNameTaken
		}
		return err
	}
	return nil
}

func (r *repository) replaceGroupPermissions(ctx context.Context, id uuid.UUID, permissions []string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	locked, err := tx.Group.Query().Where(group.IDEQ(id)).ForUpdate().All(ctx)
	if err != nil {
		return err
	}
	if len(locked) == 0 {
		return apierror.ErrNotFound
	}

	if _, err := tx.GroupPermission.Delete().Where(grouppermission.GroupIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	if len(permissions) > 0 {
		builders := make([]*ent.GroupPermissionCreate, 0, len(permissions))
		for _, permission := range permissions {
			builders = append(builders, tx.GroupPermission.Create().
				SetGroupID(id).
				SetPermission(permission))
		}
		if _, err := tx.GroupPermission.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *repository) deleteGroup(ctx context.Context, id uuid.UUID) error {
	if err := r.client.Group.DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsConstraintError(err) {
			return apierror.ErrGroupNotEmpty
		}
		return err
	}
	return nil
}

func (r *repository) deleteUsers(ctx context.Context, ids []uuid.UUID, lockOwnerCount bool) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if lockOwnerCount {
		owners, err := tx.User.Query().
			Where(user.GroupIDEQ(rbac.GroupIDOwner)).
			ForUpdate().
			All(ctx)
		if err != nil {
			return err
		}
		removed := make(map[uuid.UUID]struct{}, len(ids))
		for _, id := range ids {
			removed[id] = struct{}{}
		}
		remaining := 0
		for _, owner := range owners {
			if _, ok := removed[owner.ID]; !ok {
				remaining++
			}
		}
		if remaining < 1 {
			return apierror.ErrLastOwner
		}
	} else {
		locked, err := tx.User.Query().
			Where(user.IDIn(ids...)).
			ForUpdate().
			All(ctx)
		if err != nil {
			return err
		}
		if len(locked) != len(ids) {
			return apierror.ErrNotFound
		}
	}

	if _, err := tx.User.Delete().Where(user.IDIn(ids...)).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *repository) updateUser(ctx context.Context, params updateUserParams) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if params.LockOwnerCount {
		owners, err := tx.User.Query().
			Where(user.GroupIDEQ(rbac.GroupIDOwner)).
			ForUpdate().
			All(ctx)
		if err != nil {
			return err
		}
		if len(owners) < 2 {
			return apierror.ErrLastOwner
		}
	} else {
		locked, err := tx.User.Query().
			Where(user.IDEQ(params.UserID)).
			ForUpdate().
			All(ctx)
		if err != nil {
			return err
		}
		if len(locked) == 0 {
			return apierror.ErrNotFound
		}
	}

	if params.GroupChanged {
		if err := tx.User.UpdateOneID(params.UserID).SetGroupID(params.GroupID).Exec(ctx); err != nil {
			if ent.IsConstraintError(err) {
				return apierror.ErrNotFound
			}
			return err
		}
	}

	if params.ReplaceExtra {
		if _, err := tx.UserPermission.Delete().Where(userpermission.UserIDEQ(params.UserID)).Exec(ctx); err != nil {
			return err
		}
		if len(params.Extra) > 0 {
			builders := make([]*ent.UserPermissionCreate, 0, len(params.Extra))
			for _, permission := range params.Extra {
				builders = append(builders, tx.UserPermission.Create().
					SetUserID(params.UserID).
					SetPermission(permission))
			}
			if _, err := tx.UserPermission.CreateBulk(builders...).Save(ctx); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
