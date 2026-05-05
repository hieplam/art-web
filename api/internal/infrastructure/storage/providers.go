// api/internal/infrastructure/storage/providers.go
package storage

import (
	"github.com/google/wire"

	infraconfig "local/art-web/api/internal/infrastructure/config"
)

// ProviderSet currently exposes only the LocalFS-backed storage so the Wire
// graph compiles. The R2 branch is manually wired in main.go for now; a
// follow-up commit will introduce an env-driven storage selector here.
var ProviderSet = wire.NewSet(
	NewLocalFSFromConfig,
)

// NewLocalFSFromConfig adapts the typed config into a Storage. Returns the
// Storage interface so Wire treats it as a bound implementation directly.
func NewLocalFSFromConfig(cfg infraconfig.StorageConfig) Storage {
	root := cfg.Root
	if root == "" {
		root = "./var/storage"
	}
	return NewLocalFS(root)
}
