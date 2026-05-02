package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

func tagsHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(chi.URLParam(r, "name"))
		cursor, err := parseCursor(r)
		if err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_cursor", "message": err.Error()})
			return
		}
		page, err := d.Artworks.ListByTag(r.Context(), name, cursor, parseLimit(r))
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		renderFeed(w, r.Context(), d, page)
	})
}
