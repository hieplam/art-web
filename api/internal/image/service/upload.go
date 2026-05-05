package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	imagedomain "local/art-web/api/internal/image/domain"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
)

// mimeForFormat maps image.Decode format names to MIME types.
var mimeForFormat = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
}

// imageRepo is a narrow seam for service-level tests. *imagepostgres.Repo satisfies it; this
// interface is not exported.
//
// IMPORTANT: signatures must match repo.go exactly. Insert returns
// (*InsertResult, error) — pointer to InsertResult — and FindByClientImageID
// returns nil (not the zero value) when the row is missing.
type imageRepo interface {
	FindByClientImageID(ctx context.Context, artworkID, clientImageID string) (*imagepostgres.InsertedImage, error)
	Insert(ctx context.Context, in imagepostgres.InsertInput) (*imagepostgres.InsertResult, error)
}

type Service struct {
	store    infrastorage.Storage
	images   imageRepo
	artworks *artworkpostgres.Repo
	ids      IDProvider
}

func NewService(s infrastorage.Storage, im *imagepostgres.Repo, a *artworkpostgres.Repo) *Service {
	return &Service{store: s, images: im, artworks: a, ids: NewUUIDProvider()}
}

// NewServiceWithIDs is the test-mode constructor. The contract suite passes a
// deterministic *CounterIDProvider here. Production uses NewService (which
// takes *imagepostgres.Repo). This constructor accepts the imageRepo interface directly so
// stub-based tests don't need to bypass it.
func NewServiceWithIDs(s infrastorage.Storage, im imageRepo, a *artworkpostgres.Repo, ids IDProvider) *Service {
	return &Service{store: s, images: im, artworks: a, ids: ids}
}

const MaxBytes = 25 * 1024 * 1024

type UploadOne struct {
	Manifest imagedomain.ManifestEntry
	Body     io.Reader
}

type UploadResult struct {
	Image   imagepostgres.InsertedImage
	Existed bool
}

var (
	ErrTooLarge            = errors.New("file exceeds 25 MB")
	ErrContentTypeMismatch = errors.New("body content type does not match declared manifest content_type")
)

func (s *Service) UploadOne(ctx context.Context, art *artworkpostgres.Artwork, in UploadOne) (*UploadResult, error) {
	limited := io.LimitReader(in.Body, MaxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(buf) > MaxBytes {
		return nil, ErrTooLarge
	}

	sum := sha256.Sum256(buf)
	sha := hex.EncodeToString(sum[:])

	// Spec §6.5 requires the fingerprint check before decode, blurhash, or storage writes.
	existing, err := s.images.FindByClientImageID(ctx, art.ID, in.Manifest.ClientImageID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.SourceSHA256 != sha {
			return nil, imagepostgres.ErrFingerprintMismatch
		}
		return &UploadResult{Image: *existing, Existed: true}, nil
	}

	dec, err := DecodeAndBlurhash(bytes.NewReader(buf), in.Manifest.ContentType)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if actual, ok := mimeForFormat[dec.Format]; ok && actual != in.Manifest.ContentType {
		return nil, ErrContentTypeMismatch
	}

	imgID := s.ids.NewID()
	ext, _ := imagedomain.ExtFor(in.Manifest.ContentType)
	key := fmt.Sprintf("%s/%s/%s.%s", art.Visibility, art.ID, imgID, ext)

	if err := s.store.Put(ctx, key, bytes.NewReader(buf), in.Manifest.ContentType); err != nil {
		return nil, err
	}

	res, err := s.images.Insert(ctx, imagepostgres.InsertInput{
		ID: imgID, ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
		Position: in.Manifest.Position, ContentType: in.Manifest.ContentType,
		StorageKey: key, SourceSHA256: sha,
		Width: dec.Width, Height: dec.Height,
		ByteSize: len(buf), Blurhash: dec.Blurhash,
	})
	if err != nil {
		_ = s.store.Delete(ctx, key) // best-effort: prevent orphan on concurrent-race insert failure
		return nil, err
	}
	if res.Existed {
		_ = s.store.Delete(ctx, key)
		existing, err := s.images.FindByClientImageID(ctx, art.ID, in.Manifest.ClientImageID)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, errors.New("idempotent insert returned existing row but row was not found")
		}
		return &UploadResult{Image: *existing, Existed: true}, nil
	}

	return &UploadResult{
		Image: imagepostgres.InsertedImage{
			ID: res.ID, ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
			StorageKey: key, ContentType: in.Manifest.ContentType, Blurhash: dec.Blurhash,
			SourceSHA256: sha,
			Position:     in.Manifest.Position, Width: dec.Width, Height: dec.Height, ByteSize: len(buf),
		},
		Existed: res.Existed,
	}, nil
}
