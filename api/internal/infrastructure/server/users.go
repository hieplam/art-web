package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	authhttp "local/art-web/api/internal/auth/adapters/http"
)

func userProfileHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		viewer, _ := authhttp.UserIDFrom(r.Context())
		profileUser, err := d.Users.GetBySlug(r.Context(), slug)
		if err != nil {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		isOwner := viewer == profileUser.ID
		cursor, err := parseCursor(r)
		if err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_cursor", "message": err.Error()})
			return
		}
		page, err := d.Artworks.ListByUser(r.Context(), profileUser.ID, isOwner, cursor, parseLimit(r))
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		items := make([]any, 0, len(page.Items))
		for i := range page.Items {
			a := &page.Items[i]
			cover, artist, err := loadCoverAndArtist(r.Context(), d, a)
			if err != nil {
				continue
			}
			items = append(items, renderArtworkSummary(d, a, cover, artist))
		}
		renderJSON(w, 200, map[string]any{
			"user":        renderUser(profileUser),
			"artworks":    items,
			"next_cursor": encodeCursor(page.NextCursor),
		})
	})
}
