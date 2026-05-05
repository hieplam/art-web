// api/internal/storage/r2.go
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type r2Store struct {
	cli    *s3.Client
	bucket string
}

func NewR2(cli *s3.Client, bucket string) Storage { return &r2Store{cli: cli, bucket: bucket} }

func (r *r2Store) Put(ctx context.Context, key string, body io.Reader, ct string) error {
	buf, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	_, err = r.cli.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(r.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(buf),
		ContentType: aws.String(ct),
	})
	return err
}

func (r *r2Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := r.cli.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (r *r2Store) Delete(ctx context.Context, key string) error {
	_, err := r.cli.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(key),
	})
	return err
}

func (r *r2Store) Move(ctx context.Context, src, dst string) error {
	_, err := r.cli.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(r.bucket),
		CopySource: aws.String(r.bucket + "/" + src),
		Key:        aws.String(dst),
	})
	if err != nil {
		return err
	}
	return r.Delete(ctx, src)
}

func (r *r2Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.cli.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.bucket), Key: aws.String(key),
	})
	if err != nil {
		var nf *types.NotFound
		if errors.As(err, &nf) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (r *r2Store) SignedURL(_ context.Context, _ string, _ time.Duration) (string, error) {
	return "", errors.New("r2 store: signing handled by Worker, not direct presign")
}
