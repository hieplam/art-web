// api/internal/auth/google.go
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

type googleProvider struct {
	cfg         *oauth2.Config
	userinfoURL string
}

func NewGoogleProvider(clientID, clientSecret, redirectURL string) Provider {
	return &googleProvider{
		cfg: &oauth2.Config{
			ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURL,
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/v2/auth",
				TokenURL: "https://oauth2.googleapis.com/token",
			},
			Scopes: []string{"openid", "email", "profile"},
		},
		userinfoURL: "https://openidconnect.googleapis.com/v1/userinfo",
	}
}

func NewGoogleProviderForTest(id, secret, redirect, authURL, tokenURL, userinfoURL string) Provider {
	return &googleProvider{
		cfg: &oauth2.Config{
			ClientID: id, ClientSecret: secret, RedirectURL: redirect,
			Endpoint: oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
			Scopes:   []string{"openid", "email", "profile"},
		},
		userinfoURL: userinfoURL,
	}
}

func (g *googleProvider) Name() string { return "google" }

func (g *googleProvider) AuthURL(state string) string {
	return g.cfg.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *googleProvider) Exchange(ctx context.Context, code string) (*Profile, error) {
	tok, err := g.cfg.Exchange(ctx, code)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", g.userinfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("userinfo: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, errors.New("userinfo: " + resp.Status)
	}
	var u struct {
		Sub, Email, Name, Picture string
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return nil, err
	}
	if u.Sub == "" {
		return nil, errors.New("userinfo: empty sub")
	}
	return &Profile{
		Subject: u.Sub, Email: u.Email,
		DisplayName: strings.TrimSpace(u.Name),
		AvatarURL:   u.Picture,
	}, nil
}
