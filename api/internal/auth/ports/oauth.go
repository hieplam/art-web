// api/internal/auth/ports/oauth.go
package ports

import "context"

type Profile struct {
	Subject     string
	Email       string
	DisplayName string
	AvatarURL   string
}

type Provider interface {
	Name() string
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*Profile, error)
}
