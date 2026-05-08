package domain

import "errors"

var (
	// HTTP-observable
	ErrNotFound = errors.New("artwork: not found") // → 404 not_found
	ErrNoImages = errors.New("artwork: no images") // service-internal

	// Internal-only — services translate before returning. Per spec §7.2.1:
	//   ErrForbidden      → translates to ErrNotFound (404, not 403)
	//   ErrAlreadyPublished → translates to nil (204, not 409)
	ErrForbidden        = errors.New("artwork: caller does not own resource")
	ErrAlreadyPublished = errors.New("artwork: already published")

	// Validation errors (set by service or domain constructor)
	ErrBadVisibility    = errors.New("artwork: bad visibility")
	ErrBadCoverPosition = errors.New("artwork: bad cover position")
)
