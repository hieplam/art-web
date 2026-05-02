package httpapi

import (
	"net/http"
	"strings"
)

func CORSFor(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(204)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CacheControlByPath() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch h := classifyCachePath(r.URL.Path); h {
			case cachePublicShort:
				w.Header().Set("Cache-Control", "public, max-age=60")
			case cachePrivateNoStore:
				w.Header().Set("Cache-Control", "private, no-store")
			}
			next.ServeHTTP(w, r)
		})
	}
}

type cacheHint int

const (
	cacheNone cacheHint = iota
	cachePublicShort
	cachePrivateNoStore
)

func classifyCachePath(p string) cacheHint {
	switch p {
	case "/artworks":
		return cachePublicShort
	case "/me":
		return cachePrivateNoStore
	}
	switch {
	case strings.HasPrefix(p, "/tags/"):
		return cachePublicShort
	case strings.HasPrefix(p, "/users/"):
		return cachePrivateNoStore
	case strings.HasPrefix(p, "/artworks/"):
		return cachePrivateNoStore
	}
	return cacheNone
}
