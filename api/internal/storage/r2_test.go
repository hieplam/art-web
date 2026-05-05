// api/internal/storage/r2_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"local/art-web/api/internal/storage"
)

func TestR2_PutGetMove(t *testing.T) {
	cli := newR2Client(t)
	s := storage.NewR2(cli, "art")
	ctx := context.Background()

	if err := s.Put(ctx, "public/x/0.jpg", bytes.NewReader([]byte("hi")), "image/jpeg"); err != nil {
		t.Fatalf("put: %v", err)
	}
	r, err := s.Get(ctx, "public/x/0.jpg")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	got, _ := io.ReadAll(r)
	r.Close()
	if string(got) != "hi" {
		t.Fatalf("want hi got %q", got)
	}

	if err := s.Move(ctx, "public/x/0.jpg", "private/x/0.jpg"); err != nil {
		t.Fatalf("move: %v", err)
	}
	if ok, _ := s.Exists(ctx, "public/x/0.jpg"); ok {
		t.Fatal("src still exists after move")
	}
	if ok, _ := s.Exists(ctx, "private/x/0.jpg"); !ok {
		t.Fatal("dst missing after move")
	}
}

func TestR2_Get_MissingKey_ReturnsError(t *testing.T) {
	cli := newR2Client(t)
	s := storage.NewR2(cli, "art")
	_, err := s.Get(context.Background(), "no/such/key.jpg")
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestR2_Delete_MissingKey_NoError(t *testing.T) {
	cli := newR2Client(t)
	s := storage.NewR2(cli, "art")
	// S3/MinIO DeleteObject is idempotent — deleting a non-existent key returns 200/204.
	if err := s.Delete(context.Background(), "no/such/key.jpg"); err != nil {
		t.Fatalf("expected nil for missing-key Delete, got %v", err)
	}
}

func TestR2_Exists_FalseForMissing(t *testing.T) {
	cli := newR2Client(t)
	s := storage.NewR2(cli, "art")
	ok, err := s.Exists(context.Background(), "no/such/key.jpg")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if ok {
		t.Fatal("expected false for missing key")
	}
}

// TestR2_SignedURL_ReturnsUnsupported pins the contract that the R2 adapter
// does NOT presign URLs directly — signing is delegated to a separate Worker
// (see r2.go:80-82). HTTP-level signed URLs come from auth.URLBuilder.
func TestR2_SignedURL_ReturnsUnsupported(t *testing.T) {
	cli := newR2Client(t)
	s := storage.NewR2(cli, "art")
	url, err := s.SignedURL(context.Background(), "k", 5*time.Minute)
	if err == nil {
		t.Fatal("expected error from R2 SignedURL")
	}
	if url != "" {
		t.Fatalf("expected empty URL on error, got %q", url)
	}
	if !strings.Contains(err.Error(), "signing handled by Worker") {
		t.Fatalf("expected 'signing handled by Worker' in error, got %v", err)
	}
}

// newR2Client extracts the MinIO + S3-client setup pattern from
// TestR2_PutGetMove so the four tests above don't duplicate 15 lines each.
// It boots a per-test MinIO container, creates the "art" bucket, and returns
// the configured S3 client. The container is torn down via t.Cleanup.
func newR2Client(t *testing.T) *s3.Client {
	t.Helper()
	ctx := context.Background()
	c, err := tcminio.Run(ctx, "minio/minio:RELEASE.2024-12-18T13-15-44Z")
	if err != nil {
		t.Fatalf("minio: %v", err)
	}
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	endpoint, _ := c.ConnectionString(ctx)
	cli := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", ""),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://" + endpoint)
		o.UsePathStyle = true
	})
	if _, err := cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("art")}); err != nil {
		t.Fatalf("create bucket: %v", err)
	}
	return cli
}
