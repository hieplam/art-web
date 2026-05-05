// api/internal/auth/adapters/http/middleware.go
package http

import (
	"context"
	"net/http"

	"local/art-web/api/internal/auth/service"
)

type ctxKey int

const userIDKey ctxKey = 1

func Middleware(j *service.JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie("auth")
			if err == nil {
				if sub, err := j.Verify(c.Value); err == nil {
					r = r.WithContext(context.WithValue(r.Context(), userIDKey, sub))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserIDFrom(r.Context()); !ok {
			jsonError(w, `{"error":"unauthorized"}`, 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserIDFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok && v != ""
}
