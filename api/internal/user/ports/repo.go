// Package ports defines the user slice's outbound interfaces. Implementations
// live under adapters/postgres/. Ports import only the slice's domain package.
package ports

import (
	"context"

	userdomain "local/art-web/api/internal/user/domain"
)

// UserRepository is the user slice's persistence port. The concrete adapter
// is user/adapters/postgres.Repo.
type UserRepository interface {
	UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatar string) (string, error)
	Get(ctx context.Context, id string) (*userdomain.User, error)
	GetBySlug(ctx context.Context, slug string) (*userdomain.User, error)
}
