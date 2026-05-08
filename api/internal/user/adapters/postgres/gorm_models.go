// api/internal/user/adapters/postgres/gorm_models.go
package postgres

import "time"

// userModel is the GORM-tagged shape. Column tags must match the existing
// schema EXACTLY — see migrations/0001_init.up.sql. AutoMigrate is forbidden
// per spec §10.2; tags exist solely so GORM emits matching SQL.
type userModel struct {
	ID            string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	OAuthProvider string    `gorm:"column:oauth_provider;type:text;not null"`
	OAuthSubject  string    `gorm:"column:oauth_subject;type:text;not null"`
	Email         string    `gorm:"column:email;type:text;not null"`
	DisplayName   string    `gorm:"column:display_name;type:text;not null"`
	Slug          string    `gorm:"column:slug;type:text;uniqueIndex;not null"`
	AvatarURL     *string   `gorm:"column:avatar_url;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (userModel) TableName() string { return "users" }
