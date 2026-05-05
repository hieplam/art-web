// api/internal/artwork/adapters/log/rollback_test.go
package log_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	artworklog "local/art-web/api/internal/artwork/adapters/log"
)

// TestZerologReporter_Report_EmitsErrorEvent asserts the reporter writes a
// structured error log line with the image_id field and the underlying error
// when called. The Report method is the leak-window instrumentation called
// from VisibilityService.rollbackMoves when a compensating undo move fails.
func TestZerologReporter_Report_EmitsErrorEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Logger()
	r := artworklog.NewZerologReporter(logger)

	r.Report(context.Background(), "img-42", errors.New("undo failed"))

	out := buf.String()
	if !strings.Contains(out, `"image_id":"img-42"`) {
		t.Errorf("missing image_id field in output: %s", out)
	}
	if !strings.Contains(out, `"error":"undo failed"`) {
		t.Errorf("missing error field in output: %s", out)
	}
	if !strings.Contains(out, `"flip rollback"`) {
		t.Errorf("missing static message in output: %s", out)
	}
}
