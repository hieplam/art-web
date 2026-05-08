package domain

// User is the canonical user entity. Mirrors current user.User.
type User struct {
	ID          string
	Slug        string
	DisplayName string
	Email       string
	AvatarURL   string
}
