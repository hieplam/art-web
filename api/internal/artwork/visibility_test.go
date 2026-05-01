package artwork_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/storage"
)

func TestFlip_PrivateToPublic_MovesObjectsAndUpdatesKeys(t *testing.T) {
	repo, _, uid := newCtx(t)
	store := storage.NewLocalFS(t.TempDir())
	svc := artwork.NewVisibilityService(repo, store)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	_ = store.Put(t.Context(), "private/"+aid+"/img1.jpg", strings.NewReader("bytes"), "image/jpeg")
	_, _ = repo.Pool().Exec(t.Context(),
		`INSERT INTO artwork_images (artwork_id, client_image_id, storage_key, source_sha256, width, height, byte_size, content_type, position)
		 VALUES ($1,'K',$2,'sha', 1,1,1,'image/jpeg',0)`, aid, "private/"+aid+"/img1.jpg")

	if err := svc.Flip(t.Context(), aid, "public"); err != nil {
		t.Fatalf("flip: %v", err)
	}
	if ok, _ := store.Exists(t.Context(), "public/"+aid+"/img1.jpg"); !ok {
		t.Fatal("public copy missing")
	}
	if ok, _ := store.Exists(t.Context(), "private/"+aid+"/img1.jpg"); ok {
		t.Fatal("private copy not deleted")
	}
}

type flakyMoveStore struct {
	storage.Storage
	failOn int32
	calls  atomic.Int32
}

func (f *flakyMoveStore) Move(ctx context.Context, src, dst string) error {
	n := f.calls.Add(1)
	if n == f.failOn {
		return errors.New("simulated move failure")
	}
	return f.Storage.Move(ctx, src, dst)
}

func TestFlip_PartialFailure_RollsBackMoves(t *testing.T) {
	repo, _, uid := newCtx(t)
	base := storage.NewLocalFS(t.TempDir())
	store := &flakyMoveStore{Storage: base, failOn: 2}
	svc := artwork.NewVisibilityService(repo, store)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	for i, k := range []string{"img1", "img2"} {
		_ = base.Put(t.Context(), "private/"+aid+"/"+k+".jpg",
			strings.NewReader("bytes"+k), "image/jpeg")
		if _, err := repo.Pool().Exec(t.Context(),
			`INSERT INTO artwork_images (artwork_id, client_image_id, storage_key, source_sha256, width, height, byte_size, content_type, position)
			 VALUES ($1,$2,$3,'sha', 1,1,1,'image/jpeg',$4)`,
			aid, "K"+k, "private/"+aid+"/"+k+".jpg", i); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	if err := svc.Flip(t.Context(), aid, "public"); err == nil {
		t.Fatal("expected flip to fail when second move errors")
	}

	for _, k := range []string{"img1", "img2"} {
		if ok, _ := store.Exists(t.Context(), "public/"+aid+"/"+k+".jpg"); ok {
			t.Fatalf("PRIVACY LEAK: %s.jpg ended up in /public/ after rollback failure", k)
		}
		if ok, _ := store.Exists(t.Context(), "private/"+aid+"/"+k+".jpg"); !ok {
			t.Fatalf("rollback dropped image: %s.jpg missing from /private/", k)
		}
	}

	got, _ := repo.Get(t.Context(), aid)
	if got.Visibility != "private" {
		t.Fatalf("visibility changed despite failure: %q", got.Visibility)
	}
}
