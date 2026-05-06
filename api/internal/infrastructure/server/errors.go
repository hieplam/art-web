// api/internal/infrastructure/server/errors.go
//
// This file re-exports the WriteError boundary from infrastructure/httperr.
// The actual implementation lives there to avoid the import cycle that would
// arise if this package (which imports slice routers via registrar.go) also
// directly imported the domain packages that WriteError needs.
package server

import (
	"local/art-web/api/internal/infrastructure/httperr"
)

// HTTPError, ErrBadJSON, ErrBadCursor, WrapBadCursor, and WriteError are
// the public API consumed by slice HTTP adapters. They are all provided by
// the httperr sub-package to break the import cycle.
//
// Slice adapters should import httperr directly:
//
//	import "local/art-web/api/internal/infrastructure/httperr"
//
// and call httperr.WriteError / httperr.ErrBadJSON etc.

// NewValidator creates a new validator instance for use by slice handlers.
// Kept here (not in httperr) because validator has no domain imports.
//
// Re-exported type alias so callers can use server.HTTPError if needed.
type HTTPError = httperr.HTTPError
