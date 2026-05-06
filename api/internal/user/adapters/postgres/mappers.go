// api/internal/user/adapters/postgres/mappers.go
package postgres

import userdomain "local/art-web/api/internal/user/domain"

// toDomainUser converts a GORM row model into the domain User. Pure function;
// the repo composes this with database.DB(ctx, r.db).First(&m, ...) so the
// adapter boundary does the persistence-shape↔domain translation per spec §6.5.
func toDomainUser(m *userModel) *userdomain.User {
	avatar := ""
	if m.AvatarURL != nil {
		avatar = *m.AvatarURL
	}
	return &userdomain.User{
		ID:          m.ID,
		Slug:        m.Slug,
		DisplayName: m.DisplayName,
		Email:       m.Email,
		AvatarURL:   avatar,
	}
}
