package postgres_test

import (
	"context"
	"fmt"
	"testing"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	"local/art-web/api/internal/infrastructure/database"
	infratest "local/art-web/api/internal/infrastructure/testing"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

func setup(t *testing.T) (*imagepostgres.Repo, string) {
	db, cleanup, err := database.NewGormDBFromDSN(infratest.StartPostgres(t))
	if err != nil {
		t.Fatalf("NewGormDB: %v", err)
	}
	t.Cleanup(cleanup)
	infratest.TruncateAllGorm(t, db)
	users := userpostgres.NewRepo(db)
	uid, _ := users.UpsertOAuth(t.Context(), "google", "S", "a@b", "alice", "")
	arts := artworkpostgres.NewRepo(db)
	aid, _ := arts.Create(t.Context(), uid, "x", "", "private")
	return imagepostgres.NewRepo(db), aid
}

func TestCase17_RetryWithSameClientIDAndSameBytes_IsNoOp(t *testing.T) {
	repo, aid := setup(t)
	in := imagepostgres.InsertInput{ArtworkID: aid, ClientImageID: "K1", Position: 0,
		ContentType: "image/jpeg", StorageKey: "private/" + aid + "/abc.jpg",
		SourceSHA256: "deadbeef", Width: 50, Height: 50, ByteSize: 1234, Blurhash: "L0"}
	r1, err := repo.Insert(t.Context(), in)
	if err != nil {
		t.Fatalf("first insert: %v", err)
	}
	r2, err := repo.Insert(t.Context(), in)
	if err != nil {
		t.Fatalf("retry insert: %v", err)
	}
	if r1.ID != r2.ID || !r2.Existed {
		t.Fatalf("retry returned different row: r1=%+v r2=%+v", r1, r2)
	}
}

func TestCase17b_SameClientIDDifferentBytes_RejectedAsFingerprintMismatch(t *testing.T) {
	repo, aid := setup(t)
	in := imagepostgres.InsertInput{ArtworkID: aid, ClientImageID: "K1", Position: 0,
		ContentType: "image/jpeg", StorageKey: "k1",
		SourceSHA256: "deadbeef", Width: 50, Height: 50, ByteSize: 1, Blurhash: ""}
	if _, err := repo.Insert(t.Context(), in); err != nil {
		t.Fatalf("first: %v", err)
	}
	in.SourceSHA256 = "cafebabe"
	in.StorageKey = "k1-other"
	if _, err := repo.Insert(t.Context(), in); err != imagepostgres.ErrFingerprintMismatch {
		t.Fatalf("want ErrFingerprintMismatch, got %v", err)
	}
}

func TestCase18_PositionCollision_RejectedAs412(t *testing.T) {
	repo, aid := setup(t)
	a := imagepostgres.InsertInput{ArtworkID: aid, ClientImageID: "K1", Position: 0,
		ContentType: "image/jpeg", StorageKey: "k1", SourceSHA256: "aaa",
		Width: 1, Height: 1, ByteSize: 1, Blurhash: ""}
	b := a
	b.ClientImageID = "K2"
	b.StorageKey = "k2"
	b.SourceSHA256 = "bbb"
	if _, err := repo.Insert(t.Context(), a); err != nil {
		t.Fatalf("a: %v", err)
	}
	if _, err := repo.Insert(t.Context(), b); err != imagepostgres.ErrPositionTaken {
		t.Fatalf("expected ErrPositionTaken, got %v", err)
	}
}

func TestListByArtwork_ReturnsImagesInPositionOrder(t *testing.T) {
	repo, aid := setup(t)
	ctx := context.Background()

	for _, p := range []int{2, 0, 1} {
		_, err := repo.Insert(ctx, imagepostgres.InsertInput{
			ArtworkID:     aid,
			ClientImageID: fmt.Sprintf("CID-%d", p),
			ContentType:   "image/jpeg",
			StorageKey:    fmt.Sprintf("private/%s/%d.jpg", aid, p),
			SourceSHA256:  fmt.Sprintf("sha-%d", p),
			Width:         1, Height: 1, ByteSize: 10,
			Position: p,
			Blurhash: "",
		})
		if err != nil {
			t.Fatalf("insert position %d: %v", p, err)
		}
	}

	got, err := repo.ListByArtwork(ctx, aid)
	if err != nil {
		t.Fatalf("ListByArtwork: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 images, got %d", len(got))
	}
	for i, im := range got {
		if im.Position != i {
			t.Fatalf("image[%d].Position = %d, want %d", i, im.Position, i)
		}
	}
}
