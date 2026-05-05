// api/internal/infrastructure/config/config.go
package config

import (
	"errors"
	"os"
	"strconv"
)

// AppConfig is the root config. Each sub-config is provided to consumers via
// wire.FieldsOf so each slice receives only what it needs (spec §6.7).
type AppConfig struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Storage  StorageConfig
	Image    ImageConfig
	Logger   LoggerConfig
}

type ServerConfig struct {
	AppEnv        string
	Frontend      string
	AllowedOrigin string
	Port          int
}

type DatabaseConfig struct {
	URL string
}

type AuthConfig struct {
	JWTKey  string
	SignKey string
	Cookie  CookieConfig
}

type CookieConfig struct {
	Domain string
	Secure bool
}

type StorageConfig struct {
	Backend string // "localfs" or "r2"
	Root    string // localfs only
	Bucket  string // r2 only
}

type ImageConfig struct {
	MaxBytes int64
}

type LoggerConfig struct {
	Level string // debug, info, warn, error
}

// Load reads env vars and returns AppConfig. Errors on missing required keys.
func Load() (*AppConfig, error) {
	cfg := &AppConfig{
		Server: ServerConfig{
			AppEnv:        getenv("APP_ENV", "production"),
			Frontend:      getenv("FRONTEND_URL", "http://localhost:3000/"),
			AllowedOrigin: getenv("ALLOWED_ORIGIN", "http://localhost:3000"),
			Port:          atoi(getenv("PORT", "8787")),
		},
		Database: DatabaseConfig{URL: os.Getenv("DATABASE_URL")},
		Auth: AuthConfig{
			JWTKey:  os.Getenv("JWT_KEY"),
			SignKey: os.Getenv("URL_SIGN_KEY"),
			Cookie: CookieConfig{
				Domain: getenv("COOKIE_DOMAIN", ""),
				Secure: getenv("COOKIE_SECURE", "true") == "true",
			},
		},
		Storage: StorageConfig{
			Backend: getenv("STORAGE_BACKEND", "localfs"),
			Root:    getenv("STORAGE_ROOT", "/tmp/art-web"),
			Bucket:  os.Getenv("R2_BUCKET"),
		},
		Image:  ImageConfig{MaxBytes: int64(atoi(getenv("IMAGE_MAX_BYTES", "26214401")))},
		Logger: LoggerConfig{Level: getenv("LOG_LEVEL", "info")},
	}
	if cfg.Database.URL == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	if cfg.Auth.JWTKey == "" || cfg.Auth.SignKey == "" {
		return nil, errors.New("JWT_KEY and URL_SIGN_KEY are required")
	}
	return cfg, nil
}

func getenv(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
