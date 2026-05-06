package service_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	artworkservice "local/art-web/api/internal/artwork/service"
	"local/art-web/api/internal/infrastructure/database"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	infratest "local/art-web/api/internal/infrastructure/testing"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

// captureRollbackReporter implements ports.RollbackReporter for tests that need
// to count or inspect rollback firings without touching package-global state.
type captureRollbackReporter struct {
	calls atomic.Int32
}

func (c *captureRollbackReporter) Report(_ context.Context, _ string, _ error) {
	c.calls.Add(1)
}

func newVisCtx(t *testing.T) (*artworkpostgres.Repo, string) {
	t.Helper()
	db, cleanup, err := database.NewGormDBFromDSN(infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	t.Cleanup(cleanup)
	infratest.TruncateAllGorm(t, db)
	users := userpostgres.NewRepo(db)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S1", "a@b", "alice", "")
	return artworkpostgres.NewRepo(db), uid
}

// seedImageRow inserts a synthetic artwork_images row using raw GORM Exec so
// each visibility-flip test can stage rows without going through the image
// service's full upload pipeline.
func seedImageRow(t *testing.T, repo *artworkpostgres.Repo, artworkID, clientID, storageKey string, position int) {
	t.Helper()
	err := repo.DB().WithContext(t.Context()).Exec(
		`INSERT INTO artwork_images (artwork_id, client_image_id, storage_key, source_sha256, width, height, byte_size, content_type, position)
		 VALUES (?, ?, ?, 'sha', 1, 1, 1, 'image/jpeg', ?)`,
		artworkID, clientID, storageKey, position).Error
	if err != nil {
		t.Fatalf("seed image row: %v", err)
	}
}

func TestFlip_PrivateToPublic_MovesObjectsAndUpdatesKeys(t *testing.T) {
	repo, uid := newVisCtx(t)
	store := infrastorage.NewLocalFS(t.TempDir())
	svc := artworkservice.NewVisibilityService(repo, store, nil)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	_ = store.Put(t.Context(), "private/"+aid+"/img1.jpg", strings.NewReader("bytes"), "image/jpeg")
	seedImageRow(t, repo, aid, "K", "private/"+aid+"/img1.jpg", 0)

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
	infrastorage.Storage
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
	repo, uid := newVisCtx(t)
	base := infrastorage.NewLocalFS(t.TempDir())
	store := &flakyMoveStore{Storage: base, failOn: 2}
	svc := artworkservice.NewVisibilityService(repo, store, nil)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	for i, k := range []string{"img1", "img2"} {
		_ = base.Put(t.Context(), "private/"+aid+"/"+k+".jpg",
			strings.NewReader("bytes"+k), "image/jpeg")
		seedImageRow(t, repo, aid, "K"+k, "private/"+aid+"/"+k+".jpg", i)
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

// TestFlip_SameVisibility_NoOp covers the early-return path: flipping
// public→public (or private→private) is a no-op that returns nil immediately
// without touching storage or the DB.
func TestFlip_SameVisibility_NoOp(t *testing.T) {
	repo, uid := newVisCtx(t)
	store := infrastorage.NewLocalFS(t.TempDir())
	svc := artworkservice.NewVisibilityService(repo, store, nil)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := svc.Flip(t.Context(), aid, "private"); err != nil {
		t.Fatalf("same-visibility flip should be no-op, got %v", err)
	}
	got, _ := repo.Get(t.Context(), aid)
	if got.Visibility != "private" {
		t.Fatalf("visibility changed: %q", got.Visibility)
	}
}

// TestFlip_BadTarget_ReturnsError covers the target-validation guard. Targets
// other than "public"/"private" must return errBadTarget without touching
// storage or DB.
func TestFlip_BadTarget_ReturnsError(t *testing.T) {
	repo, uid := newVisCtx(t)
	store := infrastorage.NewLocalFS(t.TempDir())
	svc := artworkservice.NewVisibilityService(repo, store, nil)

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")
	if err := svc.Flip(t.Context(), aid, "draft"); err == nil {
		t.Fatal("expected error for invalid target")
	}
}

// reverseFailStore makes forward Moves succeed but reverse Moves fail. Used to
// exercise the RollbackReporter path: after the primary forward Move fails, the
// undo (reverse Move) ALSO fails — RollbackReporter.Report is called.
type reverseFailStore struct {
	infrastorage.Storage
	forwardCalls  atomic.Int32
	failForwardOn int32
	aid           string
}

func (r *reverseFailStore) Move(ctx context.Context, src, dst string) error {
	forward := strings.HasPrefix(src, "private/"+r.aid+"/") && strings.HasPrefix(dst, "public/"+r.aid+"/")
	if forward {
		n := r.forwardCalls.Add(1)
		if n == r.failForwardOn {
			return errors.New("trigger forward move failure")
		}
		return r.Storage.Move(ctx, src, dst)
	}
	return errors.New("simulated reverse-move failure")
}

func TestFlip_RollbackReporterFires_WhenUndoMoveFails(t *testing.T) {
	repo, uid := newVisCtx(t)
	base := infrastorage.NewLocalFS(t.TempDir())

	aid, _ := repo.Create(t.Context(), uid, "x", "", "private")

	for i, k := range []string{"img1", "img2"} {
		_ = base.Put(t.Context(), "private/"+aid+"/"+k+".jpg",
			strings.NewReader("bytes"+k), "image/jpeg")
		seedImageRow(t, repo, aid, "K"+k, "private/"+aid+"/"+k+".jpg", i)
	}

	store := &reverseFailStore{Storage: base, failForwardOn: 2, aid: aid}
	reporter := &captureRollbackReporter{}
	svc := artworkservice.NewVisibilityService(repo, store, reporter)

	if err := svc.Flip(t.Context(), aid, "public"); err == nil {
		t.Fatal("expected flip to fail when forward move fails")
	}
	if reporter.calls.Load() == 0 {
		t.Fatal("expected RollbackReporter to fire when undo move fails")
	}
}
