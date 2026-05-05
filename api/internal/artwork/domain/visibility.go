package domain

type Visibility string

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

func ValidVisibility(s string) bool {
	return s == "public" || s == "private"
}
