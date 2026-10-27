package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"website/backend/internal/database"
)

type mediaAsset struct {
	ID           uuid.UUID
	Provider     string
	Key          string
	SizeBytes    int64
	OriginalName string
	UploadedBy   *uuid.UUID
	CreatedAt    time.Time
}

type store interface {
	Insert(ctx context.Context, asset mediaAsset) error
	Get(ctx context.Context, id uuid.UUID) (mediaAsset, error)
	Delete(ctx context.Context, id uuid.UUID) (bool, error)
}

type Service struct {
	store         store
	providers     map[string]Provider
	writeProvider Provider
	policy        UploadPolicy
	root          string
}

var (
	errNotFound            = errors.New("media: not found")
	errInvalidOriginalName = errors.New("media: invalid original name")
	errUnknownProvider     = errors.New("media: unknown provider")
	errStorage             = errors.New("media: storage failure")
	errEmptyRoot           = errors.New("media: storage root is empty")
)

var extensionPattern = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

func newService(db *database.Database, root string, policy UploadPolicy) (*Service, error) {
	if root == "" {
		return nil, errEmptyRoot
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}

	probe, err := os.CreateTemp(abs, ".probe-*")
	if err != nil {
		return nil, err
	}
	probeName := probe.Name()
	if err := probe.Close(); err != nil {
		os.Remove(probeName)
		return nil, err
	}
	if err := os.Remove(probeName); err != nil {
		return nil, err
	}

	slog.Info("media storage root", "path", abs)

	local := newLocal(abs)
	return &Service{
		store:         newRepository(db),
		providers:     map[string]Provider{"local": local},
		writeProvider: local,
		policy:        policy,
		root:          abs,
	}, nil
}

func (s *Service) Root() string {
	return s.root
}

func (s *Service) URL(asset mediaAsset) (string, error) {
	provider, ok := s.providers[asset.Provider]
	if !ok {
		return "", errUnknownProvider
	}
	return provider.URL(asset.Key), nil
}

func (s *Service) Upload(ctx context.Context, uploaderID uuid.UUID, name string, r io.Reader) (mediaAsset, error) {
	if err := validateOriginalName(name); err != nil {
		return mediaAsset{}, err
	}

	ext := normalizeExtension(name)
	now := time.Now().UTC()
	id := uuid.Must(uuid.NewV7())
	key := now.Format("2006/01/02") + "/" + id.String() + "." + ext

	size, err := s.writeProvider.Save(ctx, key, r)
	if err != nil {
		return mediaAsset{}, fmt.Errorf("%w: %w", errStorage, err)
	}
	if err := s.policy.Check(size); err != nil {
		s.discard(ctx, key)
		return mediaAsset{}, err
	}

	asset := mediaAsset{
		ID:           id,
		Provider:     s.writeProvider.Name(),
		Key:          key,
		SizeBytes:    size,
		OriginalName: name,
		UploadedBy:   &uploaderID,
		CreatedAt:    now,
	}
	if err := s.store.Insert(ctx, asset); err != nil {
		s.discard(ctx, key)
		return mediaAsset{}, err
	}
	return asset, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	asset, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}

	provider, ok := s.providers[asset.Provider]
	if !ok {
		return errUnknownProvider
	}
	if err := provider.Delete(ctx, asset.Key); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	deleted, err := s.store.Delete(ctx, id)
	if err != nil {
		return err
	}
	if !deleted {
		return errNotFound
	}
	return nil
}

func (s *Service) discard(ctx context.Context, key string) {
	if err := s.writeProvider.Delete(context.WithoutCancel(ctx), key); err != nil {
		slog.Error("media: failed to remove file after failed upload", "key", key, "error", err)
	}
}

func normalizeExtension(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if !extensionPattern.MatchString(ext) {
		return "bin"
	}
	return ext
}

func validateOriginalName(name string) error {
	if strings.ContainsRune(name, 0) || !utf8.ValidString(name) {
		return errInvalidOriginalName
	}
	return nil
}
