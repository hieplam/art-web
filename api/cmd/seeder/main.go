// cmd/seeder is the standalone CLI that replaces the legacy POST /dev/seed
// route (Phase 1 Task 10). It builds the same fixture data that route used to
// produce, then prints the resulting JSON envelope to stdout.
//
// Usage:
//
//	seeder [-suffix=<deterministic-suffix>] [-many=<bulk-count>]
//
// The CLI reads its DB URL, JWT key, S3/R2 storage, and frontend home from
// the same env vars cmd/api uses, so a dev workflow can swap a curl call for
// a binary invocation with no other changes.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	authservice "local/art-web/api/internal/auth/service"
	infraconfig "local/art-web/api/internal/infrastructure/config"
	"local/art-web/api/internal/infrastructure/database"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	"local/art-web/api/internal/seeder"
)

func main() {
	suffix := flag.String("suffix", "", "deterministic slug suffix (default: random hex)")
	many := flag.Int("many", 0, "extra bulk public artworks to seed (max 200)")
	flag.Parse()

	if err := run(*suffix, *many); err != nil {
		fmt.Fprintln(os.Stderr, "seeder:", err)
		os.Exit(1)
	}
}

func run(suffix string, many int) error {
	ctx := context.Background()

	dsn := mustEnv("DATABASE_URL")
	db, cleanup, err := database.NewGormDB(infraconfig.DatabaseConfig{URL: dsn})
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer cleanup()

	store, err := buildStorage()
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}

	jwtKey, err := decodeHexKey("JWT_SIGNING_KEY", mustEnv("JWT_SIGNING_KEY"))
	if err != nil {
		return err
	}
	jwts := authservice.NewJWT(jwtKey, time.Now)

	out, err := seeder.Run(ctx, db, store, jwts, suffix, many)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	return enc.Encode(out)
}

// buildStorage mirrors cmd/api/main.go's storage selection so the seeder
// writes objects exactly where the running api would expect to read them.
func buildStorage() (infrastorage.Storage, error) {
	appEnv := getEnv("APP_ENV", "dev")
	endpoint := os.Getenv("S3_ENDPOINT")
	switch {
	case appEnv == "dev":
		return infrastorage.NewLocalFS("./var/storage"), nil
	case appEnv == "test" && endpoint == "":
		return infrastorage.NewLocalFS("./var/storage"), nil
	default:
		bucket := getEnv("R2_BUCKET", "art-dev")
		keyID := os.Getenv("R2_ACCESS_KEY_ID")
		keySecret := os.Getenv("R2_ACCESS_KEY_SECRET")
		account := os.Getenv("R2_ACCOUNT_ID")
		if endpoint == "" {
			endpoint = "https://" + account + ".r2.cloudflarestorage.com"
		}
		s3cli := s3.NewFromConfig(aws.Config{
			Region:      "auto",
			Credentials: credentials.NewStaticCredentialsProvider(keyID, keySecret, ""),
		}, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
		return infrastorage.NewR2(s3cli, bucket), nil
	}
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		fmt.Fprintf(os.Stderr, "seeder: missing env %s\n", k)
		os.Exit(1)
	}
	return v
}

func decodeHexKey(name, value string) ([]byte, error) {
	b, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("%s not valid hex: %w", name, err)
	}
	if len(b) < 32 {
		return nil, fmt.Errorf("%s decoded to %s bytes (need ≥32)", name, strconv.Itoa(len(b)))
	}
	return b, nil
}
