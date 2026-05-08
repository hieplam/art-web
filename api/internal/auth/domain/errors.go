package domain

import "errors"

// HTTP-observable
var (
	ErrUnknownProvider = errors.New("auth: unknown provider")
	ErrBadState        = errors.New("auth: state cookie mismatch")
	ErrExchangeFailed  = errors.New("auth: oauth exchange failed")
)
