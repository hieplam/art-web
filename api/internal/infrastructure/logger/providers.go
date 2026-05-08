// api/internal/infrastructure/logger/providers.go
package logger

import "github.com/google/wire"

var ProviderSet = wire.NewSet(New)
