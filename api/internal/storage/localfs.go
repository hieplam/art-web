// api/internal/storage/localfs.go
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type localFS struct{ root string }

func NewLocalFS(root string) Storage { return &localFS{root: root} }

func (l *localFS) abs(key string) (string, error) {
	if strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return "", fmt.Errorf("invalid key %q", key)
	}
	return filepath.Join(l.root, filepath.FromSlash(key)), nil
}

func (l *localFS) Put(_ context.Context, key string, body io.Reader, _ string) error {
	p, err := l.abs(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func (l *localFS) Get(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.abs(key)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (l *localFS) Delete(_ context.Context, key string) error {
	p, err := l.abs(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (l *localFS) Move(ctx context.Context, src, dst string) error {
	sp, err := l.abs(src)
	if err != nil {
		return err
	}
	dp, err := l.abs(dst)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dp), 0o755); err != nil {
		return err
	}
	return os.Rename(sp, dp)
}

func (l *localFS) Exists(_ context.Context, key string) (bool, error) {
	p, err := l.abs(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (l *localFS) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("localfs does not support signed URLs")
}
