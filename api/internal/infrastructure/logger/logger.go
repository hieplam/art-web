// api/internal/infrastructure/logger/logger.go
package logger

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"local/art-web/api/internal/infrastructure/config"
)

// New returns a configured zerolog.Logger and mutates two zerolog globals:
// it calls zerolog.SetGlobalLevel and assigns log.Logger so any third-party
// code that uses zerolog/log directly picks up the same level + sink.
//
// Call once at startup. Calling from tests permanently mutates global state
// for any concurrent or subsequent tests in the same process; tests that
// need a custom logger should construct a zerolog.Logger directly rather
// than going through this function.
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
