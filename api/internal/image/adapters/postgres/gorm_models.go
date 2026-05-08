// api/internal/image/adapters/postgres/gorm_models.go
package postgres

import "time"

// imageModel mirrors the artwork_images table EXACTLY (see
// migrations/0001_init.up.sql). AutoMigrate is forbidden per spec §10.2;
// column tags exist solely so GORM emits matching SQL against the
// migration-driven schema.
type imageModel struct {
	ID            string    `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	ArtworkID     string    `gorm:"column:artwork_id;type:uuid;not null"`
	ClientImageID string    `gorm:"column:client_image_id;type:text;not null"`
	StorageKey    string    `gorm:"column:storage_key;type:text;not null"`
	SourceSHA256  string    `gorm:"column:source_sha256;type:text;not null"`
	Width         int       `gorm:"column:width;type:int;not null"`
	Height        int       `gorm:"column:height;type:int;not null"`
	ByteSize      int       `gorm:"column:byte_size;type:int;not null"`
	ContentType   string    `gorm:"column:content_type;type:text;not null"`
	Position      int       `gorm:"column:position;type:int;not null"`
	Blurhash      *string   `gorm:"column:blurhash;type:text"`
	CreatedAt     time.Time `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
}

func (imageModel) TableName() string { return "artwork_images" }
