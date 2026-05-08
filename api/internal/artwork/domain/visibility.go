package domain

// Visibility is a string-typed alias so values can be compared against and
// concatenated with plain string literals throughout the codebase without
// explicit conversions. Constants below pin the only two legal values.
type Visibility = string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

func ValidVisibility(s string) bool {
	return s == VisibilityPublic || s == VisibilityPrivate
}
