package media

import (
	"context"
	"io"
)

type Provider interface {
	Name() string
	Save(ctx context.Context, key string, r io.Reader) (int64, error)
	Delete(ctx context.Context, key string) error
	URL(key string) string
}

type UploadPolicy interface {
	Check(size int64) error
}

type allowAll struct{}

func (allowAll) Check(int64) error { return nil }
