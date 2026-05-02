package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"local/art-web/api/internal/artwork"
	"local/art-web/api/internal/auth"
	"local/art-web/api/internal/db"
	"local/art-web/api/internal/httpapi"
	"local/art-web/api/internal/image"
	"local/art-web/api/internal/storage"
	"local/art-web/api/internal/user"
)

func main() {
	cfg := loadConfig()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	ctx := context.Background()

	if err := db.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		log.Error("migrate", "err", err)
		os.Exit(1)
	}
	pool, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("db", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	var store storage.Storage
	switch {
	case cfg.AppEnv == "dev":
		store = storage.NewLocalFS("./var/storage")
	case cfg.AppEnv == "test" && cfg.S3Endpoint == "":
		store = storage.NewLocalFS("./var/storage")
	default:
		endpoint := cfg.S3Endpoint
		if endpoint == "" {
			endpoint = "https://" + cfg.R2AccountID + ".r2.cloudflarestorage.com"
		}
		s3cli := s3.NewFromConfig(aws.Config{
			Region:      "auto",
			Credentials: credentials.NewStaticCredentialsProvider(cfg.R2KeyID, cfg.R2KeySecret, ""),
		}, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
		store = storage.NewR2(s3cli, cfg.R2Bucket)
	}

	signKey := mustDecodeHexKey("WORKER_SIGNING_KEY", cfg.WorkerSigningKey)
	jwtKey := mustDecodeHexKey("JWT_SIGNING_KEY", cfg.JWTSigningKey)

	jwts := auth.NewJWT(jwtKey, time.Now)
	urls := auth.NewURLBuilder(cfg.CDNOrigin, signKey, time.Now)

	users := user.NewRepo(pool)
	arts := artwork.NewRepo(pool)
	tags := artwork.NewTagsRepo(pool)
	images := image.NewRepo(pool)
	imgSvc := image.NewService(store, images, arts)
	upload := image.NewHandler(imgSvc, arts, urls)
	vis := artwork.NewVisibilityService(arts, store)

	artwork.RollbackLog = func(err error) { log.Error("flip rollback", "err", err) }

	r := httpapi.New(&httpapi.Deps{
		AppEnv: cfg.AppEnv,
		JWT:    jwts, URL: urls,
		Providers: map[string]auth.Provider{
			"google": auth.NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL),
		},
		Users: users, Artworks: arts, Tags: tags, Images: images, Store: store,
		Upload: upload, Vis: vis,
		Frontend:      cfg.FrontendURL,
		AllowedOrigin: cfg.AllowedOrigin,
		CookieOpts: auth.CookieOpts{
			Domain: cfg.CookieDomain,
			Secure: secureCookieForEnv(cfg.AppEnv),
		},
	})

	log.Info("listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Error("server", "err", err)
		os.Exit(1)
	}
}

func secureCookieForEnv(appEnv string) bool {
	return appEnv != "dev" && appEnv != "test"
}
