// api/internal/storage/r2_test.go
package storage_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	tcminio "github.com/testcontainers/testcontainers-go/modules/minio"

	"local/art-web/api/internal/storage"
)

func TestR2_PutGetMove(t *testing.T) {
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
	_, err = cli.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("art")})
	if err != nil {
		t.Fatalf("create bucket: %v", err)
	}

	s := storage.NewR2(cli, "art")

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
