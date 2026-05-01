package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/user"
)

func renderJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func renderUser(u *user.User) map[string]any {
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

func renderImageRef(d *Deps, im *image.InsertedImage, visibility string) map[string]any {
	var url string
	if visibility == "public" {
		url = d.URL.Public(im.StorageKey)
	} else {
		url = d.URL.Private(im.StorageKey, 5*time.Minute)
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

func renderArtworkSummary(d *Deps, a *artwork.Artwork, cover *image.InsertedImage, artist *user.User) map[string]any {
	var pub *string
	if a.PublishedAt != nil {
		s := a.PublishedAt.UTC().Format(time.RFC3339)
		pub = &s
	}
	var coverJSON any
	if cover != nil {
		coverJSON = renderImageRef(d, cover, a.Visibility)
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
