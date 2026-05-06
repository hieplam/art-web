package testing_test

import (
	"net/http"
	"testing"

	infratest "local/art-web/api/internal/infrastructure/testing"
)

func TestBootApp_HealthzReturns200(t *testing.T) {
	booted := infratest.BootApp(t, infratest.BootOpts{})

	resp, err := http.Get(booted.Server.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status=%d want 200", resp.StatusCode)
	}
}
