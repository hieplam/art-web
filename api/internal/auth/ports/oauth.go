// Package ports defines the auth slice's outbound interfaces. Implementations
// live under adapters/oauth/, adapters/http/, and so on. Ports import only the
// slice's domain package — never gorm, chi, zerolog, or any adapter package.
package ports

import (
	"context"
	"time"
)

// Profile is the OAuth-provider-supplied profile after a successful Exchange.
// Mirrors the historical auth.Profile struct.
type Profile struct {
	Subject     string
	Email       string
	DisplayName string
	AvatarURL   string
}

// OAuthProvider is the abstraction over a single identity provider (e.g.
// Google). Each implementation handles the authorize-URL build + the code-for-
// profile exchange.
type OAuthProvider interface {
	Name() string
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*Profile, error)
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
