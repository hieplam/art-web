package domain

import "errors"

var (
	// HTTP-observable: 404 not_found in WriteError (§7.4)
	ErrNotFound = errors.New("user: not found")
)
