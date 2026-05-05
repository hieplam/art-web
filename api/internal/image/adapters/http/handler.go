package http

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	imagedomain "local/art-web/api/internal/image/domain"
	imageservice "local/art-web/api/internal/image/service"
	"local/art-web/api/pkg/signing"
)

type Handler struct {
	svc *imageservice.Service
	art *artworkpostgres.Repo
	url *signing.URLBuilder
}

func NewHandler(s *imageservice.Service, ar *artworkpostgres.Repo, u *signing.URLBuilder) *Handler {
	return &Handler{svc: s, art: ar, url: u}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	uid, _ := authhttp.UserIDFrom(r.Context())
	artID := chi.URLParam(r, "id")

	art, err := h.art.Get(r.Context(), artID)
	if err != nil || art.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}

	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "multipart/form-data" {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "unsupported_media_type"})
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad_multipart"})
		return
	}
	manifestRaw := r.FormValue("manifest")
	if manifestRaw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "manifest_required"})
		return
	}
	entries, err := imagedomain.ParseManifest(strings.NewReader(manifestRaw))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	files := r.MultipartForm.File["files"]
	if len(files) != len(entries) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file_count_mismatch"})
		return
	}

	results := make([]any, 0, len(entries))
	status := http.StatusOK
	for i, e := range entries {
		f, err := files[i].Open()
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "open_file"})
			return
		}
		out, err := h.svc.UploadOne(r.Context(), art, imageservice.UploadOne{Manifest: e, Body: f})
		f.Close()
		switch {
		case errors.Is(err, imagepostgres.ErrFingerprintMismatch):
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":   "fingerprint_mismatch",
				"message": "client_image_id reused with different bytes",
			})
			return
		case errors.Is(err, imagepostgres.ErrPositionTaken):
			writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "position_taken"})
			return
		case errors.Is(err, imageservice.ErrTooLarge):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "too_large"})
			return
		case errors.Is(err, imageservice.ErrContentTypeMismatch):
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
				"error":   "content_type_mismatch",
				"message": "body does not match declared content_type",
			})
			return
		case err != nil:
			if strings.HasPrefix(err.Error(), "decode:") {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "decode_failed"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "upload_failed"})
			return
		}
		if !out.Existed {
			status = http.StatusCreated
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
	writeJSON(w, status, results)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
