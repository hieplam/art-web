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

// App is the root composition. Cleanup function shuts down server, then DB.
// NOTE: Server *http.Server is omitted here; it has no provider yet and will
// be added in Task 7 when server.ProviderSet lands.
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
		// dep), so Wire elides it from wire_gen.go. Repos in Task 7 will
		// activate it.
		database.ProviderSet,
		// later: storage.ProviderSet, auth.ProviderSet, user.ProviderSet,
		//        artwork.ProviderSet, image.ProviderSet, server.ProviderSet,
		wire.Struct(new(App), "*"),
	)
	return nil, nil, nil // wire fills these in
}
