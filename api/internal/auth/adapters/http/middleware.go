// api/internal/auth/adapters/http/middleware.go
package http

import (
	"context"
	"net/http"

	"local/art-web/api/internal/auth/service"
)

type ctxKey int

const userIDKey ctxKey = 1

// Middleware bundles the auth slice's chi middlewares so other slice routers
// can inject one instance via Wire instead of passing a *service.JWT around
// directly. ParseToken populates the request context when a valid auth cookie
// is present; RequireUser is the route gate that 401s anonymous callers.
type Middleware struct {
	jwt *service.JWT
}

// NewMiddleware wires a Middleware against the JWT verifier. Wire calls this
// once and shares the instance across every slice's *Router.
func NewMiddleware(j *service.JWT) *Middleware { return &Middleware{jwt: j} }

// ParseToken returns the middleware that decodes the auth cookie (if any) into
// a userID stored in the request context. Mounted globally on the chi router.
func (m *Middleware) ParseToken() func(http.Handler) http.Handler {
	return ParseTokenMiddleware(m.jwt)
}

// RequireUser returns the gate that rejects requests without an authenticated
// user with 401 unauthorized. Mounted on routes that mutate user-owned state.
func (m *Middleware) RequireUser() func(http.Handler) http.Handler {
	return requireUserHandlerWrap
}

// ParseTokenMiddleware is the package-level handler-wrapper for code paths that
// build a chi router without going through *Middleware (e.g., legacy tests
// that pass JWT directly).
func ParseTokenMiddleware(j *service.JWT) func(http.Handler) http.Handler {
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

// requireUserHandlerWrap is the underlying gate used by both the struct method
// and the package-level RequireUser. Kept as a func(http.Handler)http.Handler
// rather than a single-arg http.Handler wrapper so it composes cleanly with
// chi.Router.Use.
func requireUserHandlerWrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := UserIDFrom(r.Context()); !ok {
			jsonError(w, `{"error":"unauthorized"}`, 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireUser is the package-level gate kept for callers that haven't switched
// to *Middleware. Functionally identical to (*Middleware).RequireUser().
func RequireUser(next http.Handler) http.Handler {
	return requireUserHandlerWrap(next)
}

// UserIDFrom reads the authenticated user id off the request context. Returns
// (id, true) on a populated value; ("", false) when ParseToken found no valid
// cookie or the route was hit anonymously.
func UserIDFrom(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(userIDKey).(string)
	return v, ok && v != ""
}
