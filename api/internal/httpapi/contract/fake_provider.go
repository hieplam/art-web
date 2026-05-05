// api/internal/httpapi/contract/fake_provider.go
package contract

import (
	"context"

	authports "local/art-web/api/internal/auth/ports"
)

// fakeGoogleProvider is a deterministic authports.OAuthProvider for the contract suite.
// Behavior chosen to exercise every auth response branch:
//
//	/auth/google/start                                    → 302 to a fixed AuthURL
//	/auth/google/callback?state=…&code=valid              → success path (UpsertOAuth + JWT)
//	/auth/google/callback?state=…&code=fail               → 502 exchange_failed
//	/auth/google/callback?state=mismatch                  → 400 bad_state (state-cookie check)
type fakeGoogleProvider struct{}

func (fakeGoogleProvider) Name() string { return "google" }

func (fakeGoogleProvider) AuthURL(state string) string {
	// State is dynamic by definition, but the contract harness already injects
	// a deterministic RandReader, so `state` is stable across runs.
	return "https://example.com/oauth2/auth?state=" + state
}

func (fakeGoogleProvider) Exchange(_ context.Context, code string) (*authports.Profile, error) {
	if code == "fail" {
		return nil, errFakeExchange
	}
	return &authports.Profile{
		Subject:     "fake-subject-1",
		Email:       "fake@example.com",
		DisplayName: "Fake User",
		AvatarURL:   "",
	}, nil
}

// errFakeExchange is the sentinel returned for code=fail; the auth handler
// maps any non-nil exchange error to 502 exchange_failed.
var errFakeExchange = &exchangeFailure{}

type exchangeFailure struct{}

func (*exchangeFailure) Error() string { return "fake exchange failure" }

// FakeProviders returns the providers map the contract suite hands to BootApp.
// Only "google" is wired today; if Phase 0 ever adds another OAuth provider
// for testing, add it here with the same deterministic-success/failure shape.
func FakeProviders() map[string]authports.OAuthProvider {
	return map[string]authports.OAuthProvider{"google": fakeGoogleProvider{}}
}
