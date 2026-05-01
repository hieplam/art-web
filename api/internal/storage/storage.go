// api/internal/storage/storage.go
package storage

import (
	"context"
	"io"
	"time"
)

type Storage interface {
	Put(ctx context.Context, key string, body io.Reader, contentType string) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Move(ctx context.Context, srcKey, dstKey string) error
	Exists(ctx context.Context, key string) (bool, error)
	SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error)
}
