package dbtest_test

import (
	"testing"

	"local/art-web/api/internal/dbtest"
)

func TestStartMinio_ReturnsEndpoint(t *testing.T) {
	info := dbtest.StartMinio(t)
	if info.Endpoint == "" {
		t.Fatal("StartMinio returned empty endpoint")
	}
	if info.Bucket == "" {
		t.Fatal("StartMinio returned empty bucket")
	}
}
