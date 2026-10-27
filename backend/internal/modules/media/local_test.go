package media

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func TestLocalSaveWritesFileAndReturnsSize(t *testing.T) {
	root := t.TempDir()
	p := newLocal(root)

	size, err := p.Save(context.Background(), "2026/10/27/a.txt", strings.NewReader("hello"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if size != 5 {
		t.Fatalf("size = %d, want 5", size)
	}

	got, err := os.ReadFile(filepath.Join(root, "2026", "10", "27", "a.txt"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want %q", got, "hello")
	}
}

func TestLocalSaveCreatesNestedDirectories(t *testing.T) {
	root := t.TempDir()
	p := newLocal(root)

	if _, err := p.Save(context.Background(), "2026/10/27/x.jpg", bytes.NewReader(nil)); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(filepath.Join(root, "2026", "10", "27"))
	if err != nil || !info.IsDir() {
		t.Fatalf("nested dir missing: %v", err)
	}
}

func TestLocalSaveRemovesTempFileOnReadError(t *testing.T) {
	root := t.TempDir()
	p := newLocal(root)

	if _, err := p.Save(context.Background(), "2026/10/27/a.txt", errorReader{}); err == nil {
		t.Fatal("save must fail when the reader fails")
	}

	entries, err := os.ReadDir(filepath.Join(root, "2026", "10", "27"))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("directory has %d entries, want 0", len(entries))
	}
}

func TestLocalSaveRemovesTempFileOnRenameConflict(t *testing.T) {
	root := t.TempDir()
	p := newLocal(root)

	target := filepath.Join(root, "2026", "10", "27", "a.jpg")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("prepare conflict: %v", err)
	}

	if _, err := p.Save(context.Background(), "2026/10/27/a.jpg", strings.NewReader("x")); err == nil {
		t.Fatal("save must fail when the target path is occupied by a directory")
	}

	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".upload-") {
			t.Fatalf("temp file left behind: %s", entry.Name())
		}
	}
}

func TestLocalDeleteMissingFileReturnsErrNotExist(t *testing.T) {
	p := newLocal(t.TempDir())

	if err := p.Delete(context.Background(), "2026/10/27/none.jpg"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("delete missing = %v, want fs.ErrNotExist", err)
	}
}

func TestLocalURL(t *testing.T) {
	p := newLocal(t.TempDir())

	if got := p.URL("2026/10/27/a.jpg"); got != "/storage/2026/10/27/a.jpg" {
		t.Fatalf("URL = %q, want %q", got, "/storage/2026/10/27/a.jpg")
	}
}
