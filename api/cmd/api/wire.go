//go:build wireinject
// +build wireinject

// api/cmd/api/wire.go
package main

import (
	"context"

	"github.com/google/wire"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	infraconfig "local/art-web/api/internal/infrastructure/config"
	"local/art-web/api/internal/infrastructure/database"
	"local/art-web/api/internal/infrastructure/logger"
	// auth, user, artwork, image, storage, server ProviderSets fill in
	// from Tasks 6+ as their per-slice providers.go files land.
)

// App is the root composition wired by Wire — currently config + logger + DB
// only. The four slice ProviderSets (auth, user, artwork, image) plus
// server/storage are assembled by hand in main.go below.
//
// SCOPE NOTE (Phase 1 Task 11 carry-over): full slice integration here
// requires typed-string providers for JWT_SIGNING_KEY, WORKER_SIGNING_KEY,
// CookieDomain, AllowedOrigin, FrontendURL, R2Bucket, GoogleClientID/Secret/
// RedirectURL — i.e. a Wire-shaped re-encoding of the cmd/api/config.go
// flattening that main() already does. The decision for Phase 1: keep the
// manual wiring in main() and let Wire own only the cross-cutting infra
// providers, because (a) the manual wiring is one linear pass that's easy
// to read and step through, and (b) adding typed-string providers for every
// env var doubles the surface area without changing runtime behaviour. A
// future phase that introduces a proper config-as-struct provider can move
// to full Wire composition without touching the slices themselves.
type App struct {
	DB     *gorm.DB
	Logger zerolog.Logger
}

// InitializeApp returns the assembled *App + cleanup func. Wire generates the
// body in wire_gen.go.
func InitializeApp(ctx context.Context) (*App, func(), error) {
	wire.Build(
		infraconfig.ProviderSet,
		logger.ProviderSet,
		// database.ProviderSet provides NewGormDB + NewTransactor; the
		// transactor is currently unconsumed (no field on App, no downstream
		// dep), so Wire elides it from wire_gen.go. The slice repos in
		// main.go bind their own *gorm.DB directly.
		database.ProviderSet,
		wire.Struct(new(App), "*"),
	)
	return nil, nil, nil // wire fills these in
}
