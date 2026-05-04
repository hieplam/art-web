// api/internal/user/repo.go
package user

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID, Slug, DisplayName, Email, AvatarURL string
}

// ErrNotFound is returned by Get when the requested user row does not exist.
// It lets callers distinguish "JWT subject vanished" (return 401) from a
// generic DB failure (return 500) without depending on pgx error sentinels.
var ErrNotFound = errors.New("user not found")

type Repo struct{ pool *pgxpool.Pool }

func NewRepo(p *pgxpool.Pool) *Repo { return &Repo{pool: p} }

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "user"
	}
	if len(s) > 32 {
		s = s[:32]
	}
	return s
}

func (r *Repo) UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatar string) (string, error) {
	if id, ok, err := r.lookupExistingOAuth(ctx, provider, subject); err != nil {
		return "", err
	} else if ok {
		_, err := r.pool.Exec(ctx,
			`UPDATE users SET email=$1, display_name=$2, avatar_url=NULLIF($3,'') WHERE id=$4`,
			email, displayName, avatar, id)
		if err != nil {
			return "", err
		}
		return id, nil
	}

	base := slugify(displayName)
	slug := base
	for i := 0; i < 50; i++ {
		var id string
		err := r.pool.QueryRow(ctx, `
			INSERT INTO users (oauth_provider, oauth_subject, email, display_name, slug, avatar_url)
			VALUES ($1,$2,$3,$4,$5, NULLIF($6,''))
			RETURNING id`,
			provider, subject, email, displayName, slug, avatar).Scan(&id)
		if err == nil {
			return id, nil
		}
		switch uniqueConstraint(err) {
		case "users_slug_key":
			slug = fmt.Sprintf("%s-%d", base, i+2)
			continue
		case "users_oauth_provider_oauth_subject_key":
			if id, ok, lerr := r.lookupExistingOAuth(ctx, provider, subject); lerr != nil {
				return "", lerr
			} else if ok {
				return id, nil
			}
			return "", errors.New("oauth conflict but row not found on re-read")
		default:
			return "", err
		}
	}
	return "", errors.New("slug exhausted")
}

func (r *Repo) lookupExistingOAuth(ctx context.Context, provider, subject string) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx,
		`SELECT id FROM users WHERE oauth_provider=$1 AND oauth_subject=$2`,
		provider, subject).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return "", false, err
}

func (r *Repo) Get(ctx context.Context, id string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, slug, display_name, email, COALESCE(avatar_url,'') FROM users WHERE id=$1`,
		id).Scan(&u.ID, &u.Slug, &u.DisplayName, &u.Email, &u.AvatarURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

func (r *Repo) GetBySlug(ctx context.Context, slug string) (*User, error) {
	var u User
	err := r.pool.QueryRow(ctx,
		`SELECT id, slug, display_name, email, COALESCE(avatar_url,'') FROM users WHERE slug=$1`,
		slug).Scan(&u.ID, &u.Slug, &u.DisplayName, &u.Email, &u.AvatarURL)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

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
