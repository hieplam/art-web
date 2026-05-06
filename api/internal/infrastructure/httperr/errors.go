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

// ErrBadJSON is the http-layer sentinel for JSON decode failures.
// Handlers that encounter json.Decode errors pass this to WriteError.
var ErrBadJSON = errors.New("bad json")

// ErrBadCursor is the sentinel for cursor-parse failures. Use WrapBadCursor
// to carry the original parse-error message through WriteError.
var ErrBadCursor = errors.New("bad cursor")

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
	// --- 404 ---
	case errors.Is(err, userdomain.ErrNotFound),
		errors.Is(err, artworkdomain.ErrNotFound):
		write(w, http.StatusNotFound, HTTPError{Error: "not_found"})

	// --- 400 (parse) ---
	case errors.Is(err, ErrBadJSON):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_json"})
	case errors.Is(err, ErrBadCursor):
		write(w, http.StatusBadRequest, HTTPError{Error: "bad_cursor", Message: err.Error()})

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

	// --- 409 (image: fingerprint mismatch, Shape B) ---
	case errors.Is(err, imagedomain.ErrFingerprintMismatch):
		write(w, http.StatusConflict, HTTPError{
			Error:   "fingerprint_mismatch",
			Message: "client_image_id reused with different bytes",
		})

	// --- 422 (image) ---
	case errors.Is(err, imageservice.ErrTooLarge):
		write(w, http.StatusUnprocessableEntity, HTTPError{Error: "too_large"})
	case errors.Is(err, imageservice.ErrContentTypeMismatch):
		write(w, http.StatusUnprocessableEntity, HTTPError{
			Error:   "content_type_mismatch",
			Message: "body does not match declared content_type",
		})

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
