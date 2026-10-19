package access

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
)

const maxGroupNameLength = 64

type Service struct {
	repo *repository
}

func newService(db *database.Database) *Service {
	return &Service{repo: newRepository(db)}
}

func (s *Service) IdentityByUserID(ctx context.Context, userID string) (guard.Identity, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return guard.Identity{}, guard.ErrUserNotFound
	}
	record, err := s.repo.findUser(ctx, id)
	if err != nil {
		if errors.Is(err, apierror.ErrNotFound) {
			return guard.Identity{}, guard.ErrUserNotFound
		}
		return guard.Identity{}, err
	}
	permissions, err := s.effectivePermissions(ctx, record)
	if err != nil {
		return guard.Identity{}, err
	}
	return guard.Identity{
		UserID:      record.ID.String(),
		AuthVersion: record.AuthVersion,
		GroupID:     record.Group.ID.String(),
		GroupKey:    record.Group.SystemKey,
		GroupName:   record.Group.Name,
		Permissions: permissions,
	}, nil
}

func (s *Service) ListGroups(ctx context.Context) ([]Group, error) {
	records, counts, permissionsByGroup, err := s.repo.listGroups(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]Group, 0, len(records))
	for _, record := range records {
		groupPermissions := permissionsByGroup[record.ID]
		if record.SystemKey != "" {
			groupPermissions = rbac.PresetPermissions(record.SystemKey)
		}
		groups = append(groups, Group{
			ID:          record.ID.String(),
			SystemKey:   optionalString(record.SystemKey),
			Name:        record.Name,
			IsPreset:    record.SystemKey != "",
			MemberCount: counts[record.ID],
			Permissions: rbac.Expand(groupPermissions),
		})
	}
	return groups, nil
}

func (s *Service) CreateGroup(ctx context.Context, operator guard.Identity, name string, permissions []string) (Group, error) {
	normalizedName, err := normalizeGroupName(name)
	if err != nil {
		return Group{}, err
	}
	normalizedPermissions, err := normalizePermissions(permissions)
	if err != nil {
		return Group{}, err
	}
	if err := assertGrantable(operator, normalizedPermissions); err != nil {
		return Group{}, err
	}

	record, err := s.repo.createGroup(ctx, normalizedName, normalizedPermissions)
	if err != nil {
		return Group{}, err
	}
	return s.groupDetail(ctx, record.ID)
}

func (s *Service) UpdateGroup(ctx context.Context, operator guard.Identity, groupID string, input UpdateGroupInput) (Group, error) {
	id, err := uuid.Parse(groupID)
	if err != nil {
		return Group{}, apierror.ErrNotFound
	}
	record, err := s.repo.findGroup(ctx, id)
	if err != nil {
		return Group{}, err
	}
	if record.SystemKey != "" {
		return Group{}, apierror.ErrPresetGroupLocked
	}

	var nextName *string
	if input.Name != nil {
		normalized, err := normalizeGroupName(*input.Name)
		if err != nil {
			return Group{}, err
		}
		nextName = &normalized
	}

	var nextPermissions *[]string
	if input.Permissions != nil {
		normalized, err := normalizePermissions(*input.Permissions)
		if err != nil {
			return Group{}, err
		}
		nextPermissions = &normalized
	}

	if nextPermissions != nil {
		current, err := s.repo.groupPermissions(ctx, id)
		if err != nil {
			return Group{}, err
		}
		added := difference(rbac.Expand(*nextPermissions), rbac.Expand(current))
		if err := assertGrantable(operator, added); err != nil {
			return Group{}, err
		}
	}

	if nextName != nil {
		if err := s.repo.updateGroupName(ctx, id, *nextName); err != nil {
			return Group{}, err
		}
	}
	if nextPermissions != nil {
		if err := s.repo.replaceGroupPermissions(ctx, id, *nextPermissions); err != nil {
			return Group{}, err
		}
	}
	return s.groupDetail(ctx, id)
}

func (s *Service) DeleteGroup(ctx context.Context, groupID string) error {
	id, err := uuid.Parse(groupID)
	if err != nil {
		return apierror.ErrNotFound
	}
	record, err := s.repo.findGroup(ctx, id)
	if err != nil {
		return err
	}
	if record.SystemKey != "" {
		return apierror.ErrPresetGroupLocked
	}
	count, err := s.repo.memberCount(ctx, id)
	if err != nil {
		return err
	}
	if count > 0 {
		return apierror.ErrGroupNotEmpty
	}
	return s.repo.deleteGroup(ctx, id)
}

func (s *Service) ListUsers(ctx context.Context) ([]UserSummary, error) {
	records, permissionsByUser, err := s.repo.listUsers(ctx)
	if err != nil {
		return nil, err
	}
	users := make([]UserSummary, 0, len(records))
	for _, record := range records {
		users = append(users, UserSummary{
			ID:               record.ID.String(),
			Email:            record.Email,
			Nickname:         record.Nickname,
			Group:            groupRef(record.Group),
			ExtraPermissions: rbac.Expand(permissionsByUser[record.ID]),
		})
	}
	return users, nil
}

func (s *Service) GetUser(ctx context.Context, userID string) (UserDetail, error) {
	id, err := uuid.Parse(userID)
	if err != nil {
		return UserDetail{}, apierror.ErrNotFound
	}
	record, err := s.repo.findUser(ctx, id)
	if err != nil {
		return UserDetail{}, err
	}
	extra, err := s.repo.userPermissions(ctx, id)
	if err != nil {
		return UserDetail{}, err
	}
	permissions, err := s.effectivePermissions(ctx, record)
	if err != nil {
		return UserDetail{}, err
	}
	return UserDetail{
		UserSummary: UserSummary{
			ID:               record.ID.String(),
			Email:            record.Email,
			Nickname:         record.Nickname,
			Group:            groupRef(record.Group),
			ExtraPermissions: rbac.Expand(extra),
		},
		Permissions: permissions,
	}, nil
}

func (s *Service) UpdateUser(ctx context.Context, operator guard.Identity, userID string, input UpdateUserInput) (UserDetail, error) {
	operatorID, err := uuid.Parse(operator.UserID)
	if err != nil {
		return UserDetail{}, apierror.ErrNotFound
	}
	targetID, err := uuid.Parse(userID)
	if err != nil {
		return UserDetail{}, apierror.ErrNotFound
	}
	if operatorID == targetID {
		return UserDetail{}, apierror.ErrCannotModifySelf
	}

	target, err := s.repo.findUser(ctx, targetID)
	if err != nil {
		return UserDetail{}, err
	}

	nextGroup := target.Group
	groupChanged := false
	if input.GroupID != nil {
		groupID, err := uuid.Parse(*input.GroupID)
		if err != nil {
			return UserDetail{}, apierror.ErrNotFound
		}
		record, err := s.repo.findGroup(ctx, groupID)
		if err != nil {
			return UserDetail{}, err
		}
		nextGroup = record
		groupChanged = record.ID != target.Group.ID
	}

	if isPrivilegedGroupKey(target.Group.SystemKey) || isPrivilegedGroupKey(nextGroup.SystemKey) {
		if operator.GroupKey != rbac.GroupKeyOwner {
			return UserDetail{}, apierror.ErrOwnerProtected
		}
	}

	nextExtra := []string{}
	replaceExtra := false
	if input.ExtraPermissions != nil {
		normalized, err := normalizePermissions(*input.ExtraPermissions)
		if err != nil {
			return UserDetail{}, err
		}
		nextExtra = normalized
		replaceExtra = true
	}

	currentExtra, err := s.repo.userPermissions(ctx, targetID)
	if err != nil {
		return UserDetail{}, err
	}
	currentGroupPermissions, err := s.permissionsOfGroup(ctx, target.Group)
	if err != nil {
		return UserDetail{}, err
	}
	nextGroupPermissions, err := s.permissionsOfGroup(ctx, nextGroup)
	if err != nil {
		return UserDetail{}, err
	}

	current := rbac.Expand(append(currentGroupPermissions, currentExtra...))
	nextExtrasForDiff := currentExtra
	if replaceExtra {
		nextExtrasForDiff = nextExtra
	}
	next := rbac.Expand(append(nextGroupPermissions, nextExtrasForDiff...))
	if err := assertGrantable(operator, difference(next, current)); err != nil {
		return UserDetail{}, err
	}

	params := updateUserParams{
		UserID:         targetID,
		GroupID:        nextGroup.ID,
		GroupChanged:   groupChanged,
		ReplaceExtra:   replaceExtra,
		Extra:          nextExtra,
		LockOwnerCount: target.Group.SystemKey == rbac.GroupKeyOwner && nextGroup.SystemKey != rbac.GroupKeyOwner,
	}
	if err := s.repo.updateUser(ctx, params); err != nil {
		return UserDetail{}, err
	}
	return s.GetUser(ctx, userID)
}

func (s *Service) DeleteUser(ctx context.Context, operator guard.Identity, userID string) error {
	return s.DeleteUsers(ctx, operator, []string{userID})
}

func (s *Service) DeleteUsers(ctx context.Context, operator guard.Identity, ids []string) error {
	operatorID, err := uuid.Parse(operator.UserID)
	if err != nil {
		return apierror.ErrNotFound
	}

	targets := make([]uuid.UUID, 0, len(ids))
	seen := make(map[uuid.UUID]struct{}, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(raw)
		if err != nil {
			return apierror.ErrNotFound
		}
		if id == operatorID {
			return apierror.ErrCannotModifySelf
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return apierror.ErrInvalidRequest
	}

	rows, err := s.repo.findUsers(ctx, targets)
	if err != nil {
		return err
	}
	if len(rows) != len(targets) {
		return apierror.ErrNotFound
	}

	lockOwnerCount := false
	for _, row := range rows {
		if isPrivilegedGroupKey(row.Group.SystemKey) && operator.GroupKey != rbac.GroupKeyOwner {
			return apierror.ErrOwnerProtected
		}
		if row.Group.SystemKey == rbac.GroupKeyOwner {
			lockOwnerCount = true
		}
	}
	return s.repo.deleteUsers(ctx, targets, lockOwnerCount)
}

func (s *Service) groupDetail(ctx context.Context, id uuid.UUID) (Group, error) {
	record, err := s.repo.findGroup(ctx, id)
	if err != nil {
		return Group{}, err
	}
	count, err := s.repo.memberCount(ctx, id)
	if err != nil {
		return Group{}, err
	}
	permissions, err := s.permissionsOfGroup(ctx, record)
	if err != nil {
		return Group{}, err
	}
	return Group{
		ID:          record.ID.String(),
		SystemKey:   optionalString(record.SystemKey),
		Name:        record.Name,
		IsPreset:    record.SystemKey != "",
		MemberCount: count,
		Permissions: rbac.Expand(permissions),
	}, nil
}

func (s *Service) effectivePermissions(ctx context.Context, record userRecord) ([]string, error) {
	groupPermissions, err := s.permissionsOfGroup(ctx, record.Group)
	if err != nil {
		return nil, err
	}
	extra, err := s.repo.userPermissions(ctx, record.ID)
	if err != nil {
		return nil, err
	}
	combined := make([]string, 0, len(groupPermissions)+len(extra))
	combined = append(combined, groupPermissions...)
	combined = append(combined, extra...)
	return rbac.Expand(combined), nil
}

func (s *Service) permissionsOfGroup(ctx context.Context, group groupRecord) ([]string, error) {
	if group.SystemKey != "" {
		return rbac.PresetPermissions(group.SystemKey), nil
	}
	return s.repo.groupPermissions(ctx, group.ID)
}

func groupRef(group groupRecord) GroupRef {
	return GroupRef{
		ID:        group.ID.String(),
		SystemKey: optionalString(group.SystemKey),
		Name:      group.Name,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func normalizeGroupName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || len([]rune(trimmed)) > maxGroupNameLength {
		return "", apierror.ErrInvalidRequest
	}
	return trimmed, nil
}

func normalizePermissions(permissions []string) ([]string, error) {
	out := make([]string, 0, len(permissions))
	seen := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		if !rbac.IsValid(permission) {
			return nil, apierror.ErrInvalidPermission
		}
		if _, ok := seen[permission]; ok {
			continue
		}
		seen[permission] = struct{}{}
		out = append(out, permission)
	}
	return out, nil
}

func assertGrantable(operator guard.Identity, added []string) error {
	for _, permission := range rbac.Expand(added) {
		if !slices.Contains(operator.Permissions, permission) {
			return apierror.ErrGrantForbidden
		}
	}
	return nil
}

func difference(next []string, current []string) []string {
	held := make(map[string]struct{}, len(current))
	for _, key := range current {
		held[key] = struct{}{}
	}
	out := make([]string, 0, len(next))
	for _, key := range next {
		if _, ok := held[key]; ok {
			continue
		}
		out = append(out, key)
	}
	return out
}

func isPrivilegedGroupKey(key string) bool {
	return key == rbac.GroupKeyOwner || key == rbac.GroupKeyAdmin
}
