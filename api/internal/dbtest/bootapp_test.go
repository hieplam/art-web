package dbtest_test

import (
	"net/http"
	"testing"

	"local/art-web/api/internal/dbtest"
)

func TestBootApp_HealthzReturns200(t *testing.T) {
	srv := dbtest.BootApp(t, dbtest.BootOpts{})

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
}
