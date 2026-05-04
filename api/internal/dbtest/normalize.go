// api/internal/dbtest/normalize.go
package dbtest

import (
	"net/http"
	"regexp"
)

// Patterns are documented in spec §5.3.2. Order matters; see NormalizeBody.
var (
	// nextCursorRe matches the JSON field `"next_cursor":"<base64-RawURL>"`
	// and rewrites the value to <CURSOR>. The base64.RawURLEncoding alphabet
	// is [A-Za-z0-9_-] (no padding). The regex is anchored to the field name so
	// arbitrary base64-looking strings elsewhere in the body are NOT replaced.
	// `null` cursors are not matched (the regex requires a quoted string).
	nextCursorRe = regexp.MustCompile(`"next_cursor":"[A-Za-z0-9_\-]+"`)
	uuidRe       = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	rfc3339Re    = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)
	signedURLRe  = regexp.MustCompile(`\?sig=[0-9a-f]+&exp=\d+`)
	cookieAuthRe = regexp.MustCompile(`auth=[^;]+`)
	jwtBearerRe  = regexp.MustCompile(`Bearer [A-Za-z0-9_\-\.]+`)
)

// NormalizeBody rewrites every non-deterministic byte sequence in body to a
// stable placeholder so post-normalization snapshots compare equal.
//
// Order matters:
//  1. nextCursorRe runs first so the opaque base64 cursor is collapsed before
//     any later rule could match digits or hyphens inside it.
//  2. signedURLRe runs before rfc3339Re because the signed-URL `exp=` integer
//     would otherwise be left as a bare digit run.
//  3. rfc3339Re runs before uuidRe because timestamps' digit groups don't
//     overlap with the UUID pattern, but explicit ordering documents intent.
func NormalizeBody(body []byte) []byte {
	body = nextCursorRe.ReplaceAll(body, []byte(`"next_cursor":"<CURSOR>"`))
	body = signedURLRe.ReplaceAll(body, []byte(`?sig=<SIG>&exp=<EXP>`))
	body = rfc3339Re.ReplaceAll(body, []byte(`<TIMESTAMP>`))
	body = uuidRe.ReplaceAll(body, []byte(`<UUID>`))
	return body
}

// NormalizeHeaders rewrites Set-Cookie and Authorization headers in place.
func NormalizeHeaders(h http.Header) {
	if vals, ok := h["Set-Cookie"]; ok {
		for i, v := range vals {
			vals[i] = cookieAuthRe.ReplaceAllString(v, "auth=<JWT>")
		}
	}
	if vals, ok := h["Authorization"]; ok {
		for i, v := range vals {
			vals[i] = jwtBearerRe.ReplaceAllString(v, "Bearer <JWT>")
		}
	}
}
