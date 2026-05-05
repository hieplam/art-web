package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

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

var (
	ErrPositionTaken       = errors.New("position already taken by a different client_image_id")
	ErrFingerprintMismatch = errors.New("client_image_id reused with a different file (sha256 mismatch)")
)

type InsertInput struct {
	ID, ArtworkID, ClientImageID, ContentType, StorageKey, Blurhash, SourceSHA256 string
	Position, Width, Height, ByteSize                                             int
}

type InsertResult struct {
	ID      string
	Existed bool
}

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(p *pgxpool.Pool) *Repo { return &Repo{pool: p} }

func (r *Repo) Insert(ctx context.Context, in InsertInput) (*InsertResult, error) {
	if in.SourceSHA256 == "" {
		return nil, errors.New("SourceSHA256 is required")
	}
	var existingID, existingSHA string
	err := r.pool.QueryRow(ctx,
		`SELECT id, source_sha256 FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`,
		in.ArtworkID, in.ClientImageID).Scan(&existingID, &existingSHA)
	if err == nil {
		if existingSHA == in.SourceSHA256 {
			return &InsertResult{ID: existingID, Existed: true}, nil
		}
		return nil, ErrFingerprintMismatch
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var id string
	if in.ID != "" {
		err = r.pool.QueryRow(ctx, `
			INSERT INTO artwork_images
			  (id, artwork_id, client_image_id, storage_key, source_sha256,
			   width, height, byte_size, content_type, position, blurhash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10, NULLIF($11,''))
			RETURNING id`,
			in.ID, in.ArtworkID, in.ClientImageID, in.StorageKey, in.SourceSHA256,
			in.Width, in.Height, in.ByteSize, in.ContentType, in.Position, in.Blurhash).Scan(&id)
	} else {
		err = r.pool.QueryRow(ctx, `
			INSERT INTO artwork_images
			  (artwork_id, client_image_id, storage_key, source_sha256,
			   width, height, byte_size, content_type, position, blurhash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, NULLIF($10,''))
			RETURNING id`,
			in.ArtworkID, in.ClientImageID, in.StorageKey, in.SourceSHA256,
			in.Width, in.Height, in.ByteSize, in.ContentType, in.Position, in.Blurhash).Scan(&id)
	}
	if err == nil {
		return &InsertResult{ID: id, Existed: false}, nil
	}

	switch uniqueConstraint(err) {
	case "artwork_images_artwork_id_client_image_id_key":
		err = r.pool.QueryRow(ctx,
			`SELECT id, source_sha256 FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`,
			in.ArtworkID, in.ClientImageID).Scan(&existingID, &existingSHA)
		if err != nil {
			return nil, err
		}
		if existingSHA != in.SourceSHA256 {
			return nil, ErrFingerprintMismatch
		}
		return &InsertResult{ID: existingID, Existed: true}, nil
	case "artwork_images_artwork_id_position_key":
		return nil, ErrPositionTaken
	default:
		return nil, err
	}
}

// FindByClientImageID returns the stored row for (artworkID, clientImageID), or nil if none exists.
func (r *Repo) FindByClientImageID(ctx context.Context, artworkID, clientImageID string) (*InsertedImage, error) {
	var im InsertedImage
	err := r.pool.QueryRow(ctx, `
		SELECT id, artwork_id, client_image_id, storage_key, source_sha256,
		       width, height, byte_size, content_type, position, COALESCE(blurhash,'')
		FROM artwork_images WHERE artwork_id=$1 AND client_image_id=$2`,
		artworkID, clientImageID).Scan(
		&im.ID, &im.ArtworkID, &im.ClientImageID, &im.StorageKey, &im.SourceSHA256,
		&im.Width, &im.Height, &im.ByteSize, &im.ContentType, &im.Position, &im.Blurhash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &im, nil
}

func (r *Repo) ListByArtwork(ctx context.Context, artworkID string) ([]InsertedImage, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, artwork_id, client_image_id, storage_key, source_sha256,
		       width, height, byte_size, content_type, position, COALESCE(blurhash,'')
		FROM artwork_images WHERE artwork_id = $1 ORDER BY position`, artworkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InsertedImage
	for rows.Next() {
		var im InsertedImage
		if err := rows.Scan(&im.ID, &im.ArtworkID, &im.ClientImageID, &im.StorageKey, &im.SourceSHA256,
			&im.Width, &im.Height, &im.ByteSize, &im.ContentType, &im.Position, &im.Blurhash); err != nil {
			return nil, err
		}
		out = append(out, im)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

type InsertedImage struct {
	ID, ArtworkID, ClientImageID, StorageKey, ContentType, Blurhash, SourceSHA256 string
	Position, Width, Height, ByteSize                                             int
}
