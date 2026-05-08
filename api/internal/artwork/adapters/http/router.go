// Package http exposes the artwork slice's HTTP routes — the /artworks/*
// CRUD set plus /tags/{name} — wired against the slice's services and ports.
package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/rs/zerolog"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	artworkservice "local/art-web/api/internal/artwork/service"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	"local/art-web/api/internal/infrastructure/httperr"
	"local/art-web/api/internal/infrastructure/httputil"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

// Handler holds every collaborator the artwork-CRUD set needs. The image
// upload route (POST /artworks/{id}/images) lives in the image slice's Router
// — this slice owns only the artwork-data routes plus the tag feed.
type Handler struct {
	artworks  *artworkpostgres.Repo
	tags      *artworkpostgres.TagsRepo
	users     *userpostgres.Repo
	images    *imagepostgres.Repo
	vis       *artworkservice.VisibilityService
	url       *signing.URLBuilder
	log       zerolog.Logger
	validator *validator.Validate
}

// NewHandler wires the artwork-CRUD HTTP handler.
func NewHandler(
	artworks *artworkpostgres.Repo,
	tags *artworkpostgres.TagsRepo,
	users *userpostgres.Repo,
	images *imagepostgres.Repo,
	vis *artworkservice.VisibilityService,
	url *signing.URLBuilder,
	log zerolog.Logger,
	v *validator.Validate,
) *Handler {
	return &Handler{
		artworks:  artworks,
		tags:      tags,
		users:     users,
		images:    images,
		vis:       vis,
		url:       url,
		log:       log,
		validator: v,
	}
}

// ListFeed handles GET /artworks — the public feed.
func (h *Handler) ListFeed(w http.ResponseWriter, r *http.Request) {
	cursor, err := httputil.ParseCursor(r)
	if err != nil {
		httperr.WriteError(w, h.log, httperr.WrapBadCursor(err))
		return
	}
	page, err := h.artworks.PublicFeed(r.Context(), cursor, httputil.ParseLimit(r))
	if err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	h.renderFeed(w, r.Context(), page)
}

// Tag handles GET /tags/{name} — feed of public artworks for a given tag.
func (h *Handler) Tag(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(chi.URLParam(r, "name"))
	cursor, err := httputil.ParseCursor(r)
	if err != nil {
		httperr.WriteError(w, h.log, httperr.WrapBadCursor(err))
		return
	}
	page, err := h.artworks.ListByTag(r.Context(), name, cursor, httputil.ParseLimit(r))
	if err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	h.renderFeed(w, r.Context(), page)
}

// Get handles GET /artworks/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	uid, _ := authhttp.UserIDFrom(r.Context())
	a, err := h.artworks.Get(r.Context(), id)
	if err != nil || (a.Visibility == "private" && a.UserID != uid) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	images, err := h.images.ListByArtwork(r.Context(), a.ID)
	if err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	artist, err := h.users.Get(r.Context(), a.UserID)
	if err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	tags, _ := h.tags.GetTags(r.Context(), a.ID)
	var description *string
	if a.Description != nil && *a.Description != "" {
		description = a.Description
	}
	imgs := make([]any, 0, len(images))
	for i := range images {
		imgs = append(imgs, h.renderImageRef(&images[i], a.Visibility))
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
	summary := h.renderArtworkSummary(a, cover, artist)
	summary["description"] = description
	summary["tags"] = tags
	summary["images"] = imgs
	writeJSON(w, http.StatusOK, summary)
}

// Create handles POST /artworks.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	uid, _ := authhttp.UserIDFrom(r.Context())
	var req CreateArtworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.WriteError(w, h.log, httperr.ErrBadJSON)
		return
	}
	if req.Visibility == "" {
		req.Visibility = "private"
	}
	if err := h.validator.Struct(&req); err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	id, err := h.artworks.Create(r.Context(), uid, req.Title, req.Description, req.Visibility)
	if err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	if len(req.Tags) > 0 {
		_ = h.tags.SetTags(r.Context(), id, req.Tags)
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// Patch handles PATCH /artworks/{id}.
func (h *Handler) Patch(w http.ResponseWriter, r *http.Request) {
	uid, _ := authhttp.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	a, err := h.artworks.Get(r.Context(), id)
	if err != nil || a.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	var req PatchArtworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperr.WriteError(w, h.log, httperr.ErrBadJSON)
		return
	}
	if err := h.validator.Struct(&req); err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	if req.Title != nil {
		if err := h.artworks.PatchTitle(r.Context(), id, *req.Title); err != nil {
			httperr.WriteError(w, h.log, err)
			return
		}
	}
	if req.Description != nil {
		if err := h.artworks.PatchDescription(r.Context(), id, *req.Description); err != nil {
			httperr.WriteError(w, h.log, err)
			return
		}
	}
	if req.CoverPosition != nil {
		if err := h.artworks.SetCoverPosition(r.Context(), id, *req.CoverPosition); err != nil {
			httperr.WriteError(w, h.log, err)
			return
		}
	}
	if req.Visibility != nil {
		if err := h.vis.Flip(r.Context(), id, *req.Visibility); err != nil {
			httperr.WriteError(w, h.log, err)
			return
		}
	}
	if req.Tags != nil {
		if err := h.tags.SetTags(r.Context(), id, req.Tags); err != nil {
			httperr.WriteError(w, h.log, err)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete handles DELETE /artworks/{id}.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	uid, _ := authhttp.UserIDFrom(r.Context())
	id := chi.URLParam(r, "id")
	a, err := h.artworks.Get(r.Context(), id)
	if err != nil || a.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	// TODO post-v1: also purge R2 objects (currently leaks).
	if err := h.artworks.Delete(r.Context(), id); err != nil {
		httperr.WriteError(w, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// renderFeed builds the JSON payload returned by ListFeed and Tag — both share
// the same items+next_cursor envelope.
func (h *Handler) renderFeed(w http.ResponseWriter, ctx context.Context, page *artworkpostgres.FeedPage) {
	items := make([]any, 0, len(page.Items))
	for i := range page.Items {
		a := &page.Items[i]
		cover, artist, err := h.loadCoverAndArtist(ctx, a)
		if err != nil {
			continue
		}
		items = append(items, h.renderArtworkSummary(a, cover, artist))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": httputil.EncodeCursor(page.NextCursor),
	})
}

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

// Router exposes the artwork slice's routes via RouteRegistrar.
type Router struct {
	handler *Handler
	auth    *authhttp.Middleware
}

// NewRouter pairs the artwork Handler with the auth middleware (for
// RequireUser-gated routes).
func NewRouter(h *Handler, auth *authhttp.Middleware) *Router {
	return &Router{handler: h, auth: auth}
}

// RegisterRoutes mounts /artworks/* (sans /artworks/{id}/images, which the
// image slice owns) and /tags/{name} on the supplied router.
func (router *Router) RegisterRoutes(r chi.Router) {
	r.Get("/artworks", router.handler.ListFeed)
	r.Get("/artworks/{id}", router.handler.Get)
	r.With(router.auth.RequireUser()).Post("/artworks", router.handler.Create)
	r.With(router.auth.RequireUser()).Patch("/artworks/{id}", router.handler.Patch)
	r.With(router.auth.RequireUser()).Delete("/artworks/{id}", router.handler.Delete)
	r.Get("/tags/{name}", router.handler.Tag)
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
