package contract_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"local/art-web/api/internal/dbtest"
	"local/art-web/api/internal/httpapi/contract"
	"local/art-web/api/internal/image"
)

// fixedNow is the deterministic clock for every contract test.
var fixedNow = time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)

// fixedRand returns a deterministic 16-byte source for OAuth state.
type fixedRand struct{}

func (fixedRand) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i + 1)
	}
	return len(p), nil
}

// seedResponse mirrors the /dev/seed response shape (httpapi/devseed.go).
//
// IMPORTANT: AliceCookie / BobCookie are the FULL Set-Cookie value
// ("auth=<JWT>") — devseed.go builds them as "auth=" + jwt. Strip the
// "auth=" prefix before using them as a cookie value, otherwise requests
// produce `Cookie: auth=auth=<JWT>` and JWT verification fails with 401.
type seedResponse struct {
	AliceCookie string `json:"aliceCookie"`
	BobCookie   string `json:"bobCookie"`
	AliceSlug   string `json:"aliceSlug"`
	PID         string `json:"pId"` // public artwork
	QID         string `json:"qId"` // private artwork
}

// jwtFromAuthCookie strips the "auth=" prefix devseed writes around the JWT.
func jwtFromAuthCookie(s string) string { return strings.TrimPrefix(s, "auth=") }

// fixedOAuthState is what randState() returns when stateRand is fixedRand{}.
// fixedRand fills 16 bytes with 0x01..0x10; hex-encoded that's
// "0102030405060708090a0b0c0d0e0f10". Cells targeting /auth/google/callback
// pre-set this cookie so the handler's state-match check passes.
const fixedOAuthState = "0102030405060708090a0b0c0d0e0f10"

type contractCase struct {
	name    string
	method  string
	path    string
	viewer  string // "anon", "owner", "other"
	body    string // body bytes; empty allowed
	ctype   string // Content-Type override; if empty and body != "" defaults to application/json
	bodyMP  func(t *testing.T) (io.Reader, string) // optional: builds a multipart body & returns ctype
	cookies []*http.Cookie                         // optional: extra cookies (e.g. oauth_state for callback cells)
}

// noRedirectClient prevents auto-following 302s so we capture the redirect
// as the actual response — auth/start and auth/callback both 302.
var noRedirectClient = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func bootContract(t *testing.T) (string, seedResponse, func(c contractCase) *http.Response) {
	srv := dbtest.BootApp(t, dbtest.BootOpts{
		FixedNow:   fixedNow,
		IDProvider: &image.CounterIDProvider{},
		RandReader: fixedRand{},
		Providers:  contract.FakeProviders(),
	})

	// Seed once via /dev/seed?suffix=fixed1234 — deterministic suffix so the
	// resulting AliceSlug/BobSlug are stable across runs.
	r, err := noRedirectClient.Post(srv.URL+"/dev/seed?suffix=fixed1234", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /dev/seed: %v", err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("seed status=%d want 200", r.StatusCode)
	}
	var seed seedResponse
	if err := json.NewDecoder(r.Body).Decode(&seed); err != nil {
		t.Fatalf("decode seed: %v", err)
	}

	send := func(c contractCase) *http.Response {
		var bodyReader io.Reader
		var ctype string
		switch {
		case c.bodyMP != nil:
			bodyReader, ctype = c.bodyMP(t)
		case c.body != "":
			bodyReader = strings.NewReader(c.body)
			ctype = "application/json"
		default:
			bodyReader = nil
		}
		if c.ctype != "" {
			ctype = c.ctype
		}
		req, _ := http.NewRequest(c.method, srv.URL+c.path, bodyReader)
		if ctype != "" {
			req.Header.Set("Content-Type", ctype)
		}
		switch c.viewer {
		case "owner":
			req.AddCookie(&http.Cookie{Name: "auth", Value: jwtFromAuthCookie(seed.AliceCookie)})
		case "other":
			req.AddCookie(&http.Cookie{Name: "auth", Value: jwtFromAuthCookie(seed.BobCookie)})
		case "anon":
			// no cookie
		default:
			t.Fatalf("unknown viewer %q", c.viewer)
		}
		for _, ck := range c.cookies {
			req.AddCookie(ck)
		}
		resp, err := noRedirectClient.Do(req)
		if err != nil {
			t.Fatalf("do %s %s: %v", c.method, c.path, err)
		}
		return resp
	}

	return srv.URL, seed, send
}

// validImageUpload returns a multipart body builder that uploads a tiny PNG.
func validImageUpload(clientID string, position int) func(*testing.T) (io.Reader, string) {
	return func(t *testing.T) (io.Reader, string) {
		buf := &bytes.Buffer{}
		mw := multipart.NewWriter(buf)
		manifest := `[{"client_image_id":"` + clientID + `","position":` +
			strconv.Itoa(position) + `,"content_type":"image/png"}]`
		_ = mw.WriteField("manifest", manifest)
		w, _ := mw.CreateFormFile("files", clientID+".png")
		_, _ = w.Write(seedPNGBytes)
		_ = mw.Close()
		return buf, mw.FormDataContentType()
	}
}

// seedPNGBytes is a 1×1 transparent PNG (67 bytes), copied from testutil_test.go.
var seedPNGBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x04, 0x00, 0x00, 0x00, 0xb5, 0x1c, 0x0c, 0x02, 0x00, 0x00, 0x00,
	0x0b, 0x49, 0x44, 0x41, 0x54, 0x78, 0xda, 0x63, 0xfc, 0xff, 0x1f, 0x00,
	0x03, 0x03, 0x02, 0x00, 0xef, 0xbf, 0xa7, 0xdb, 0x00, 0x00, 0x00, 0x00,
	0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// TestContractMatrix enumerates every (auth × resource × shape) cell from spec
// §5.2. Each cell asserts against a checked-in golden. Until PR 0.8 these fail.
func TestContractMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("contract suite is slow; run without -short")
	}
	_, seed, send := bootContract(t)

	missingID := "00000000-0000-0000-0000-000000000000"
	cases := []contractCase{
		// === /healthz ===
		{name: "healthz_anon", method: "GET", path: "/healthz", viewer: "anon"},

		// === /me ===
		{name: "me_anon_401", method: "GET", path: "/me", viewer: "anon"},
		{name: "me_owner_200", method: "GET", path: "/me", viewer: "owner"},
		{name: "me_other_200", method: "GET", path: "/me", viewer: "other"},

		// === GET /artworks (feed) ===
		{name: "feed_anon", method: "GET", path: "/artworks", viewer: "anon"},
		{name: "feed_owner", method: "GET", path: "/artworks", viewer: "owner"},
		{name: "feed_anon_limit_1", method: "GET", path: "/artworks?limit=1", viewer: "anon"},
		{name: "feed_anon_limit_oob", method: "GET", path: "/artworks?limit=9999", viewer: "anon"},
		{name: "feed_anon_bad_cursor", method: "GET", path: "/artworks?cursor=not-base64!!", viewer: "anon"},

		// === POST /artworks ===
		{name: "create_anon_401", method: "POST", path: "/artworks", viewer: "anon", body: `{"title":"x"}`},
		{name: "create_owner_minimal", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x"}`},
		{name: "create_owner_full", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x","description":"d","visibility":"public","tags":["a","b"]}`},
		{name: "create_owner_default_visibility", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x","visibility":""}`},
		{name: "create_owner_bad_json", method: "POST", path: "/artworks", viewer: "owner", body: `not json`},
		{name: "create_owner_bad_visibility", method: "POST", path: "/artworks", viewer: "owner", body: `{"title":"x","visibility":"draft"}`},

		// === GET /artworks/{id} — full 7-cell privacy matrix ===
		{name: "get_artwork_owner_public", method: "GET", path: "/artworks/" + seed.PID, viewer: "owner"},
		{name: "get_artwork_other_public", method: "GET", path: "/artworks/" + seed.PID, viewer: "other"},
		{name: "get_artwork_anon_public", method: "GET", path: "/artworks/" + seed.PID, viewer: "anon"},
		{name: "get_artwork_owner_private", method: "GET", path: "/artworks/" + seed.QID, viewer: "owner"},
		{name: "get_artwork_other_private_404", method: "GET", path: "/artworks/" + seed.QID, viewer: "other"},
		{name: "get_artwork_anon_private_404", method: "GET", path: "/artworks/" + seed.QID, viewer: "anon"},
		{name: "get_artwork_anon_missing", method: "GET", path: "/artworks/" + missingID, viewer: "anon"},

		// === PATCH /artworks/{id} ===
		{name: "patch_anon_401", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "anon", body: `{"title":"x"}`},
		{name: "patch_owner_title_204", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"title":"renamed"}`},
		{name: "patch_owner_same_visibility_204", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"visibility":"public"}`},
		{name: "patch_owner_flip_to_private_204", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"visibility":"private"}`},
		{name: "patch_owner_full_204", method: "PATCH", path: "/artworks/" + seed.QID, viewer: "owner", body: `{"title":"t","description":"d","cover_position":0,"tags":["a"]}`},
		{name: "patch_owner_bad_json", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `not json`},
		{name: "patch_owner_bad_visibility", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"visibility":"draft"}`},
		{name: "patch_owner_bad_cover_position", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "owner", body: `{"cover_position":-1}`},
		{name: "patch_other_public_404", method: "PATCH", path: "/artworks/" + seed.PID, viewer: "other", body: `{"title":"x"}`},
		{name: "patch_other_private_404", method: "PATCH", path: "/artworks/" + seed.QID, viewer: "other", body: `{"title":"x"}`},

		// === DELETE /artworks/{id} ===
		{name: "delete_anon_401", method: "DELETE", path: "/artworks/" + seed.PID, viewer: "anon"},
		{name: "delete_other_public_404", method: "DELETE", path: "/artworks/" + seed.PID, viewer: "other"},
		{name: "delete_other_private_404", method: "DELETE", path: "/artworks/" + seed.QID, viewer: "other"},
		{name: "delete_owner_missing_404", method: "DELETE", path: "/artworks/" + missingID, viewer: "owner"},

		// === POST /artworks/{id}/images ===
		{name: "upload_anon_401", method: "POST", path: "/artworks/" + seed.PID + "/images", viewer: "anon",
			ctype: "multipart/form-data; boundary=fake"},
		{name: "upload_other_404", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "other",
			ctype: "multipart/form-data; boundary=fake"},
		{name: "upload_owner_415_non_multipart", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			body: `{}`},
		{name: "upload_owner_400_no_manifest", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			bodyMP: func(t *testing.T) (io.Reader, string) {
				buf := &bytes.Buffer{}
				mw := multipart.NewWriter(buf)
				_ = mw.Close()
				return buf, mw.FormDataContentType()
			}},
		{name: "upload_owner_400_file_count_mismatch", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			bodyMP: func(t *testing.T) (io.Reader, string) {
				buf := &bytes.Buffer{}
				mw := multipart.NewWriter(buf)
				_ = mw.WriteField("manifest", `[{"client_image_id":"K","position":0,"content_type":"image/png"}]`)
				_ = mw.Close()
				return buf, mw.FormDataContentType()
			}},
		{name: "upload_owner_201", method: "POST", path: "/artworks/" + seed.QID + "/images", viewer: "owner",
			bodyMP: validImageUpload("K-new", 1)},

		// === /users/{slug} ===
		{name: "user_profile_owner_self", method: "GET", path: "/users/" + seed.AliceSlug, viewer: "owner"},
		{name: "user_profile_anon", method: "GET", path: "/users/" + seed.AliceSlug, viewer: "anon"},
		{name: "user_profile_missing_slug", method: "GET", path: "/users/no-such-slug", viewer: "anon"},
		{name: "user_profile_bad_cursor", method: "GET", path: "/users/" + seed.AliceSlug + "?cursor=not-base64!!", viewer: "anon"},

		// === /tags/{name} ===
		{name: "tag_present", method: "GET", path: "/tags/t", viewer: "anon"},
		{name: "tag_missing", method: "GET", path: "/tags/no-such-tag", viewer: "anon"},

		// === /auth/{provider}/start — fake provider produces 302 ===
		{name: "auth_google_start_anon_302", method: "GET", path: "/auth/google/start", viewer: "anon"},
		{name: "auth_unknown_provider_404", method: "GET", path: "/auth/notreal/start", viewer: "anon"},

		// === /auth/{provider}/callback ===
		{name: "auth_callback_unknown_provider_404", method: "GET",
			path: "/auth/notreal/callback?state=" + fixedOAuthState + "&code=y", viewer: "anon"},
		{name: "auth_callback_bad_state_400", method: "GET",
			path:    "/auth/google/callback?state=mismatch&code=y",
			viewer:  "anon",
			cookies: []*http.Cookie{{Name: "oauth_state", Value: fixedOAuthState}},
		},
		{name: "auth_callback_success_302", method: "GET",
			path:    "/auth/google/callback?state=" + fixedOAuthState + "&code=valid",
			viewer:  "anon",
			cookies: []*http.Cookie{{Name: "oauth_state", Value: fixedOAuthState}},
		},
		{name: "auth_callback_exchange_failed_502", method: "GET",
			path:    "/auth/google/callback?state=" + fixedOAuthState + "&code=fail",
			viewer:  "anon",
			cookies: []*http.Cookie{{Name: "oauth_state", Value: fixedOAuthState}},
		},

		// === /auth/logout ===
		{name: "logout_anon_204", method: "POST", path: "/auth/logout", viewer: "anon"},
		{name: "logout_owner_204", method: "POST", path: "/auth/logout", viewer: "owner"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := send(c)
			defer resp.Body.Close()
			dbtest.AssertGolden(t, c.name, resp)
		})
	}
}
