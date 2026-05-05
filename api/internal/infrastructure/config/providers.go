// api/internal/infrastructure/config/providers.go
package config

import "github.com/google/wire"

// ProviderSet exposes AppConfig + each sub-config via FieldsOf so consumers
// receive only the slice they need.
var ProviderSet = wire.NewSet(
	Load,
	wire.FieldsOf(new(*AppConfig),
		"Server", "Database", "Auth", "Storage", "Image", "Logger",
	),
)
