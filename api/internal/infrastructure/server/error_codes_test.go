package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// errorBody is the universal Shape-A/B decoder.
type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

func decodeError(t *testing.T, body io.Reader) errorBody {
	t.Helper()
	var b errorBody
	if err := json.NewDecoder(body).Decode(&b); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return b
}

// ---- /artworks ----

func TestErrors_ArtworksList_BadCursor(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/artworks?cursor=not-base64!!")
	if code != 400 {
		t.Fatalf("status=%d want 400; body=%s", code, body)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_cursor" {
		t.Fatalf("error=%q want bad_cursor", got.Error)
	}
	if got.Message == "" {
		t.Fatal("Shape B requires non-empty message")
	}
}

func TestErrors_ArtworkPatch_BadJSON(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, "not valid json")
	if code != 400 {
		t.Fatalf("status=%d want 400; body=%s", code, body)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_json" {
		t.Fatalf("error=%q want bad_json", got.Error)
	}
}

func TestErrors_ArtworkPatch_BadVisibility(t *testing.T) {
	env := setupMatrixEnv(t)
	_, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, `{"visibility":"draft"}`)
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
}

func TestErrors_ArtworkPatch_BadCoverPosition(t *testing.T) {
	env := setupMatrixEnv(t)
	_, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID, `{"cover_position":-1}`)
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
}

func TestErrors_ArtworkCreate_BadJSON(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.requestWithJSONBody(t, "owner", "POST", "/artworks", "not json")
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_json" {
		t.Fatalf("error=%q want bad_json", got.Error)
	}
}

func TestErrors_ArtworkCreate_BadVisibility(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.requestWithJSONBody(t, "owner", "POST", "/artworks", `{"title":"x","visibility":"draft"}`)
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_visibility" {
		t.Fatalf("error=%q want bad_visibility", got.Error)
	}
}

// ---- /me ----

func TestErrors_Me_Unauthorized(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/me")
	if code != 401 {
		t.Fatalf("status=%d want 401", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "unauthorized" {
		t.Fatalf("error=%q want unauthorized", got.Error)
	}
}

// ---- /users/{slug} ----

func TestErrors_UserProfile_BadCursor(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/users/alice-"+env.AliceSuffix+"?cursor=not-base64!!")
	if code != 400 {
		t.Fatalf("status=%d want 400", code)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_cursor" || got.Message == "" {
		t.Fatalf("expected Shape B bad_cursor, got %+v", got)
	}
}

// ---- /auth ----

func TestErrors_Auth_UnknownProvider_Start(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/auth/notreal/start")
	if code != 404 {
		t.Fatalf("status=%d want 404", code)
	}
	if !strings.Contains(body, "unknown_provider") {
		t.Fatalf("expected unknown_provider, got %s", body)
	}
}

func TestErrors_AuthCallback_UnknownProvider(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/auth/notreal/callback?state=x&code=y")
	if code != 404 {
		t.Fatalf("status=%d want 404", code)
	}
	if !strings.Contains(body, "unknown_provider") {
		t.Fatalf("expected unknown_provider, got %s", body)
	}
}

// ---- POST /artworks/{id}/images — non-image-package error paths ----

// TestErrors_ImageUpload_415_NonMultipart hits the unsupported_media_type
// branch in image/handler.go via the httpapi router. The image package's
// handler tests cover this directly; this test confirms the path is reachable
// from the httpapi composition.
func TestErrors_ImageUpload_415_NonMultipart(t *testing.T) {
	env := setupMatrixEnv(t)
	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != 415 {
		t.Fatalf("status=%d want 415", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "unsupported_media_type" {
		t.Fatalf("error=%q want unsupported_media_type", got.Error)
	}
}

func TestErrors_ImageUpload_400_ManifestRequired(t *testing.T) {
	env := setupMatrixEnv(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	// no manifest field, just an empty multipart envelope
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "manifest_required" {
		t.Fatalf("error=%q want manifest_required", got.Error)
	}
}

func TestErrors_ImageUpload_400_FileCountMismatch(t *testing.T) {
	env := setupMatrixEnv(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	_ = mw.WriteField("manifest", `[{"client_image_id":"K1","position":0,"content_type":"image/jpeg"}]`)
	// no `files` field — count mismatch (manifest=1, files=0)
	mw.Close()

	req := httptest.NewRequest("POST", "/artworks/"+env.PID+"/images", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: "auth", Value: env.ownerToken})
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
	got := decodeError(t, rec.Body)
	if got.Error != "file_count_mismatch" {
		t.Fatalf("error=%q want file_count_mismatch", got.Error)
	}
}

// ---- /tags/{name} ----

func TestErrors_Tags_BadCursor(t *testing.T) {
	env := setupMatrixEnv(t)
	body, code := env.request(t, "anon", "GET", "/tags/t?cursor=not-base64!!")
	if code != 400 {
		t.Fatalf("status=%d want 400; body=%s", code, body)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_cursor" || got.Message == "" {
		t.Fatalf("expected Shape B bad_cursor, got %+v", got)
	}
}

// TestTags_HappyPath exercises the tags feed happy path including cursor encoding
// (encodeCursor non-nil branch) via the next_cursor in the response.
func TestTags_HappyPath(t *testing.T) {
	env := setupMatrixEnv(t)
	// Use limit=1 so the feed produces a next_cursor (encodeCursor non-nil path).
	body, code := env.request(t, "anon", "GET", "/tags/t?limit=1")
	if code != 200 {
		t.Fatalf("status=%d want 200; body=%s", code, body)
	}
}

// TestErrors_Cursor_MalformedParts exercises the parseCursor branch where
// base64 decodes but the "|"-split produces wrong number of fields.
func TestErrors_Cursor_MalformedParts(t *testing.T) {
	env := setupMatrixEnv(t)
	// "nopipe" base64-encodes cleanly but has no "|" separator → missing field error
	encoded := "bm9waXBl" // base64.RawURLEncoding of "nopipe"
	body, code := env.request(t, "anon", "GET", "/artworks?cursor="+encoded)
	if code != 400 {
		t.Fatalf("status=%d want 400; body=%s", code, body)
	}
	got := decodeError(t, strings.NewReader(body))
	if got.Error != "bad_cursor" {
		t.Fatalf("error=%q want bad_cursor", got.Error)
	}
}
