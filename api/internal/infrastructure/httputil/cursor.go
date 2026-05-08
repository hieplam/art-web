// Package httputil holds tiny chi-aware helpers shared across slice HTTP
// adapters. Today it just hosts the cursor parse/encode and the limit parser
// — both used by /artworks, /tags, and /users feed endpoints. Lives outside
// the slices because no single slice owns it.
package httputil

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	artworkports "local/art-web/api/internal/artwork/ports"
)

// ParseLimit reads ?limit from the request, clamps to the historical [1,100]
// range, and falls back to 24 (the legacy default) when the param is missing
// or out of range.
func ParseLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 100 {
		return 24
	}
	return n
}

// ParseCursor decodes the ?cursor base64 token into a FeedCursor. An empty
// cursor maps to the zero value (first page). A malformed cursor returns an
// error whose message becomes the bad_cursor "message" field.
func ParseCursor(r *http.Request) (artworkports.FeedCursor, error) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return artworkports.FeedCursor{}, nil
	}
	dec, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return artworkports.FeedCursor{}, fmt.Errorf("bad_cursor: %w", err)
	}
	parts := strings.SplitN(string(dec), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return artworkports.FeedCursor{}, errors.New("bad_cursor: missing field")
	}
	stamp, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return artworkports.FeedCursor{}, fmt.Errorf("bad_cursor: timestamp %w", err)
	}
	return artworkports.FeedCursor{Stamp: stamp.UTC(), ID: parts[1]}, nil
}

// EncodeCursor renders a FeedCursor as the base64 token clients echo back as
// ?cursor= on the next page request. nil → nil (no more pages).
func EncodeCursor(c *artworkports.FeedCursor) *string {
	if c == nil {
		return nil
	}
	raw := c.Stamp.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	enc := base64.RawURLEncoding.EncodeToString([]byte(raw))
	return &enc
}
