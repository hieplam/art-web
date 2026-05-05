package server_test

import (
	"strings"
	"testing"
)


type privacyCase struct {
	name           string
	viewer         string
	method, path   string
	expectStatus   int
	mustNotContain []string
	mustContain    []string
}

func TestPrivacyMatrix_Cases1to5(t *testing.T) {
	env := setupMatrixEnv(t)
	cases := []privacyCase{
		// Case 1: public feed — never shows private artwork Q
		{name: "feed-anon", viewer: "anon", method: "GET", path: "/artworks", expectStatus: 200,
			mustNotContain: []string{env.QID}},
		{name: "feed-owner", viewer: "owner", method: "GET", path: "/artworks", expectStatus: 200,
			mustNotContain: []string{env.QID}},
		{name: "feed-other", viewer: "other", method: "GET", path: "/artworks", expectStatus: 200,
			mustNotContain: []string{env.QID}},
		// Case 2: private artwork detail — non-owner and anon get 404
		{name: "detail-Q-anon", viewer: "anon", method: "GET", path: "/artworks/" + env.QID, expectStatus: 404},
		{name: "detail-Q-other", viewer: "other", method: "GET", path: "/artworks/" + env.QID, expectStatus: 404},
		// Case 3: owner gets private artwork detail
		{name: "detail-Q-owner", viewer: "owner", method: "GET", path: "/artworks/" + env.QID, expectStatus: 200,
			mustContain: []string{env.QID}},
		// Case 4: user profile
		{name: "profile-anon", viewer: "anon", method: "GET", path: "/users/alice-" + env.AliceSuffix, expectStatus: 200,
			mustNotContain: []string{env.QID}},
		{name: "profile-owner", viewer: "owner", method: "GET", path: "/users/alice-" + env.AliceSuffix, expectStatus: 200,
			mustContain: []string{env.QID}},
		{name: "profile-other", viewer: "other", method: "GET", path: "/users/alice-" + env.AliceSuffix, expectStatus: 200,
			mustNotContain: []string{env.QID}},
		// Case 5: tag listing — never shows private artwork Q
		{name: "tag-anon", viewer: "anon", method: "GET", path: "/tags/t", expectStatus: 200,
			mustNotContain: []string{env.QID}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body, status := env.request(t, c.viewer, c.method, c.path)
			if status != c.expectStatus {
				t.Fatalf("status=%d want=%d body=%s", status, c.expectStatus, body)
			}
			for _, s := range c.mustNotContain {
				if strings.Contains(body, s) {
					t.Errorf("body must not contain %q; body=%s", s, body)
				}
			}
			for _, s := range c.mustContain {
				if !strings.Contains(body, s) {
					t.Errorf("body must contain %q; body=%s", s, body)
				}
			}
		})
	}
}

func TestPrivacyMatrix_Gaps(t *testing.T) {
	env := setupMatrixEnv(t)

	// All cells use a valid JSON body so PATCH cases get past the bad_json
	// short-circuit. We're testing privacy semantics, not body parsing.
	const validPatchBody = `{"title":"x"}`

	cases := []struct {
		name   string
		viewer string
		method string
		path   string
		body   string // empty for GET/DELETE
		want   int
	}{
		// other → private GET = 404 (not 403, per spec §5.2.0a)
		{"other_get_private", "other", "GET", "/artworks/" + env.QID, "", 404},
		// anon → private GET = 404
		{"anon_get_private", "anon", "GET", "/artworks/" + env.QID, "", 404},
		// other → public PATCH = 404 (non-owner)
		{"other_patch_public", "other", "PATCH", "/artworks/" + env.PID, validPatchBody, 404},
		// other → private PATCH = 404
		{"other_patch_private", "other", "PATCH", "/artworks/" + env.QID, validPatchBody, 404},
		// other → public DELETE = 404
		{"other_delete_public", "other", "DELETE", "/artworks/" + env.PID, "", 404},
		// other → private DELETE = 404
		{"other_delete_private", "other", "DELETE", "/artworks/" + env.QID, "", 404},
		// anon → public PATCH = 401
		{"anon_patch_public", "anon", "PATCH", "/artworks/" + env.PID, validPatchBody, 401},
		// anon → public DELETE = 401
		{"anon_delete_public", "anon", "DELETE", "/artworks/" + env.PID, "", 401},
		// anon → POST artwork = 401
		{"anon_post_artwork", "anon", "POST", "/artworks", `{"title":"x"}`, 401},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var code int
			if tc.body != "" {
				_, code = env.requestWithJSONBody(t, tc.viewer, tc.method, tc.path, tc.body)
			} else {
				_, code = env.request(t, tc.viewer, tc.method, tc.path)
			}
			if code != tc.want {
				t.Fatalf("status=%d want %d", code, tc.want)
			}
		})
	}
}

// TestPrivacyMatrix_OwnerOps_Succeed exercises owner success paths on PATCH and
// DELETE so those handlers' happy paths get coverage too. Currently these
// handlers are 2.6% / 9.1% covered — the privacy-matrix-gap tests above hit
// only the early-return error paths.
func TestPrivacyMatrix_OwnerOps_Succeed(t *testing.T) {
	env := setupMatrixEnv(t)

	// Owner PATCH the public artwork's title — should return 204.
	_, code := env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID,
		`{"title":"renamed"}`)
	if code != 204 {
		t.Fatalf("owner PATCH status=%d want 204", code)
	}

	// Owner PATCH the public artwork to flip visibility (private→public was
	// the seeded state; flip to private exercises the visibility-flip path).
	_, code = env.requestWithJSONBody(t, "owner", "PATCH", "/artworks/"+env.PID,
		`{"visibility":"private"}`)
	if code != 204 {
		t.Fatalf("owner PATCH visibility flip status=%d want 204", code)
	}

	// Owner DELETE the now-private (was-public) artwork — should return 204.
	_, code = env.request(t, "owner", "DELETE", "/artworks/"+env.PID)
	if code != 204 {
		t.Fatalf("owner DELETE status=%d want 204", code)
	}

	// Owner POST a new artwork — should return 201 with an "id" body.
	body, code := env.requestWithJSONBody(t, "owner", "POST", "/artworks",
		`{"title":"new","visibility":"public"}`)
	if code != 201 {
		t.Fatalf("owner POST status=%d want 201; body=%s", code, body)
	}
	if !strings.Contains(body, `"id"`) {
		t.Fatalf("owner POST body should contain id; got %s", body)
	}
}
