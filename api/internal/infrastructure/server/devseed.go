package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"gorm.io/gorm"

	authservice "local/art-web/api/internal/auth/service"
	infrastorage "local/art-web/api/internal/infrastructure/storage"
	"local/art-web/api/internal/seeder"
)

// DevSeed is a thin HTTP shim around seeder.Run, mounted at POST /dev/seed
// only when AppEnv == "test". Production wiring passes nil and the route
// stays unregistered.
//
// Why this exists after Task 10 dropped the original 600-line handler:
// cross-language test consumers (web/e2e in TypeScript, dev-up.sh in bash)
// can't import the seeder package directly. The architectural win of Task 10
// — one canonical seeding implementation in internal/seeder, callable from
// the cmd/seeder binary AND from in-process Go tests — is preserved; this
// route is a thin delegating wrapper that exists purely so non-Go callers
// keep working without a docker-exec dance.
//
// Go tests should still call seeder.Run directly via BootApp's *Booted
// bundle (see internal/httpapi/contract/matrix_test.go). The shim is only
// for cross-language paths.
type DevSeed struct {
	DB    *gorm.DB
	Store infrastorage.Storage
	JWT   *authservice.JWT
}

// Handle parses ?suffix=&many= from the query string, calls seeder.Run, and
// returns the JSON envelope. Failure paths emit 500 with a seed_failed code
// to match the legacy handler's shape.
func (h *DevSeed) Handle(w http.ResponseWriter, r *http.Request) {
	suffix := r.URL.Query().Get("suffix")
	many, _ := strconv.Atoi(r.URL.Query().Get("many"))
	if many < 0 {
		many = 0
	}

	out, err := seeder.Run(r.Context(), h.DB, h.Store, h.JWT, suffix, many)
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "seed_failed",
			"message": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(out)
}
