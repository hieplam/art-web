// Package artwork is the slice's Wire entry point. Logic lives in adapters/,
// ports/, service/, domain/.
package artwork

import (
	"github.com/google/wire"

	httpadapter "local/art-web/api/internal/artwork/adapters/http"
	logadapter "local/art-web/api/internal/artwork/adapters/log"
	"local/art-web/api/internal/artwork/adapters/postgres"
	"local/art-web/api/internal/artwork/ports"
	"local/art-web/api/internal/artwork/service"
)

// ProviderSet bundles the artwork slice's constructors + interface bindings.
var ProviderSet = wire.NewSet(
	postgres.NewRepo,
	postgres.NewTagsRepo,
	logadapter.NewZerologReporter,
	service.NewVisibilityService,
	httpadapter.NewHandler,
	httpadapter.NewRouter,
	wire.Bind(new(ports.ArtworkRepository), new(*postgres.Repo)),
	wire.Bind(new(ports.RollbackReporter), new(*logadapter.ZerologReporter)),
)
