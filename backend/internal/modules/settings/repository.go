package settings

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"entgo.io/ent/dialect/sql"

	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/settingvalue"
)

type repository struct {
	client *ent.Client
}

func (r *repository) upsert(ctx context.Context, group, name string, value json.RawMessage) error {
	return r.client.SettingValue.Create().
		SetSettingGroup(group).
		SetName(name).
		SetValue(value).
		SetUpdatedAt(time.Now()).
		OnConflict(sql.ConflictColumns("setting_group", "name")).
		UpdateNewValues().
		Exec(ctx)
}

func (r *repository) upsertMany(ctx context.Context, group string, values map[string]json.RawMessage) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	now := time.Now()
	for _, name := range names {
		if err := tx.SettingValue.Create().
			SetSettingGroup(group).
			SetName(name).
			SetValue(values[name]).
			SetUpdatedAt(now).
			OnConflict(sql.ConflictColumns("setting_group", "name")).
			UpdateNewValues().
			Exec(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *repository) get(ctx context.Context, group, name string) (json.RawMessage, bool, error) {
	row, err := r.client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ(group), settingvalue.NameEQ(name)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return row.Value, true, nil
}

func (r *repository) listByGroup(ctx context.Context, group string) ([]*ent.SettingValue, error) {
	return r.client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ(group)).
		Order(settingvalue.ByName()).
		All(ctx)
}

func (r *repository) delete(ctx context.Context, group, name string) (int, error) {
	return r.client.SettingValue.Delete().
		Where(settingvalue.SettingGroupEQ(group), settingvalue.NameEQ(name)).
		Exec(ctx)
}
