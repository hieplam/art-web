package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"local/art-web/api/internal/auth/ports"
	"local/art-web/api/internal/auth/service"
	userpostgres "local/art-web/api/internal/user/adapters/postgres"
)

// Handler holds the auth slice's per-request collaborators. The constructor
// receives the providers map plus the userRepo + JWT directly so /me and
// /auth/* can share one stateful struct rather than separate factories.
type Handler struct {
	providers    map[string]ports.OAuthProvider
	users        *userpostgres.Repo
	jwt          *service.JWT
	frontendHome string
	cookieOpts   CookieOpts
}

// AuthDeps is the wire-friendly parameter object that NewHandler takes. Wire's
// wire.Struct provider builds it field-wise from the constituent providers.
type AuthDeps struct {
	Providers    map[string]ports.OAuthProvider
	Users        *userpostgres.Repo
	JWT          *service.JWT
	FrontendHome FrontendHome
	CookieOpts   CookieOpts
}

// FrontendHome is a typed string so Wire can distinguish it from other
// stringly-typed deps without name collision.
type FrontendHome string

// NewHandler wires the auth slice's HTTP handler from a fully-populated
// AuthDeps.
func NewHandler(d AuthDeps) *Handler {
	return &Handler{
		providers:    d.Providers,
		users:        d.Users,
		jwt:          d.JWT,
		frontendHome: string(d.FrontendHome),
		cookieOpts:   d.CookieOpts,
	}
}

// Start handles GET /auth/{provider}/start.
func (h *Handler) Start(w http.ResponseWriter, r *http.Request) {
	StartHandler(h.providers, h.cookieOpts).ServeHTTP(w, r)
}

// Callback handles GET /auth/{provider}/callback.
func (h *Handler) Callback(w http.ResponseWriter, r *http.Request) {
	CallbackHandler(h.providers, h.users, h.jwt, h.frontendHome, h.cookieOpts).ServeHTTP(w, r)
}

// Logout handles POST /auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	LogoutHandler(h.cookieOpts).ServeHTTP(w, r)
}

// Me handles GET /me. Returns 401 with no Set-Cookie when there's no auth
// cookie, 401 with a clearing Set-Cookie when the JWT subject does not exist
// in the users table (recovers from a stale cookie after schema reseeding),
// and 200 with the user payload on success.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	uid, ok := UserIDFrom(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	u, err := h.users.Get(r.Context(), uid)
	if errors.Is(err, userpostgres.ErrNotFound) {
		ClearAuthCookie(w, h.cookieOpts)
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "user_lookup_failed"})
		return
	}
	writeJSON(w, http.StatusOK, renderUser(u))
}

// Router exposes the auth slice's routes via the RouteRegistrar interface.
// Composed via the slice's providers.go and consumed by
// infrastructure/server.ProvideRouteRegistrars.
type Router struct {
	handler *Handler
}

// NewRouter pairs a Handler with the RouteRegistrar contract.
func NewRouter(h *Handler) *Router { return &Router{handler: h} }

// RegisterRoutes mounts /auth/* and /me on the supplied chi.Router.
func (router *Router) RegisterRoutes(r chi.Router) {
	r.Route("/auth", func(r chi.Router) {
		r.Get("/{provider}/start", router.handler.Start)
		r.Get("/{provider}/callback", router.handler.Callback)
		r.Post("/logout", router.handler.Logout)
	})
	r.Get("/me", router.handler.Me)
}

// renderUser serializes a postgres.User (alias for userdomain.User) into the
// response shape the historical /me handler emitted. Duplicated from the
// legacy server respond.go so the auth slice doesn't depend on it.
func renderUser(u *userpostgres.User) map[string]any {
	var avatar *string
	if u.AvatarURL != "" {
		v := u.AvatarURL
		avatar = &v
	}
	return map[string]any{
		"id":           u.ID,
		"display_name": u.DisplayName,
		"slug":         u.Slug,
		"avatar_url":   avatar,
	}
}

// writeJSON encodes body as JSON with the standard content type header.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
