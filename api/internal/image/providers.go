// Package image is the slice's Wire entry point.
package image

import (
	"github.com/google/wire"

	httpadapter "local/art-web/api/internal/image/adapters/http"
	"local/art-web/api/internal/image/adapters/postgres"
	"local/art-web/api/internal/image/ports"
	"local/art-web/api/internal/image/service"
)

// ProviderSet bundles the image slice's constructors + interface bindings.
var ProviderSet = wire.NewSet(
	postgres.NewRepo,
	service.NewService,
	httpadapter.NewHandler,
	httpadapter.NewRouter,
	wire.Bind(new(ports.ImageRepository), new(*postgres.Repo)),
)
