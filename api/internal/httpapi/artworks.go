package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/user"
)

func parseLimit(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if n <= 0 || n > 100 {
		return 24
	}
	return n
}

func parseCursor(r *http.Request) (artwork.FeedCursor, error) {
	raw := r.URL.Query().Get("cursor")
	if raw == "" {
		return artwork.FeedCursor{}, nil
	}
	dec, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return artwork.FeedCursor{}, fmt.Errorf("bad_cursor: %w", err)
	}
	parts := strings.SplitN(string(dec), "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return artwork.FeedCursor{}, errors.New("bad_cursor: missing field")
	}
	stamp, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return artwork.FeedCursor{}, fmt.Errorf("bad_cursor: timestamp %w", err)
	}
	return artwork.FeedCursor{Stamp: stamp.UTC(), ID: parts[1]}, nil
}

func encodeCursor(c *artwork.FeedCursor) *string {
	if c == nil {
		return nil
	}
	raw := c.Stamp.UTC().Format(time.RFC3339Nano) + "|" + c.ID
	enc := base64.RawURLEncoding.EncodeToString([]byte(raw))
	return &enc
}

func meHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := auth.UserIDFrom(r.Context())
		if !ok {
			renderJSON(w, 401, map[string]string{"error": "unauthorized"})
			return
		}
		u, err := d.Users.Get(r.Context(), uid)
		if err != nil {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		renderJSON(w, 200, renderUser(u))
	})
}

func listFeedHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor, err := parseCursor(r)
		if err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_cursor", "message": err.Error()})
			return
		}
		page, err := d.Artworks.PublicFeed(r.Context(), cursor, parseLimit(r))
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		renderFeed(w, r.Context(), d, page)
	})
}

func renderFeed(w http.ResponseWriter, ctx context.Context, d *Deps, page *artwork.FeedPage) {
	items := make([]any, 0, len(page.Items))
	for i := range page.Items {
		a := &page.Items[i]
		cover, artist, err := loadCoverAndArtist(ctx, d, a)
		if err != nil {
			continue
		}
		items = append(items, renderArtworkSummary(d, a, cover, artist))
	}
	renderJSON(w, 200, map[string]any{
		"items":       items,
		"next_cursor": encodeCursor(page.NextCursor),
	})
}

func loadCoverAndArtist(ctx context.Context, d *Deps, a *artwork.Artwork) (*image.InsertedImage, *user.User, error) {
	images, err := d.Images.ListByArtwork(ctx, a.ID)
	if err != nil {
		return nil, nil, err
	}
	var cover *image.InsertedImage
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
	u, err := d.Users.Get(ctx, a.UserID)
	return cover, u, err
}

func createArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		var body struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Visibility  string   `json:"visibility"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_json"})
			return
		}
		if body.Visibility == "" {
			body.Visibility = "private"
		}
		if body.Visibility != "public" && body.Visibility != "private" {
			renderJSON(w, 400, map[string]string{"error": "bad_visibility"})
			return
		}
		id, err := d.Artworks.Create(r.Context(), uid, body.Title, body.Description, body.Visibility)
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "create_failed"})
			return
		}
		if len(body.Tags) > 0 {
			_ = d.Tags.SetTags(r.Context(), id, body.Tags)
		}
		renderJSON(w, 201, map[string]string{"id": id})
	})
}

func patchArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		id := chi.URLParam(r, "id")
		a, err := d.Artworks.Get(r.Context(), id)
		if err != nil || a.UserID != uid {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		var body struct {
			Title         *string  `json:"title,omitempty"`
			Description   *string  `json:"description,omitempty"`
			Visibility    *string  `json:"visibility,omitempty"`
			CoverPosition *int     `json:"cover_position,omitempty"`
			Tags          []string `json:"tags,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			renderJSON(w, 400, map[string]string{"error": "bad_json"})
			return
		}
		if body.Title != nil {
			if err := d.Artworks.PatchTitle(r.Context(), id, *body.Title); err != nil {
				renderJSON(w, 500, map[string]string{"error": "patch_failed"})
				return
			}
		}
		if body.Description != nil {
			if err := d.Artworks.PatchDescription(r.Context(), id, *body.Description); err != nil {
				renderJSON(w, 500, map[string]string{"error": "patch_failed"})
				return
			}
		}
		if body.CoverPosition != nil {
			if *body.CoverPosition < 0 {
				renderJSON(w, 400, map[string]string{"error": "bad_cover_position"})
				return
			}
			if err := d.Artworks.SetCoverPosition(r.Context(), id, *body.CoverPosition); err != nil {
				renderJSON(w, 500, map[string]string{"error": "patch_failed"})
				return
			}
		}
		if body.Visibility != nil {
			if *body.Visibility != "public" && *body.Visibility != "private" {
				renderJSON(w, 400, map[string]string{"error": "bad_visibility"})
				return
			}
			if err := d.Vis.Flip(r.Context(), id, *body.Visibility); err != nil {
				renderJSON(w, 500, map[string]string{"error": "flip_failed"})
				return
			}
		}
		if body.Tags != nil {
			if err := d.Tags.SetTags(r.Context(), id, body.Tags); err != nil {
				renderJSON(w, 500, map[string]string{"error": "tag_failed"})
				return
			}
		}
		w.WriteHeader(204)
	})
}

func deleteArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, _ := auth.UserIDFrom(r.Context())
		id := chi.URLParam(r, "id")
		a, err := d.Artworks.Get(r.Context(), id)
		if err != nil || a.UserID != uid {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		// TODO post-v1: also purge R2 objects (currently leaks).
		if err := d.Artworks.Delete(r.Context(), id); err != nil {
			renderJSON(w, 500, map[string]string{"error": "delete_failed"})
			return
		}
		w.WriteHeader(204)
	})
}

func getArtworkHandler(d *Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		uid, _ := auth.UserIDFrom(r.Context())
		a, err := d.Artworks.Get(r.Context(), id)
		if err != nil || (a.Visibility == "private" && a.UserID != uid) {
			renderJSON(w, 404, map[string]string{"error": "not_found"})
			return
		}
		images, err := d.Images.ListByArtwork(r.Context(), a.ID)
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "list_failed"})
			return
		}
		artist, err := d.Users.Get(r.Context(), a.UserID)
		if err != nil {
			renderJSON(w, 500, map[string]string{"error": "user_failed"})
			return
		}
		tags, _ := d.Tags.GetTags(r.Context(), a.ID)
		var description *string
		if a.Description != nil && *a.Description != "" {
			description = a.Description
		}
		imgs := make([]any, 0, len(images))
		for i := range images {
			imgs = append(imgs, renderImageRef(d, &images[i], a.Visibility))
		}
		var cover *image.InsertedImage
		for i := range images {
			if images[i].Position == a.CoverPosition {
				cover = &images[i]
				break
			}
		}
		if cover == nil && len(images) > 0 {
			cover = &images[0]
		}
		summary := renderArtworkSummary(d, a, cover, artist)
		summary["description"] = description
		summary["tags"] = tags
		summary["images"] = imgs
		renderJSON(w, 200, summary)
	})
}
