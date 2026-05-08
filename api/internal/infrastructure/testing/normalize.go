// api/internal/infrastructure/testing/normalize.go
package testing

import (
	"net/http"
	"regexp"
)

// Patterns are documented in spec §5.3.2.
var (
	// nextCursorRe matches the JSON field `"next_cursor":"<base64-RawURL>"`
	// and rewrites the value to <CURSOR>. The base64.RawURLEncoding alphabet
	// is [A-Za-z0-9_-] (no padding). The regex is anchored to the field name so
	// arbitrary base64-looking strings elsewhere in the body are NOT replaced.
	// `null` cursors are not matched (the regex requires a quoted string).
	nextCursorRe = regexp.MustCompile(`"next_cursor":"[A-Za-z0-9_-]+"`)
	uuidRe       = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	rfc3339Re    = regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z`)
	// signedURLRe matches the URLBuilder.Private output: ?sig=<hex>&exp=<unix>.
	// Real response bodies are emitted by Go's encoding/json with default HTML
	// escaping, which writes `&` as `&` in JSON string values. The regex
	// matches both forms so the rule fires whether the body bytes hold literal
	// `&` or the escaped sequence.
	signedURLRe = regexp.MustCompile(`\?sig=[0-9a-f]+(?:&|\\u0026)exp=\d+`)
	// cookieAuthRe is anchored with ^ so the rule ONLY rewrites the cookie's
	// own name=value at the start of a Set-Cookie value, never an unrelated
	// substring like `xauth=` or a Domain attribute that happens to contain
	// `auth=`. Each Set-Cookie response header carries one cookie + its
	// attributes separated by `; `, so anchoring at start is safe and precise.
	cookieAuthRe = regexp.MustCompile(`^auth=[^;]+`)
)

// NormalizeBody rewrites every non-deterministic byte sequence in body to a
// stable placeholder so post-normalization snapshots compare equal.
//
// The four patterns are mutually disjoint within a JSON body:
//   - nextCursorRe is anchored to `"next_cursor":"…"`; the value is base64
//     RawURL ([A-Za-z0-9_-]+) which contains no `:`, `T`, `Z`, or `?`.
//   - signedURLRe requires the literal `?sig=…&exp=…`; not in any other rule.
//   - rfc3339Re requires `T…:…:…Z`; not in cursor base64 or signed-URL.
//   - uuidRe requires hyphens at positions 8-13-18-23; doesn't match any other.
//
// So the call order below is documentary, not load-bearing — but reorder with
// care: any future addition that changes a pattern's character class needs to
// re-validate disjointness.
func NormalizeBody(body []byte) []byte {
	body = nextCursorRe.ReplaceAll(body, []byte(`"next_cursor":"<CURSOR>"`))
	body = signedURLRe.ReplaceAll(body, []byte(`?sig=<SIG>&exp=<EXP>`))
	body = rfc3339Re.ReplaceAll(body, []byte(`<TIMESTAMP>`))
	body = uuidRe.ReplaceAll(body, []byte(`<UUID>`))
	return body
}

// NormalizeHeaders rewrites the Set-Cookie auth=… value in place. The contract
// suite never produces an outgoing Authorization header (the API uses a cookie,
// not a Bearer token), so no Authorization rule is registered. If PR 0.7 ever
// adds Bearer-token endpoints, add a `Bearer …` rule then with a corresponding
// test — don't pre-add it here as dead code.
func NormalizeHeaders(h http.Header) {
	if vals, ok := h["Set-Cookie"]; ok {
		for i, v := range vals {
			vals[i] = cookieAuthRe.ReplaceAllString(v, "auth=<JWT>")
		}
	}
}
