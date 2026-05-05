package image

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"local/art-web/api/internal/artwork"
)

type stubRepo struct {
	findFn   func(ctx context.Context, art, cid string) (*InsertedImage, error)
	insertFn func(ctx context.Context, in InsertInput) (*InsertResult, error)
}

func (s *stubRepo) FindByClientImageID(ctx context.Context, art, cid string) (*InsertedImage, error) {
	return s.findFn(ctx, art, cid)
}
func (s *stubRepo) Insert(ctx context.Context, in InsertInput) (*InsertResult, error) {
	return s.insertFn(ctx, in)
}

type stubStore struct {
	puts    []string
	deletes []string
	failPut bool
}

func (s *stubStore) Put(_ context.Context, k string, _ io.Reader, _ string) error {
	if s.failPut {
		return errors.New("storage Put failed")
	}
	s.puts = append(s.puts, k)
	return nil
}
func (s *stubStore) Get(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *stubStore) Delete(_ context.Context, k string) error {
	s.deletes = append(s.deletes, k)
	return nil
}
func (s *stubStore) Move(context.Context, string, string) error { return nil }
func (s *stubStore) Exists(context.Context, string) (bool, error) {
	return false, nil
}
func (s *stubStore) SignedURL(context.Context, string, time.Duration) (string, error) {
	return "", nil
}

func loadJPEG(t *testing.T) []byte {
	raw, err := os.ReadFile("testdata/sample.jpg")
	if err != nil {
		t.Fatalf("read sample.jpg: %v (cd into image/ to make testdata available)", err)
	}
	return raw
}

func TestUploadOne_StoragePutFailure_Surfaces(t *testing.T) {
	repo := &stubRepo{
		findFn:   func(_ context.Context, _, _ string) (*InsertedImage, error) { return nil, nil },
		insertFn: func(_ context.Context, _ InsertInput) (*InsertResult, error) { return &InsertResult{}, nil },
	}
	store := &stubStore{failPut: true}
	svc := &Service{store: store, images: repo, artworks: nil, ids: NewUUIDProvider()}
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}

	_, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err == nil {
		t.Fatal("expected storage Put error to surface")
	}
	if !strings.Contains(err.Error(), "storage Put failed") {
		t.Fatalf("expected wrapped storage error, got %v", err)
	}
	if len(store.deletes) != 0 {
		t.Fatalf("Delete must NOT fire on Put failure (orphan only on Insert failure); got deletes=%v", store.deletes)
	}
}

func TestUploadOne_InsertFails_OrphanIsDeleted(t *testing.T) {
	repo := &stubRepo{
		findFn: func(_ context.Context, _, _ string) (*InsertedImage, error) { return nil, nil },
		insertFn: func(_ context.Context, _ InsertInput) (*InsertResult, error) {
			return nil, errors.New("insert failure")
		},
	}
	store := &stubStore{}
	svc := &Service{store: store, images: repo, artworks: nil, ids: NewUUIDProvider()}
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}

	_, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err == nil {
		t.Fatal("expected insert error to surface")
	}
	if len(store.deletes) != 1 || store.deletes[0] == "" {
		t.Fatalf("expected one Delete for orphan cleanup, got deletes=%v", store.deletes)
	}
	if store.deletes[0] != store.puts[0] {
		t.Fatalf("orphan Delete key %q must equal Put key %q", store.deletes[0], store.puts[0])
	}
}

func TestUploadOne_IdempotentInsertButRowNotFound_ErrorsClearly(t *testing.T) {
	calls := 0
	repo := &stubRepo{
		findFn: func(_ context.Context, _, _ string) (*InsertedImage, error) {
			calls++
			// First call (pre-Insert): no row → keep going.
			// Second call (after idempotent Insert): also nil → triggers the error
			// at service.go line 109-111.
			return nil, nil
		},
		insertFn: func(_ context.Context, _ InsertInput) (*InsertResult, error) {
			return &InsertResult{ID: "imgX", Existed: true}, nil
		},
	}
	store := &stubStore{}
	svc := &Service{store: store, images: repo, artworks: nil, ids: NewUUIDProvider()}
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}

	_, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err == nil || !strings.Contains(err.Error(), "idempotent insert") {
		t.Fatalf("expected idempotent-insert sentinel error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 FindByClientImageID calls, got %d", calls)
	}
}

func TestCounterIDProvider_DeterministicSequence(t *testing.T) {
	p := &CounterIDProvider{}
	first := p.NewID()
	second := p.NewID()
	if first == second {
		t.Fatal("CounterIDProvider must return distinct IDs")
	}
	want := "00000000-0000-0000-0000-000000000001"
	if first != want {
		t.Fatalf("first ID = %q, want %q", first, want)
	}
}

func TestNewServiceWithIDs_UsesProvidedIDProvider(t *testing.T) {
	counter := &CounterIDProvider{}
	repo := &stubRepo{
		findFn:   func(_ context.Context, _, _ string) (*InsertedImage, error) { return nil, nil },
		insertFn: func(_ context.Context, in InsertInput) (*InsertResult, error) {
			// Echo back the ID supplied by the service so we can verify it came from the counter.
			return &InsertResult{ID: in.ID, Existed: false}, nil
		},
	}
	store := &stubStore{}
	svc := NewServiceWithIDs(nil, nil, nil, counter)
	// Verify the ids field was wired.
	if svc.ids != counter {
		t.Fatal("NewServiceWithIDs did not wire the provided IDProvider")
	}
	// Exercise one upload so the counter advances.
	svc.store = store
	svc.images = repo
	svc.artworks = nil
	art := &artwork.Artwork{ID: "art1", Visibility: "private"}
	res, err := svc.UploadOne(context.Background(), art, UploadOne{
		Manifest: ManifestEntry{ClientImageID: "K", ContentType: "image/jpeg", Position: 0},
		Body:     bytes.NewReader(loadJPEG(t)),
	})
	if err != nil {
		t.Fatalf("UploadOne with CounterIDProvider: %v", err)
	}
	wantID := "00000000-0000-0000-0000-000000000001"
	if res.Image.ID != wantID {
		t.Fatalf("image ID = %q, want %q", res.Image.ID, wantID)
	}
}
