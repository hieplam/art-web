// api/internal/storage/localfs_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"local/art-web/api/internal/storage"
)

func TestLocalFS_PutGet(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ctx := context.Background()
	if err := s.Put(ctx, "public/abc/0.jpg", strings.NewReader("hello"), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	r, err := s.Get(ctx, "public/abc/0.jpg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer r.Close()
	got, _ := io.ReadAll(r)
	if string(got) != "hello" {
		t.Fatalf("want hello, got %q", got)
	}
}

func TestLocalFS_Move(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ctx := context.Background()
	_ = s.Put(ctx, "public/x/0.jpg", bytes.NewReader([]byte("h")), "image/jpeg")
	if err := s.Move(ctx, "public/x/0.jpg", "private/x/0.jpg"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if ok, _ := s.Exists(ctx, "public/x/0.jpg"); ok {
		t.Fatal("source should not exist after move")
	}
	if ok, _ := s.Exists(ctx, "private/x/0.jpg"); !ok {
		t.Fatal("dest should exist after move")
	}
}

func TestLocalFS_RejectsTraversal(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	if err := s.Put(context.Background(), "../etc/passwd", strings.NewReader("x"), "text/plain"); err == nil {
		t.Fatal("expected traversal rejection")
	}
}
