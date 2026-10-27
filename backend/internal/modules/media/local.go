package media

import (
	"context"
	"io"
	"os"
	"path/filepath"
)

type localProvider struct {
	root string
}

func newLocal(root string) *localProvider {
	return &localProvider{root: root}
}

func (p *localProvider) Name() string {
	return "local"
}

func (p *localProvider) URL(key string) string {
	return "/storage/" + key
}

func (p *localProvider) Save(ctx context.Context, key string, r io.Reader) (int64, error) {
	target := p.path(key)
	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}

	tmp, err := os.CreateTemp(dir, ".upload-*")
	if err != nil {
		return 0, err
	}
	tmpName := tmp.Name()

	size, err := io.Copy(tmp, r)
	if err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return 0, err
	}
	return size, nil
}

func (p *localProvider) Delete(ctx context.Context, key string) error {
	return os.Remove(p.path(key))
}

func (p *localProvider) path(key string) string {
	return filepath.Join(p.root, filepath.FromSlash(key))
}
