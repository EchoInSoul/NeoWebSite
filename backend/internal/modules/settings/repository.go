package settings

import (
	"context"
	"encoding/json"
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
