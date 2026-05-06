// api/internal/image/adapters/postgres/mappers.go
package postgres

import imagedomain "local/art-web/api/internal/image/domain"

// toDomainImage converts an artwork_images row model into the domain Image.
// Pure function; the repo composes it after First/Find calls.
func toDomainImage(m *imageModel) *imagedomain.Image {
	blur := ""
	if m.Blurhash != nil {
		blur = *m.Blurhash
	}
	return &imagedomain.Image{
		ID:            m.ID,
		ArtworkID:     m.ArtworkID,
		ClientImageID: m.ClientImageID,
		StorageKey:    m.StorageKey,
		SourceSHA256:  m.SourceSHA256,
		ContentType:   m.ContentType,
		Position:      m.Position,
		Width:         m.Width,
		Height:        m.Height,
		ByteSize:      m.ByteSize,
		Blurhash:      blur,
	}
}
