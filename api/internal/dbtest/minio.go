// api/internal/dbtest/minio.go
package dbtest

import (
	"context"
	"sync"
	"testing"
	"time"

	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

type MinioInfo struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	Bucket    string
}

var (
	minioOnce sync.Once
	minioInfo MinioInfo
	minioErr  error
)

// StartMinio boots a single MinIO container shared across all tests in the run
// (sync.Once mirrors StartPostgres). Tests must not run in parallel with other
// tests that mutate buckets.
func StartMinio(t testing.TB) MinioInfo {
	t.Helper()
	minioOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		container, err := tcminio.Run(ctx, "minio/minio:latest",
			tcminio.WithUsername("minioadmin"),
			tcminio.WithPassword("minioadmin"),
		)
		if err != nil {
			minioErr = err
			return
		}
		endpoint, err := container.ConnectionString(ctx)
		if err != nil {
			minioErr = err
			return
		}
		minioInfo = MinioInfo{
			Endpoint:  endpoint,
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Bucket:    "artweb-test",
		}
	})
	if minioErr != nil {
		t.Fatalf("minio harness: %v", minioErr)
	}
	return minioInfo
}
