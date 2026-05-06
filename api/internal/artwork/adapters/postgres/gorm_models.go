// api/internal/artwork/adapters/postgres/gorm_models.go
package postgres

import "time"

// artworkModel mirrors the artworks table EXACTLY (see migrations/0001_init.up.sql).
// AutoMigrate is forbidden per spec §10.2; column tags exist solely so GORM
// emits matching SQL against the migration-driven schema.
type artworkModel struct {
	ID            string     `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID        string     `gorm:"column:user_id;type:uuid;not null"`
	Title         string     `gorm:"column:title;type:text;not null"`
	Description   *string    `gorm:"column:description;type:text"`
	Visibility    string     `gorm:"column:visibility;type:text;not null"`
	CoverPosition int        `gorm:"column:cover_position;type:int;not null;default:0"`
	CreatedAt     time.Time  `gorm:"column:created_at;type:timestamptz;not null;default:now()"`
	PublishedAt   *time.Time `gorm:"column:published_at;type:timestamptz"`
}

func (artworkModel) TableName() string { return "artworks" }

// artworkImageModel mirrors the artwork_images table.
type artworkImageModel struct {
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

func (artworkImageModel) TableName() string { return "artwork_images" }

// tagModel mirrors the tags table.
type tagModel struct {
	ID   string `gorm:"column:id;type:uuid;primaryKey;default:gen_random_uuid()"`
	Name string `gorm:"column:name;type:text;uniqueIndex;not null"`
}

func (tagModel) TableName() string { return "tags" }

// artworkTagModel mirrors the artwork_tags join table. Composite PK is
// (artwork_id, tag_id).
type artworkTagModel struct {
	ArtworkID string `gorm:"column:artwork_id;type:uuid;primaryKey;not null"`
	TagID     string `gorm:"column:tag_id;type:uuid;primaryKey;not null"`
}

func (artworkTagModel) TableName() string { return "artwork_tags" }
