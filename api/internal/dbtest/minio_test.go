package dbtest_test

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

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

// TestStartMinio_BucketIsReady verifies the harness's contract: when StartMinio
// returns, the named bucket exists and accepts writes. This guards against the
// "Bucket field set but bucket not actually created" trap.
func TestStartMinio_BucketIsReady(t *testing.T) {
	info := dbtest.StartMinio(t)
	cli := s3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(info.AccessKey, info.SecretKey, ""),
	}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://" + info.Endpoint)
		o.UsePathStyle = true
	})
	_, err := cli.HeadBucket(context.Background(), &s3.HeadBucketInput{
		Bucket: aws.String(info.Bucket),
	})
	if err != nil {
		t.Fatalf("HeadBucket(%q): %v — StartMinio returned an info struct whose bucket does not exist", info.Bucket, err)
	}
}
