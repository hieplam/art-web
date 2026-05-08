// api/internal/infrastructure/database/providers.go
package database

import "github.com/google/wire"

var ProviderSet = wire.NewSet(
	NewGormDB,
	NewTransactor,
)
