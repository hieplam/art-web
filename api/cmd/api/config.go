package main

import (
	"encoding/hex"
	"os"
	"strconv"
)

type config struct {
	Addr, AppEnv, DatabaseURL, FrontendURL, CDNOrigin            string
	WorkerSigningKey, JWTSigningKey, CookieDomain, AllowedOrigin string
	GoogleClientID, GoogleClientSecret, GoogleRedirectURL        string
	R2AccountID, R2KeyID, R2KeySecret, R2Bucket                  string
	S3Endpoint                                                   string
}

func loadConfig() config {
	return config{
		Addr:               getEnv("ADDR", ":8080"),
		AppEnv:             getEnv("APP_ENV", "dev"),
		DatabaseURL:        mustEnv("DATABASE_URL"),
		FrontendURL:        getEnv("FRONTEND_URL", "http://localhost:3000/"),
		CDNOrigin:          getEnv("CDN_ORIGIN", "http://localhost:8787"),
		WorkerSigningKey:   mustEnv("WORKER_SIGNING_KEY"),
		JWTSigningKey:      mustEnv("JWT_SIGNING_KEY"),
		CookieDomain:       getEnv("COOKIE_DOMAIN", ""),
		AllowedOrigin:      getEnv("ALLOWED_ORIGIN", "http://localhost:3000"),
		GoogleClientID:     getEnv("GOOGLE_OAUTH_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_OAUTH_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_OAUTH_REDIRECT_URL", "http://localhost:8080/auth/google/callback"),
		R2AccountID:        getEnv("R2_ACCOUNT_ID", ""),
		R2KeyID:            getEnv("R2_ACCESS_KEY_ID", ""),
		R2KeySecret:        getEnv("R2_ACCESS_KEY_SECRET", ""),
		R2Bucket:           getEnv("R2_BUCKET", "art-dev"),
		S3Endpoint:         getEnv("S3_ENDPOINT", ""),
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
		panic("missing env: " + k)
	}
	return v
}

func mustDecodeHexKey(name, value string) []byte {
	if value == "" {
		panic(name + " is empty")
	}
	b, err := hex.DecodeString(value)
	if err != nil {
		panic(name + " is not valid hex: " + err.Error())
	}
	if len(b) < 32 {
		panic(name + " decoded to fewer than 32 bytes (got " + strconv.Itoa(len(b)) + ")")
	}
	return b
}
