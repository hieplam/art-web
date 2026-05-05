// Package ports defines the auth slice's outbound interfaces. Implementations
// live under adapters/oauth/, adapters/http/, and so on. Ports import only the
// slice's domain package — never gorm, chi, zerolog, or any adapter package.
package ports

import (
	"context"
	"time"

	"local/art-web/api/internal/auth/domain"
)

// OAuthProvider is the abstraction over a single identity provider (e.g.
// Google). Each implementation handles the authorize-URL build + the code-for-
// identity exchange. Returns the canonical domain.Identity from auth/domain.
type OAuthProvider interface {
	Name() string
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*domain.Identity, error)
}

// UserLookup lets the auth handlers consult the user slice without importing
// it directly. Implemented by user/adapters/postgres.Repo.UpsertOAuth.
type UserLookup interface {
	UpsertOAuth(ctx context.Context, provider, subject, email, displayName, avatarURL string) (userID string, err error)
}

// JWTIssuer issues + verifies user-bound JSON Web Tokens. Implemented by
// auth/service.JWT.
type JWTIssuer interface {
	Issue(userID string, ttl time.Duration) (string, error)
	Verify(token string) (string, error)
}
