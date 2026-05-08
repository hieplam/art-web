// Package log holds adapter implementations of the artwork slice's logging
// ports. Today it just provides ZerologReporter for ports.RollbackReporter;
// future log-shaped adapters belong here too.
package log

import (
	"context"

	"github.com/rs/zerolog"
)

// ZerologReporter writes flip-rollback failures to a zerolog.Logger. Production
// composes it with the app's main logger via Wire; tests can construct one
// with zerolog.Nop() when they don't care about output.
type ZerologReporter struct {
	log zerolog.Logger
}

// NewZerologReporter returns a reporter that emits at zerolog.Error level with
// the image_id and the underlying error.
func NewZerologReporter(log zerolog.Logger) *ZerologReporter {
	return &ZerologReporter{log: log}
}

// Report writes one structured error event. Matches the historical
// `artwork.RollbackLog` semantics: the caller fires this only when the
// compensating storage-rollback move itself fails (a leak window).
func (r *ZerologReporter) Report(_ context.Context, imageID string, err error) {
	r.log.Error().Err(err).Str("image_id", imageID).Msg("flip rollback")
}
