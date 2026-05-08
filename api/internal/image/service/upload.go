package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	artworkdomain "local/art-web/api/internal/artwork/domain"
	imagedomain "local/art-web/api/internal/image/domain"
	imageports "local/art-web/api/internal/image/ports"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
)

// mimeForFormat maps image.Decode format names to MIME types.
var mimeForFormat = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
}

// imageRepo is the narrow seam for service-level tests. Aliased to
// ports.ImageRepository so the postgres adapter satisfies it directly. Kept
// unexported so consumers see only the constructor signatures.
type imageRepo = imageports.ImageRepository

// ErrFingerprintMismatch is the canonical sentinel for the same-client_image_id-
// but-different-bytes retry case. The postgres adapter returns the same value
// via re-export, so handlers that errors.Is against either name match.
var ErrFingerprintMismatch = imagedomain.ErrFingerprintMismatch

type Service struct {
	store    infrastorage.Storage
	images   imageRepo
	artworks ArtworkLookup
	ids      IDProvider
}

// ArtworkLookup is the service's narrow view of the artwork repo — it only
// needs Get for upload-time validation. The artwork postgres adapter
// satisfies this structurally.
type ArtworkLookup interface {
	Get(ctx context.Context, id string) (*artworkdomain.Artwork, error)
}

// NewService wires the upload service against ports-typed collaborators. The
// concrete *imagepostgres.Repo and *artworkpostgres.Repo satisfy these
// interfaces, so existing callers compile without conversion.
func NewService(s infrastorage.Storage, im imageRepo, a ArtworkLookup) *Service {
	return &Service{store: s, images: im, artworks: a, ids: NewUUIDProvider()}
}

// NewServiceWithIDs is the test-mode constructor with an injectable IDProvider.
// The contract suite passes a deterministic *CounterIDProvider here.
func NewServiceWithIDs(s infrastorage.Storage, im imageRepo, a ArtworkLookup, ids IDProvider) *Service {
	return &Service{store: s, images: im, artworks: a, ids: ids}
}

const MaxBytes = 25 * 1024 * 1024

type UploadOne struct {
	Manifest imagedomain.ManifestEntry
	Body     io.Reader
}

type UploadResult struct {
	Image   imagedomain.Image
	Existed bool
}

var (
	ErrTooLarge            = errors.New("file exceeds 25 MB")
	ErrContentTypeMismatch = errors.New("body content type does not match declared manifest content_type")
)

func (s *Service) UploadOne(ctx context.Context, art *artworkdomain.Artwork, in UploadOne) (*UploadResult, error) {
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
			return nil, ErrFingerprintMismatch
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

	res, err := s.images.Insert(ctx, imageports.InsertInput{
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
		Image: imagedomain.Image{
			ID: res.ID, ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
			StorageKey: key, ContentType: in.Manifest.ContentType, Blurhash: dec.Blurhash,
			SourceSHA256: sha,
			Position:     in.Manifest.Position, Width: dec.Width, Height: dec.Height, ByteSize: len(buf),
		},
		Existed: res.Existed,
	}, nil
}
