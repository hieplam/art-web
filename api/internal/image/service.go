package image

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/storage"
)

// mimeForFormat maps image.Decode format names to MIME types.
var mimeForFormat = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
}

type Service struct {
	store    storage.Storage
	images   *Repo
	artworks *artwork.Repo
}

func NewService(s storage.Storage, im *Repo, a *artwork.Repo) *Service {
	return &Service{store: s, images: im, artworks: a}
}

const MaxBytes = 25 * 1024 * 1024

type UploadOne struct {
	Manifest ManifestEntry
	Body     io.Reader
}

type UploadResult struct {
	Image   InsertedImage
	Existed bool
}

var (
	ErrTooLarge          = errors.New("file exceeds 25 MB")
	ErrContentTypeMismatch = errors.New("body content type does not match declared manifest content_type")
)

func (s *Service) UploadOne(ctx context.Context, art *artwork.Artwork, in UploadOne) (*UploadResult, error) {
	limited := io.LimitReader(in.Body, MaxBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(buf) > MaxBytes {
		return nil, ErrTooLarge
	}
	dec, err := DecodeAndBlurhash(bytes.NewReader(buf), in.Manifest.ContentType)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if actual, ok := mimeForFormat[dec.Format]; ok && actual != in.Manifest.ContentType {
		return nil, ErrContentTypeMismatch
	}

	sum := sha256.Sum256(buf)
	sha := hex.EncodeToString(sum[:])

	imgID := uuid.NewString()
	ext, _ := ExtFor(in.Manifest.ContentType)
	key := fmt.Sprintf("%s/%s/%s.%s", art.Visibility, art.ID, imgID, ext)

	// On idempotent retry (same client_image_id + same SHA256), Insert returns
	// Existed=true but `key` is a duplicate object never referenced by the DB.
	// TODO post-v1: track and sweep in R2 GC (spec §11).
	if err := s.store.Put(ctx, key, bytes.NewReader(buf), in.Manifest.ContentType); err != nil {
		return nil, err
	}

	res, err := s.images.Insert(ctx, InsertInput{
		ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
		Position: in.Manifest.Position, ContentType: in.Manifest.ContentType,
		StorageKey: key, SourceSHA256: sha,
		Width: dec.Width, Height: dec.Height,
		ByteSize: len(buf), Blurhash: dec.Blurhash,
	})
	if err != nil {
		_ = s.store.Delete(ctx, key) // best-effort: prevent orphan on insert failure
		return nil, err
	}

	return &UploadResult{
		Image: InsertedImage{
			ID: res.ID, ArtworkID: art.ID, ClientImageID: in.Manifest.ClientImageID,
			StorageKey: key, ContentType: in.Manifest.ContentType, Blurhash: dec.Blurhash,
			SourceSHA256: sha,
			Position: in.Manifest.Position, Width: dec.Width, Height: dec.Height, ByteSize: len(buf),
		},
		Existed: res.Existed,
	}, nil
}
