// api/internal/storage/localfs_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

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

func TestLocalFS_Move_MissingSource_ReturnsError(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	if err := s.Move(context.Background(), "no/such/key.jpg", "dst/key.jpg"); err == nil {
		t.Fatal("expected error when moving a missing source")
	}
}

func TestLocalFS_Delete_MissingKey_NoError(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	// localfs treats Delete-of-missing as a no-op so callers can call it
	// idempotently as part of orphan cleanup. Pin that contract.
	if err := s.Delete(context.Background(), "no/such/key.jpg"); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestLocalFS_Delete_ExistingKey_RemovesFile(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ctx := context.Background()
	if err := s.Put(ctx, "abc/x.jpg", strings.NewReader("hi"), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if ok, _ := s.Exists(ctx, "abc/x.jpg"); !ok {
		t.Fatal("file should exist after Put")
	}
	if err := s.Delete(ctx, "abc/x.jpg"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok, _ := s.Exists(ctx, "abc/x.jpg"); ok {
		t.Fatal("file should not exist after Delete")
	}
}

func TestLocalFS_Exists_FalseForMissing(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	ok, err := s.Exists(context.Background(), "no/such/key")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("expected false for missing key")
	}
}

// TestLocalFS_SignedURL_ReturnsUnsupported pins the current contract: localfs
// does NOT implement SignedURL — it returns ("", error). Verified against
// api/internal/storage/localfs.go:91-93. HTTP-level signed URLs come from
// auth.URLBuilder, not the storage adapter.
func TestLocalFS_SignedURL_ReturnsUnsupported(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	url, err := s.SignedURL(context.Background(), "abc/0.jpg", 5*time.Minute)
	if err == nil {
		t.Fatal("expected localfs.SignedURL to return an error")
	}
	if url != "" {
		t.Fatalf("expected empty URL on error, got %q", url)
	}
	if !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("expected unsupported message, got %v", err)
	}
}

func TestLocalFS_RejectsTraversal_Variants(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	bad := []string{"../etc/passwd", "abc/../../etc/passwd", "abc/./../../etc/passwd"}
	for _, k := range bad {
		if err := s.Put(context.Background(), k, strings.NewReader("x"), "text/plain"); err == nil {
			t.Fatalf("expected traversal rejection for %q", k)
		}
	}
}

func TestLocalFS_Delete_RejectsTraversal(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	if err := s.Delete(context.Background(), "../etc/passwd"); err == nil {
		t.Fatal("expected traversal rejection on Delete")
	}
}

func TestLocalFS_Move_RejectsTraversalSrc(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	if err := s.Move(context.Background(), "../etc/passwd", "valid/dst"); err == nil {
		t.Fatal("expected traversal rejection on Move src")
	}
}

func TestLocalFS_Move_RejectsTraversalDst(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	// Need to seed a real source first so Move gets past the abs(src) check
	// and reaches the abs(dst) check.
	if err := s.Put(context.Background(), "valid/src.jpg",
		strings.NewReader("x"), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := s.Move(context.Background(), "valid/src.jpg", "../etc/passwd"); err == nil {
		t.Fatal("expected traversal rejection on Move dst")
	}
}

func TestLocalFS_Exists_RejectsTraversal(t *testing.T) {
	s := storage.NewLocalFS(t.TempDir())
	_, err := s.Exists(context.Background(), "../etc/passwd")
	if err == nil {
		t.Fatal("expected traversal rejection on Exists")
	}
}
