// api/internal/artwork/adapters/postgres/mappers.go
package postgres

import (
	artworkdomain "local/art-web/api/internal/artwork/domain"
)

// toDomainArtwork converts a GORM row model into the domain Artwork. The
// CreatedAt field is normalized into a *time.Time alias so handlers that read
// either time.Time or *time.Time keep matching the legacy persistence shape.
// Pure function with no DB access; tests can drive it directly.
func toDomainArtwork(m *artworkModel) *artworkdomain.Artwork {
	if m == nil {
		// programmer error per spec §7.8: caller asked for an artwork mapping
		// but handed in a nil row.
		panic("toDomainArtwork: nil model")
	}
	cre := m.CreatedAt
	return &artworkdomain.Artwork{
		ID:            m.ID,
		UserID:        m.UserID,
		Title:         m.Title,
		Description:   m.Description,
		Visibility:    m.Visibility,
		CoverPosition: m.CoverPosition,
		CreatedAt:     &cre,
		PublishedAt:   m.PublishedAt,
	}
}
