package domain

// Identity is the OAuth-provider-supplied profile after Exchange.
// Mirrors auth.Profile from current code; renaming for domain semantics.
type Identity struct {
	Subject     string // provider's stable user ID
	Email       string
	DisplayName string
	AvatarURL   string
}

// Session is what the auth service returns after successful sign-in.
type Session struct {
	UserID string
	Token  string // signed JWT
}
