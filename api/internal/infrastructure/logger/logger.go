// api/internal/infrastructure/logger/logger.go
package logger

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"local/art-web/api/internal/infrastructure/config"
)

// New returns a configured zerolog.Logger. Production callers use this; tests
// inject their own via the Wire test ProviderSet.
func New(cfg config.LoggerConfig) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)
	logger := zerolog.New(os.Stderr).With().Timestamp().Logger()
	log.Logger = logger
	return logger
}
