package image

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
)

type Handler struct {
	svc *Service
	art *artwork.Repo
	url *auth.URLBuilder
}

func NewHandler(s *Service, ar *artwork.Repo, u *auth.URLBuilder) *Handler {
	return &Handler{svc: s, art: ar, url: u}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	uid, _ := auth.UserIDFrom(r.Context())
	artID := chi.URLParam(r, "id")

	art, err := h.art.Get(r.Context(), artID)
	if err != nil || art.UserID != uid {
		http.Error(w, `{"error":"not_found"}`, 404)
		return
	}

	if err := r.ParseMultipartForm(64 << 20); err != nil {
		http.Error(w, `{"error":"bad_multipart"}`, 400)
		return
	}
	manifestRaw := r.FormValue("manifest")
	if manifestRaw == "" {
		http.Error(w, `{"error":"manifest_required"}`, 400)
		return
	}
	entries, err := ParseManifest(strings.NewReader(manifestRaw))
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) != len(entries) {
		http.Error(w, `{"error":"file_count_mismatch"}`, 400)
		return
	}

	results := make([]any, 0, len(entries))
	for i, e := range entries {
		f, err := files[i].Open()
		if err != nil {
			http.Error(w, `{"error":"open_file"}`, 400)
			return
		}
		out, err := h.svc.UploadOne(r.Context(), art, UploadOne{Manifest: e, Body: f})
		f.Close()
		switch {
		case errors.Is(err, ErrFingerprintMismatch):
			http.Error(w, `{"error":"fingerprint_mismatch","message":"client_image_id reused with different bytes"}`, 409)
			return
		case errors.Is(err, ErrPositionTaken):
			http.Error(w, `{"error":"position_taken"}`, 412)
			return
		case errors.Is(err, ErrTooLarge):
			http.Error(w, `{"error":"too_large"}`, 422)
			return
		case err != nil:
			http.Error(w, `{"error":"upload_failed"}`, 500)
			return
		}
		results = append(results, map[string]any{
			"id":              out.Image.ID,
			"client_image_id": out.Image.ClientImageID,
			"position":        out.Image.Position,
			"width":           out.Image.Width,
			"height":          out.Image.Height,
			"blurhash":        out.Image.Blurhash,
			"existed":         out.Existed,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
