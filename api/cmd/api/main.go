package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog/log"

	artworkpostgres "local/art-web/api/internal/artwork/adapters/postgres"
	artworkservice "local/art-web/api/internal/artwork/service"
	authhttp "local/art-web/api/internal/auth/adapters/http"
	authoauth "local/art-web/api/internal/auth/adapters/oauth"
	authports "local/art-web/api/internal/auth/ports"
	authservice "local/art-web/api/internal/auth/service"
	imagehttp "local/art-web/api/internal/image/adapters/http"
	imagepostgres "local/art-web/api/internal/image/adapters/postgres"
	imageservice "local/art-web/api/internal/image/service"
	infraconfig "local/art-web/api/internal/infrastructure/config"
	"local/art-web/api/internal/infrastructure/database"
	infralogger "local/art-web/api/internal/infrastructure/logger"
	"local/art-web/api/internal/infrastructure/server"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/pkg/signing"
)

func main() {
	cfg := loadConfig()
	// infralogger.New writes to stderr and installs zerolog.SetGlobalLevel
	// from LOG_LEVEL (defaulting to InfoLevel on missing/invalid values).
	logger := infralogger.New(infraconfig.LoggerConfig{Level: os.Getenv("LOG_LEVEL")})
	ctx := context.Background()

	if err := database.MigrateUp(ctx, cfg.DatabaseURL); err != nil {
		log.Error().Err(err).Msg("migrate")
		os.Exit(1)
	}
	pool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error().Err(err).Msg("db")
		os.Exit(1)
	}
	defer pool.Close()

	var store infrastorage.Storage
	switch {
	case cfg.AppEnv == "dev":
		store = infrastorage.NewLocalFS("./var/storage")
	case cfg.AppEnv == "test" && cfg.S3Endpoint == "":
		store = infrastorage.NewLocalFS("./var/storage")
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
		store = infrastorage.NewR2(s3cli, cfg.R2Bucket)
	}

	signKey := mustDecodeHexKey("WORKER_SIGNING_KEY", cfg.WorkerSigningKey)
	jwtKey := mustDecodeHexKey("JWT_SIGNING_KEY", cfg.JWTSigningKey)

	jwts := authservice.NewJWT(jwtKey, time.Now)
	urls := signing.NewURLBuilder(cfg.CDNOrigin, signKey, time.Now)

	users := userpostgres.NewRepo(pool)
	arts := artworkpostgres.NewRepo(pool)
	tags := artworkpostgres.NewTagsRepo(pool)
	images := imagepostgres.NewRepo(pool)
	imgSvc := imageservice.NewService(store, images, arts)
	upload := imagehttp.NewHandler(imgSvc, arts, urls)
	vis := artworkservice.NewVisibilityService(arts, store)

	artworkservice.RollbackLog = func(err error) { log.Error().Err(err).Msg("flip rollback") }

	r := server.New(&server.Deps{
		AppEnv: cfg.AppEnv,
		JWT:    jwts, URL: urls,
		Providers: map[string]authports.Provider{
			"google": authoauth.NewGoogleProvider(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL),
		},
		Users: users, Artworks: arts, Tags: tags, Images: images, Store: store,
		Upload: upload, Vis: vis,
		Frontend:      cfg.FrontendURL,
		AllowedOrigin: cfg.AllowedOrigin,
		CookieOpts: authhttp.CookieOpts{
			Domain: cfg.CookieDomain,
			Secure: secureCookieForEnv(cfg.AppEnv),
		},
		Logger: logger,
	})

	log.Info().Str("addr", cfg.Addr).Msg("listening")
	if err := http.ListenAndServe(cfg.Addr, r); err != nil {
		log.Error().Err(err).Msg("server")
		os.Exit(1)
	}
}

func secureCookieForEnv(appEnv string) bool {
	return appEnv != "dev" && appEnv != "test"
}
