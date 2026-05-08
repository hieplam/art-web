// Package httperr provides the single WriteError boundary for all HTTP
// handlers. It maps domain and infrastructure errors to the captured
// (status, code) pairs from error_codes_observed.md (spec §7.4).
//
// This package lives outside infrastructure/server to avoid the import cycle
// that would arise if server (which imports slice routers) also defined
// WriteError (which slice routers import).
package httperr

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"

	artworkdomain "local/art-web/api/internal/artwork/domain"
	authdomain "local/art-web/api/internal/auth/domain"
	imagedomain "local/art-web/api/internal/image/domain"
	imageservice "local/art-web/api/internal/image/service"
	userdomain "local/art-web/api/internal/user/domain"
)

// HTTPError matches the current API's two distinct shapes (see spec §3):
//
//	{ "error": "<code>" }                          — most paths (Shape A)
//	{ "error": "<code>", "message": "<detail>" }   — parse/cursor errors (Shape B)
type HTTPError struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// HTTP-layer sentinels — handlers pass these to WriteError to map onto the
// captured (status, code) pairs from error_codes_observed.md. Domain-layer
// sentinels (auth/domain.ErrBadState, image/domain.ErrPositionTaken, etc.)
// are recognized directly and do not need package-local mirrors.
var (
	// ErrBadJSON: JSON decode failure on a request body. Maps to 400 bad_json.
	ErrBadJSON = errors.New("bad json")
	// ErrBadCursor: cursor parameter could not be parsed. Use WrapBadCursor
	// to attach the underlying parse-error message for the Shape-B response.
	ErrBadCursor = errors.New("bad cursor")
	// ErrUnauthorized: caller required to authenticate but did not.
	// Maps to 401 unauthorized. Auth middleware emits this for missing or
	// invalid JWT cookies.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrUnsupportedMediaType: image upload endpoint received a non-multipart
	// Content-Type. Maps to 415 unsupported_media_type.
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	// ErrManifestRequired: image upload missing the required manifest part.
	// Maps to 400 manifest_required.
	ErrManifestRequired = errors.New("manifest required")
	// ErrFileCountMismatch: image upload's file count does not equal the
	// manifest entry count. Maps to 400 file_count_mismatch.
	ErrFileCountMismatch = errors.New("file count mismatch")
	// ErrOpenFile: image upload couldn't open a multipart File. Maps to
	// 400 open_file (preserves the historical Shape-A leak per spec §3).
	ErrOpenFile = errors.New("open file")
	// ErrDecodeFailed: image bytes failed to decode (image/png, image/jpeg).
	// Maps to 422 decode_failed.
	ErrDecodeFailed = errors.New("decode failed")
)

// badCursorError wraps a cursor parse error so it satisfies errors.Is(ErrBadCursor)
// while preserving the original error message for Shape B output.
type badCursorError struct{ cause error }

func (e *badCursorError) Error() string   { return e.cause.Error() }
func (e *badCursorError) Unwrap() error   { return e.cause }
func (e *badCursorError) Is(t error) bool { return t == ErrBadCursor }

// WrapBadCursor wraps a cursor-parse error so WriteError emits the correct
// Shape-B response: {"error":"bad_cursor","message":"<cause message>"}.
func WrapBadCursor(err error) error { return &badCursorError{cause: err} }

// WriteError maps any error to its captured (status, code) pair from
// error_codes_observed.md. Every captured pair and every documented gap has
// a switch arm here.
func WriteError(w http.ResponseWriter, log zerolog.Logger, err error) {
	switch {
	// --- 401 ---
	case errors.Is(err, ErrUnauthorized):
		write(w, http.StatusUnauthorized, HTTPError{Error: "unauthorized"})

	// --- 404 ---
	case errors.Is(err, userdomain.ErrNotFound),
		errors.Is(err, artworkdomain.ErrNotFound):
		write(w, http.StatusNotFound, HTTPError{Error: "not_found"})
	case errors.Is(err, authdomain.ErrUnknownProvider):
		write(w, http.StatusNotFound, HTTPError{Error: "unknown_provider"})

	// --- 400 (parse) ---
	case errors.Is(err, ErrBadJSON):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_json"})
	case errors.Is(err, ErrBadCursor):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_cursor", Message: err.Error()})

	// --- 400 (auth callback state mismatch) ---
	case errors.Is(err, authdomain.ErrBadState):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_state"})

	// --- 400 (image upload structural) ---
	case errors.Is(err, ErrManifestRequired):
		write(w, http.StatusBadRequest, HTTPError{Error: "manifest_required"})
	case errors.Is(err, ErrFileCountMismatch):
		write(w, http.StatusBadRequest, HTTPError{Error: "file_count_mismatch"})
	case errors.Is(err, ErrOpenFile):
		write(w, http.StatusBadRequest, HTTPError{Error: "open_file"})

	// --- 400 (validation — validator.ValidationErrors flows here) ---
	case func() bool {
		var verrs validator.ValidationErrors
		return errors.As(err, &verrs)
	}():
		var verrs validator.ValidationErrors
		_ = errors.As(err, &verrs)
		write(w, http.StatusBadRequest, validationToHTTPError(verrs))

	// --- 412 (image: concurrent same-position write) ---
	case errors.Is(err, imagedomain.ErrPositionTaken):
		write(w, http.StatusPreconditionFailed, HTTPError{Error: "position_taken"})

	// --- 415 (image: non-multipart body) ---
	case errors.Is(err, ErrUnsupportedMediaType):
		write(w, http.StatusUnsupportedMediaType, HTTPError{Error: "unsupported_media_type"})

	// --- 409 (image: fingerprint mismatch, Shape B) ---
	case errors.Is(err, imagedomain.ErrFingerprintMismatch):
		write(w, http.StatusConflict, HTTPError{
			Error:   "fingerprint_mismatch",
			Message: "client_image_id reused with different bytes",
		})

	// --- 422 (image) ---
	case errors.Is(err, ErrDecodeFailed):
		write(w, http.StatusUnprocessableEntity, HTTPError{Error: "decode_failed"})
	case errors.Is(err, imageservice.ErrTooLarge):
		write(w, http.StatusUnprocessableEntity, HTTPError{Error: "too_large"})
	case errors.Is(err, imageservice.ErrContentTypeMismatch):
		write(w, http.StatusUnprocessableEntity, HTTPError{
			Error:   "content_type_mismatch",
			Message: "body does not match declared content_type",
		})

	// --- 502 (auth callback OAuth exchange failure) ---
	case errors.Is(err, authdomain.ErrExchangeFailed):
		write(w, http.StatusBadGateway, HTTPError{Error: "exchange_failed"})

	// --- 500 fallback (operation-name leaks per spec §3) ---
	default:
		log.Error().Err(err).Msg("internal error")
		write(w, http.StatusInternalServerError, HTTPError{Error: errorOpCode(err)})
	}
}

func write(w http.ResponseWriter, status int, body HTTPError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// errorOpCode reads the wrapped-error prefix to choose between the operation-
// specific 500 codes (create_failed, patch_failed, ...). Spec §3 documents
// these as "operation-name leaks" preserved into the contract.
func errorOpCode(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "create:"):
		return "create_failed"
	case strings.Contains(msg, "patch:"):
		return "patch_failed"
	case strings.Contains(msg, "flip:"):
		return "flip_failed"
	case strings.Contains(msg, "tag:"):
		return "tag_failed"
	case strings.Contains(msg, "delete:"):
		return "delete_failed"
	case strings.Contains(msg, "list:"):
		return "list_failed"
	case strings.Contains(msg, "user_lookup:"):
		return "user_lookup_failed"
	case strings.Contains(msg, "user:"):
		return "user_failed"
	case strings.Contains(msg, "upsert:"):
		return "upsert_failed"
	case strings.Contains(msg, "sign:"):
		return "sign_failed"
	case strings.Contains(msg, "upload:"):
		return "upload_failed"
	default:
		return "internal_error"
	}
}
