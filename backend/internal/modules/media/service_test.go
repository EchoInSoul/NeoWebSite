package media

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNormalizeExtension(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"PHOTO.JPG", "jpg"},
		{"a.tar.gz", "gz"},
		{"photo.", "bin"},
		{"中文.png", "png"},
		{"", "bin"},
		{".", "bin"},
		{"..", "bin"},
		{"abcdefghijklmnopqrstuvwxyz", "bin"},
		{"a.abcdefghijklmnopq", "bin"},
		{".gitignore", "gitignore"},
	}

	for _, c := range cases {
		if got := normalizeExtension(c.name); got != c.want {
			t.Errorf("normalizeExtension(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestValidateOriginalName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"", true},
		{"素材包.zip", true},
		{"a\x00b", false},
		{string([]byte{0xff, 0xfe}), false},
	}

	for _, c := range cases {
		err := validateOriginalName(c.name)
		if c.ok && err != nil {
			t.Errorf("validateOriginalName(%q) = %v, want nil", c.name, err)
		}
		if !c.ok && !errors.Is(err, errInvalidOriginalName) {
			t.Errorf("validateOriginalName(%q) = %v, want errInvalidOriginalName", c.name, err)
		}
	}
}

type fakeStore struct {
	insertErr error
	inserted  []mediaAsset
	getAsset  mediaAsset
	getErr    error
	deleteOK  bool
	deleteErr error
	deleted   []uuid.UUID
	events    *[]string
}

func (f *fakeStore) Insert(ctx context.Context, asset mediaAsset) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.inserted = append(f.inserted, asset)
	return nil
}

func (f *fakeStore) Get(ctx context.Context, id uuid.UUID) (mediaAsset, error) {
	return f.getAsset, f.getErr
}

func (f *fakeStore) Delete(ctx context.Context, id uuid.UUID) (bool, error) {
	f.deleted = append(f.deleted, id)
	if f.events != nil {
		*f.events = append(*f.events, "record:"+id.String())
	}
	return f.deleteOK, f.deleteErr
}

type fakeProvider struct {
	saveErr error
	delErr  error
	saved   []string
	deleted []string
	events  *[]string
}

func (f *fakeProvider) Name() string {
	return "local"
}

func (f *fakeProvider) URL(key string) string {
	return "/storage/" + key
}

func (f *fakeProvider) Save(ctx context.Context, key string, r io.Reader) (int64, error) {
	if f.saveErr != nil {
		return 0, f.saveErr
	}
	f.saved = append(f.saved, key)
	return io.Copy(io.Discard, r)
}

func (f *fakeProvider) Delete(ctx context.Context, key string) error {
	f.deleted = append(f.deleted, key)
	if f.events != nil {
		*f.events = append(*f.events, "file:"+key)
	}
	return f.delErr
}

type rejectAll struct{}

func (rejectAll) Check(int64) error {
	return errors.New("rejected")
}

func newServiceForTest(root string, p Provider, st store) *Service {
	return &Service{
		store:         st,
		providers:     map[string]Provider{p.Name(): p},
		writeProvider: p,
		policy:        allowAll{},
		root:          root,
	}
}

func TestUploadWritesFileAndRecord(t *testing.T) {
	root := t.TempDir()
	db := &fakeStore{}
	service := newServiceForTest(root, newLocal(root), db)
	uploader := uuid.Must(uuid.NewV7())

	asset, err := service.Upload(context.Background(), uploader, "a.jpg", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if len(db.inserted) != 1 {
		t.Fatalf("inserted %d records, want 1", len(db.inserted))
	}
	if asset.Provider != "local" {
		t.Fatalf("provider = %q, want local", asset.Provider)
	}
	if asset.SizeBytes != 5 {
		t.Fatalf("size = %d, want 5", asset.SizeBytes)
	}
	if asset.OriginalName != "a.jpg" {
		t.Fatalf("original name = %q, want a.jpg", asset.OriginalName)
	}
	if asset.UploadedBy == nil || *asset.UploadedBy != uploader {
		t.Fatalf("uploadedBy = %v, want %v", asset.UploadedBy, uploader)
	}
	if !strings.HasSuffix(asset.Key, ".jpg") {
		t.Fatalf("key = %q, want .jpg suffix", asset.Key)
	}

	written, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(asset.Key)))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(written) != "hello" {
		t.Fatalf("written = %q, want hello", written)
	}
}

func TestUploadRejectsNULInName(t *testing.T) {
	root := t.TempDir()
	provider := &fakeProvider{}
	service := newServiceForTest(root, provider, &fakeStore{})

	if _, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a\x00b.jpg", strings.NewReader("x")); !errors.Is(err, errInvalidOriginalName) {
		t.Fatalf("err = %v, want errInvalidOriginalName", err)
	}
	if len(provider.saved) != 0 {
		t.Fatalf("provider saved %d files, want 0", len(provider.saved))
	}
}

func TestUploadLeavesNoFileWhenReaderFails(t *testing.T) {
	root := t.TempDir()
	db := &fakeStore{}
	service := newServiceForTest(root, newLocal(root), db)

	if _, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a.jpg", errorReader{}); err == nil {
		t.Fatal("upload must fail when the reader fails")
	}
	if len(db.inserted) != 0 {
		t.Fatalf("inserted %d records, want 0", len(db.inserted))
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			t.Errorf("leftover file: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestUploadDeletesFileWhenInsertFails(t *testing.T) {
	root := t.TempDir()
	provider := &fakeProvider{}
	db := &fakeStore{insertErr: errors.New("db down")}
	service := newServiceForTest(root, provider, db)

	if _, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a.jpg", strings.NewReader("x")); err == nil {
		t.Fatal("upload must fail when the insert fails")
	}
	if len(provider.deleted) != 1 {
		t.Fatalf("provider delete calls = %d, want 1", len(provider.deleted))
	}
	if len(provider.saved) != 1 || provider.saved[0] != provider.deleted[0] {
		t.Fatalf("deleted key %v does not match saved key %v", provider.deleted, provider.saved)
	}
}

func TestUploadDeletesFileWhenPolicyRejects(t *testing.T) {
	root := t.TempDir()
	provider := &fakeProvider{}
	service := newServiceForTest(root, provider, &fakeStore{})
	service.policy = rejectAll{}

	if _, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a.jpg", strings.NewReader("x")); err == nil {
		t.Fatal("upload must fail when the policy rejects")
	}
	if len(provider.deleted) != 1 || provider.saved[0] != provider.deleted[0] {
		t.Fatalf("deleted %v, saved %v", provider.deleted, provider.saved)
	}
}

func TestUploadUsesSameTimestampForDirectoryAndRecord(t *testing.T) {
	root := t.TempDir()
	db := &fakeStore{}
	service := newServiceForTest(root, newLocal(root), db)

	asset, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a.jpg", strings.NewReader("x"))
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if len(asset.Key) < 10 {
		t.Fatalf("key too short: %q", asset.Key)
	}
	if asset.Key[:10] != asset.CreatedAt.UTC().Format("2006/01/02") {
		t.Fatalf("key date %q != created_at date %q", asset.Key[:10], asset.CreatedAt.UTC().Format("2006/01/02"))
	}
}

func TestDeleteRemovesFileBeforeRecord(t *testing.T) {
	events := []string{}
	id := uuid.Must(uuid.NewV7())
	key := "2026/10/27/x.jpg"
	provider := &fakeProvider{events: &events}
	db := &fakeStore{getAsset: mediaAsset{ID: id, Provider: "local", Key: key}, deleteOK: true, events: &events}
	service := newServiceForTest(t.TempDir(), provider, db)

	if err := service.Delete(context.Background(), id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	fileIdx, recordIdx := -1, -1
	for i, event := range events {
		if event == "file:"+key {
			fileIdx = i
		}
		if event == "record:"+id.String() {
			recordIdx = i
		}
	}
	if fileIdx < 0 || recordIdx < 0 {
		t.Fatalf("events = %v", events)
	}
	if fileIdx > recordIdx {
		t.Fatalf("file delete must precede record delete: %v", events)
	}
}

func TestDeleteKeepsRecordWhenFileRemovalFails(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	provider := &fakeProvider{delErr: errors.New("locked")}
	db := &fakeStore{getAsset: mediaAsset{ID: id, Provider: "local", Key: "2026/10/27/x.jpg"}, deleteOK: true}
	service := newServiceForTest(t.TempDir(), provider, db)

	if err := service.Delete(context.Background(), id); err == nil {
		t.Fatal("delete must fail when the file cannot be removed")
	}
	if len(db.deleted) != 0 {
		t.Fatalf("record delete calls = %d, want 0", len(db.deleted))
	}
}

func TestDeleteTreatsMissingFileAsAlreadyDeleted(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	provider := &fakeProvider{delErr: fs.ErrNotExist}
	db := &fakeStore{getAsset: mediaAsset{ID: id, Provider: "local", Key: "2026/10/27/x.jpg"}, deleteOK: true}
	service := newServiceForTest(t.TempDir(), provider, db)

	if err := service.Delete(context.Background(), id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(db.deleted) != 1 {
		t.Fatalf("record delete calls = %d, want 1", len(db.deleted))
	}
}

func TestDeleteReturnsInternalErrorWhenProviderUnknown(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	provider := &fakeProvider{}
	db := &fakeStore{getAsset: mediaAsset{ID: id, Provider: "s3", Key: "x.jpg"}, deleteOK: true}
	service := newServiceForTest(t.TempDir(), provider, db)

	if err := service.Delete(context.Background(), id); !errors.Is(err, errUnknownProvider) {
		t.Fatalf("err = %v, want errUnknownProvider", err)
	}
	if len(db.deleted) != 0 || len(provider.deleted) != 0 {
		t.Fatalf("nothing should be deleted: store=%v provider=%v", db.deleted, provider.deleted)
	}
}

func TestDeleteReturnsNotFoundWhenRecordMissing(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	db := &fakeStore{getErr: errNotFound}
	service := newServiceForTest(t.TempDir(), &fakeProvider{}, db)

	if err := service.Delete(context.Background(), id); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

func TestDeleteReturnsNotFoundWhenRowAlreadyGone(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	db := &fakeStore{getAsset: mediaAsset{ID: id, Provider: "local", Key: "2026/10/27/x.jpg"}, deleteOK: false}
	service := newServiceForTest(t.TempDir(), &fakeProvider{}, db)

	if err := service.Delete(context.Background(), id); !errors.Is(err, errNotFound) {
		t.Fatalf("err = %v, want errNotFound", err)
	}
}

type partialErrorReader struct {
	read bool
}

func (r *partialErrorReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, "abc"), nil
	}
	return 0, errors.New("boom")
}

func TestUploadLeavesNoFileWhenReaderFailsMidway(t *testing.T) {
	root := t.TempDir()
	db := &fakeStore{}
	service := newServiceForTest(root, newLocal(root), db)

	if _, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a.jpg", &partialErrorReader{}); err == nil {
		t.Fatal("upload must fail when the reader fails after some bytes")
	}
	if len(db.inserted) != 0 {
		t.Fatalf("inserted %d records, want 0", len(db.inserted))
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			t.Errorf("leftover file: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func TestUploadReportsStorageError(t *testing.T) {
	root := t.TempDir()
	provider := &fakeProvider{saveErr: io.ErrUnexpectedEOF}
	service := newServiceForTest(root, provider, &fakeStore{})

	_, err := service.Upload(context.Background(), uuid.Must(uuid.NewV7()), "a.jpg", strings.NewReader("x"))
	if !errors.Is(err, errStorage) {
		t.Fatalf("err = %v, want errStorage", err)
	}
}

func TestNewServiceRejectsEmptyRoot(t *testing.T) {
	if _, err := newService(nil, "", allowAll{}); !errors.Is(err, errEmptyRoot) {
		t.Fatalf("err = %v, want errEmptyRoot", err)
	}
}
