// api/internal/image/adapters/postgres/repo.go
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	imagedomain "local/art-web/api/internal/image/domain"
	imageports "local/art-web/api/internal/image/ports"
	"local/art-web/api/internal/infrastructure/database"
)

// uniqueConstraint inspects an error from GORM's postgres driver and returns
// the violated constraint name on a 23505 unique violation; empty otherwise.
// GORM wraps pgconn errors, so errors.As against *pgconn.PgError still works.
func uniqueConstraint(err error) string {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return ""
	}
	if pgErr.SQLState() != "23505" {
		return ""
	}
	return pgErr.ConstraintName
}

// Re-export the domain sentinels so existing handler call sites that match
// imagepostgres.ErrPositionTaken / imagepostgres.ErrFingerprintMismatch keep
// compiling. The canonical values live in the domain package.
var (
	ErrPositionTaken       = imagedomain.ErrPositionTaken
	ErrFingerprintMismatch = imagedomain.ErrFingerprintMismatch
)

// Aliases so existing call sites compile against either the postgres or ports
// type names. The struct definitions live in ports (so ports doesn't import an
// adapter), and the persistence shape is identical to imagedomain.Image.
type (
	InsertInput   = imageports.InsertInput
	InsertResult  = imageports.InsertResult
	InsertedImage = imagedomain.Image
)

// Repo is the image slice's GORM-backed persistence adapter.
type Repo struct{ db *gorm.DB }

func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB exposes the underlying root *gorm.DB for tests that need to seed
// fixtures directly. Production code should not use it.
func (r *Repo) DB() *gorm.DB { return r.db }

func (r *Repo) Insert(ctx context.Context, in InsertInput) (*InsertResult, error) {
	if in.SourceSHA256 == "" {
		return nil, errors.New("SourceSHA256 is required")
	}
	db := database.DB(ctx, r.db).WithContext(ctx)

	// Idempotency pre-check: if a row already exists for the same client_image_id,
	// either return its ID (matching SHA) or surface a fingerprint mismatch.
	var existing imageModel
	err := db.Where("artwork_id = ? AND client_image_id = ?", in.ArtworkID, in.ClientImageID).
		First(&existing).Error
	if err == nil {
		if existing.SourceSHA256 == in.SourceSHA256 {
			return &InsertResult{ID: existing.ID, Existed: true}, nil
		}
		return nil, ErrFingerprintMismatch
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var blurPtr *string
	if in.Blurhash != "" {
		b := in.Blurhash
		blurPtr = &b
	}
	m := imageModel{
		ID:            in.ID,
		ArtworkID:     in.ArtworkID,
		ClientImageID: in.ClientImageID,
		StorageKey:    in.StorageKey,
		SourceSHA256:  in.SourceSHA256,
		Width:         in.Width,
		Height:        in.Height,
		ByteSize:      in.ByteSize,
		ContentType:   in.ContentType,
		Position:      in.Position,
		Blurhash:      blurPtr,
	}
	err = db.Create(&m).Error
	if err == nil {
		return &InsertResult{ID: m.ID, Existed: false}, nil
	}

	switch uniqueConstraint(err) {
	case "artwork_images_artwork_id_client_image_id_key":
		// Concurrent insert won the race for the same client_image_id;
		// re-read and decide whether bytes match.
		err := db.Where("artwork_id = ? AND client_image_id = ?", in.ArtworkID, in.ClientImageID).
			First(&existing).Error
		if err != nil {
			return nil, err
		}
		if existing.SourceSHA256 != in.SourceSHA256 {
			return nil, ErrFingerprintMismatch
		}
		return &InsertResult{ID: existing.ID, Existed: true}, nil
	case "artwork_images_artwork_id_position_key":
		return nil, ErrPositionTaken
	default:
		return nil, fmt.Errorf("image.Insert: %w", err)
	}
}

// FindByClientImageID returns the stored row for (artworkID, clientImageID),
// or nil if none exists.
func (r *Repo) FindByClientImageID(ctx context.Context, artworkID, clientImageID string) (*InsertedImage, error) {
	var m imageModel
	err := database.DB(ctx, r.db).WithContext(ctx).
		Where("artwork_id = ? AND client_image_id = ?", artworkID, clientImageID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return toDomainImage(&m), nil
}

func (r *Repo) ListByArtwork(ctx context.Context, artworkID string) ([]InsertedImage, error) {
	var rows []imageModel
	err := database.DB(ctx, r.db).WithContext(ctx).
		Where("artwork_id = ?", artworkID).
		Order("position").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]InsertedImage, 0, len(rows))
	for _, m := range rows {
		out = append(out, *toDomainImage(&m))
	}
	return out, nil
}
