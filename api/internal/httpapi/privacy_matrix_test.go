package httpapi_test

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
