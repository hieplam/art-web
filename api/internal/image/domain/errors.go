package domain

import "errors"

var (
	// HTTP-observable
	ErrTooLarge            = errors.New("image: file exceeds limit")     // 422 too_large
	ErrFingerprintMismatch = errors.New("image: client_image_id reused") // 409 fingerprint_mismatch (Shape B)
	ErrPositionTaken       = errors.New("image: position taken")         // 412 position_taken
	ErrContentTypeMismatch = errors.New("image: content type mismatch")  // 422 content_type_mismatch (Shape B)
)
