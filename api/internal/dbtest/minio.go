// api/internal/dbtest/minio.go
package dbtest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"
)

type MinioInfo struct {
	Endpoint  string // host:port, no scheme
	AccessKey string
	SecretKey string
	Bucket    string // bucket exists and is empty when StartMinio returns
}

var (
	minioOnce sync.Once
	minioInfo MinioInfo
	minioErr  error
)

// StartMinio boots a single MinIO container shared across all tests in the run
// (sync.Once mirrors StartPostgres) AND ensures the configured bucket exists.
// MinIO does not auto-create buckets, so callers receiving a MinioInfo can rely
// on the bucket being ready for Put/Get without further setup. Tests must not
// run in parallel with other tests that mutate buckets.
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
		// Create the bucket so consumers don't see NoSuchBucket on first Put.
		// Use AWS SDK v2 (already a transitive dep via internal/storage/r2.go)
		// rather than minio-go/v7 to keep the harness consistent with the
		// existing R2 test pattern.
		const bucket = "artweb-test"
		cli := s3.NewFromConfig(aws.Config{
			Region:      "us-east-1",
			Credentials: credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", ""),
		}, func(o *s3.Options) {
			o.BaseEndpoint = aws.String("http://" + endpoint)
			o.UsePathStyle = true
		})
		if _, err := cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)}); err != nil {
			minioErr = err
			return
		}
		minioInfo = MinioInfo{
			Endpoint:  endpoint,
			AccessKey: "minioadmin",
			SecretKey: "minioadmin",
			Bucket:    bucket,
		}
	})
	if minioErr != nil {
		t.Fatalf("minio harness: %v", minioErr)
	}
	return minioInfo
}
