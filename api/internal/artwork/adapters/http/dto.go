// api/internal/artwork/adapters/http/dto.go
package http

// CreateArtworkRequest is the decoded body for POST /artworks.
// Field declaration order is alphabetical by JSON tag so the json_field_order
// CI gate passes.
//
// Visibility uses omitempty so the empty-string default ("" → "private") still
// flows through; the handler sets the default before calling the validator.
type CreateArtworkRequest struct {
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Title       string   `json:"title"`
	Visibility  string   `json:"visibility" validate:"omitempty,oneof=public private"`
}

// PatchArtworkRequest is the decoded body for PATCH /artworks/{id}.
// Field declaration order is alphabetical by JSON tag.
//
// CoverPosition uses omitempty with gte=0 so negative values are rejected.
// Visibility uses omitempty with oneof so invalid values are rejected.
type PatchArtworkRequest struct {
	CoverPosition *int     `json:"cover_position,omitempty" validate:"omitempty,gte=0"`
	Description   *string  `json:"description,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Title         *string  `json:"title,omitempty"`
	Visibility    *string  `json:"visibility,omitempty" validate:"omitempty,oneof=public private"`
}
