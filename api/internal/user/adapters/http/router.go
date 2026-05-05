// Package http exposes the user slice's HTTP routes (currently just
// GET /users/{slug}) via the RouteRegistrar contract.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	"local/art-web/api/internal/infrastructure/httputil"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

// Handler renders the user-profile response, which composes data from the
// user, artwork, and image slices. The cross-slice dependency is intentional:
// the user slice owns the /users/{slug} endpoint so this is the natural home,
// but it depends on artwork+image data via concrete repos. Future work could
// hide those behind interfaces in user/ports if the dependency becomes
// onerous.
type Handler struct {
	users    *userpostgres.Repo
	artworks *artworkpostgres.Repo
	images   *imagepostgres.Repo
	url      *signing.URLBuilder
}

// NewHandler wires a user-profile handler. All four collaborators are
// required.
func NewHandler(
	users *userpostgres.Repo,
	artworks *artworkpostgres.Repo,
	images *imagepostgres.Repo,
	url *signing.URLBuilder,
) *Handler {
	return &Handler{users: users, artworks: artworks, images: images, url: url}
}

// Profile handles GET /users/{slug}. Renders the slug owner's user object
// plus the page of artworks they may see (owner sees private; others don't).
func (h *Handler) Profile(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	viewer, _ := authhttp.UserIDFrom(r.Context())
	profileUser, err := h.users.GetBySlug(r.Context(), slug)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	isOwner := viewer == profileUser.ID
	cursor, err := httputil.ParseCursor(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_cursor", "message": err.Error()})
		return
	}
	page, err := h.artworks.ListByUser(r.Context(), profileUser.ID, isOwner, cursor, httputil.ParseLimit(r))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "list_failed"})
		return
	}
	items := make([]any, 0, len(page.Items))
	for i := range page.Items {
		a := &page.Items[i]
		cover, artist, err := h.loadCoverAndArtist(r.Context(), a)
		if err != nil {
			continue
		}
		items = append(items, h.renderArtworkSummary(a, cover, artist))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":        renderUser(profileUser),
		"artworks":    items,
		"next_cursor": httputil.EncodeCursor(page.NextCursor),
	})
}

// loadCoverAndArtist looks up the cover image (preferring CoverPosition; falls
// back to the first image) and the artwork's artist user. Returns an error if
// the artwork has no images at all so the caller can skip rendering it.
func (h *Handler) loadCoverAndArtist(ctx context.Context, a *artworkpostgres.Artwork) (*imagepostgres.InsertedImage, *userpostgres.User, error) {
	images, err := h.images.ListByArtwork(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}
	var cover *imagepostgres.InsertedImage
	for i := range images {
		if images[i].Position == a.CoverPosition {
			cover = &images[i]
			break
		}
	}
	if cover == nil && len(images) > 0 {
		cover = &images[0]
	}
	if cover == nil {
		return nil, nil, errors.New("no images")
	}
	u, err := h.users.Get(ctx, a.UserID)
	return cover, u, err
}

func (h *Handler) renderArtworkSummary(a *artworkpostgres.Artwork, cover *imagepostgres.InsertedImage, artist *userpostgres.User) map[string]any {
	var pub *string
	if a.PublishedAt != nil {
		s := a.PublishedAt.UTC().Format(time.RFC3339)
		pub = &s
	}
	var coverJSON any
	if cover != nil {
		coverJSON = h.renderImageRef(cover, a.Visibility)
	}
	return map[string]any{
		"id":           a.ID,
		"title":        a.Title,
		"visibility":   a.Visibility,
		"published_at": pub,
		"created_at":   a.CreatedAt.UTC().Format(time.RFC3339),
		"cover":        coverJSON,
		"artist":       renderUser(artist),
	}
}

func (h *Handler) renderImageRef(im *imagepostgres.InsertedImage, visibility string) map[string]any {
	var url string
	if visibility == "public" {
		url = h.url.Public(im.StorageKey)
	} else {
		url = h.url.Private(im.StorageKey, 5*time.Minute)
	}
	return map[string]any{
		"id":       im.ID,
		"url":      url,
		"width":    im.Width,
		"height":   im.Height,
		"blurhash": im.Blurhash,
		"position": im.Position,
	}
}

// Router exposes the user slice's routes via RouteRegistrar.
type Router struct {
	handler *Handler
}

// NewRouter pairs a Handler with the RouteRegistrar contract.
func NewRouter(h *Handler) *Router { return &Router{handler: h} }

// RegisterRoutes mounts /users/{slug} on the supplied chi.Router.
func (router *Router) RegisterRoutes(r chi.Router) {
	r.Get("/users/{slug}", router.handler.Profile)
}

func renderUser(u *userpostgres.User) map[string]any {
	var avatar *string
	if u.AvatarURL != "" {
		v := u.AvatarURL
		avatar = &v
	}
	return map[string]any{
		"id":           u.ID,
		"display_name": u.DisplayName,
		"slug":         u.Slug,
		"avatar_url":   avatar,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
