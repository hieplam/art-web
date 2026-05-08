// api/internal/user/adapters/postgres/repo.go
package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"local/art-web/api/internal/infrastructure/database"
	userdomain "local/art-web/api/internal/user/domain"
)

// User is the persistence-shape entity returned by Repo. Aliased to the
// domain entity so handlers can read fields directly without an explicit
// mapping step.
type User = userdomain.User

// ErrNotFound is returned by Get when the requested user row does not exist.
// It lets callers distinguish "JWT subject vanished" (return 401) from a
// generic DB failure (return 500) without depending on driver error sentinels.
var ErrNotFound = errors.New("user not found")

// Repo is the user slice's GORM-backed persistence adapter.
type Repo struct{ db *gorm.DB }

// NewRepo constructs a repo with the root *gorm.DB. Inside request scope, the
// repo retrieves the active handle (root or transactional) via
// database.DB(ctx, r.db) so spec §7.6.1's transactor pattern works seamlessly.
func NewRepo(db *gorm.DB) *Repo { return &Repo{db: db} }

// DB returns the underlying root *gorm.DB. Tests use this to seed fixtures
// directly; production code should not reach into it.
func (r *Repo) DB() *gorm.DB { return r.db }

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// Slugify mirrors the previous public function; tests reference it directly.
func Slugify(s string) string {
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
	db := database.DB(ctx, r.db).WithContext(ctx)

	// Existing user with same (provider, subject)?
	var existing userModel
	err := db.Where("oauth_provider = ? AND oauth_subject = ?", provider, subject).First(&existing).Error
	if err == nil {
		updates := map[string]any{
			"email":        email,
			"display_name": displayName,
		}
		if avatar != "" {
			updates["avatar_url"] = avatar
		} else {
			updates["avatar_url"] = nil
		}
		if err := db.Model(&userModel{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return "", fmt.Errorf("user.Update: %w", err)
		}
		return existing.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("user.Lookup: %w", err)
	}

	base := Slugify(displayName)
	slug := base
	var avatarPtr *string
	if avatar != "" {
		avatarPtr = &avatar
	}
	for i := 0; i < 50; i++ {
		m := userModel{
			OAuthProvider: provider,
			OAuthSubject:  subject,
			Email:         email,
			DisplayName:   displayName,
			Slug:          slug,
			AvatarURL:     avatarPtr,
		}
		err := db.Create(&m).Error
		if err == nil {
			return m.ID, nil
		}
		switch uniqueConstraint(err) {
		case "users_slug_key":
			slug = fmt.Sprintf("%s-%d", base, i+2)
			continue
		case "users_oauth_provider_oauth_subject_key":
			// Race: someone else inserted between our Lookup and Create.
			if err := db.Where("oauth_provider = ? AND oauth_subject = ?", provider, subject).
				First(&existing).Error; err == nil {
				return existing.ID, nil
			}
			return "", errors.New("oauth conflict but row not found on re-read")
		default:
			return "", fmt.Errorf("user.UpsertOAuth: %w", err)
		}
	}
	return "", errors.New("slug exhausted")
}

func (r *Repo) Get(ctx context.Context, id string) (*User, error) {
	var m userModel
	err := database.DB(ctx, r.db).WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return toDomainUser(&m), nil
}

func (r *Repo) GetBySlug(ctx context.Context, slug string) (*User, error) {
	var m userModel
	if err := database.DB(ctx, r.db).WithContext(ctx).First(&m, "slug = ?", slug).Error; err != nil {
		// Per spec §7.2.1 + §3, GetBySlug does NOT translate to ErrNotFound;
		// the handler treats any error as 404. Pin via repo_test.go.
		return nil, err
	}
	return toDomainUser(&m), nil
}

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
