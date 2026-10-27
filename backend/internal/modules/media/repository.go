package media

import (
	"context"

	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/mediaasset"
)

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
}

func (r *repository) Insert(ctx context.Context, asset mediaAsset) error {
	_, err := r.client.MediaAsset.Create().
		SetID(asset.ID).
		SetProvider(asset.Provider).
		SetKey(asset.Key).
		SetSizeBytes(asset.SizeBytes).
		SetOriginalName(asset.OriginalName).
		SetNillableUploadedBy(asset.UploadedBy).
		SetCreatedAt(asset.CreatedAt).
		Save(ctx)
	return err
}

func (r *repository) Get(ctx context.Context, id uuid.UUID) (mediaAsset, error) {
	row, err := r.client.MediaAsset.Query().Where(mediaasset.IDEQ(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return mediaAsset{}, errNotFound
		}
		return mediaAsset{}, err
	}
	return toMediaAsset(row), nil
}

func (r *repository) Delete(ctx context.Context, id uuid.UUID) (bool, error) {
	deleted, err := r.client.MediaAsset.Delete().Where(mediaasset.IDEQ(id)).Exec(ctx)
	if err != nil {
		return false, err
	}
	return deleted > 0, nil
}

func toMediaAsset(row *ent.MediaAsset) mediaAsset {
	return mediaAsset{
		ID:           row.ID,
		Provider:     row.Provider,
		Key:          row.Key,
		SizeBytes:    row.SizeBytes,
		OriginalName: row.OriginalName,
		UploadedBy:   row.UploadedBy,
		CreatedAt:    row.CreatedAt,
	}
}
