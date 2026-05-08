// Package user is the slice's Wire entry point. The actual user logic lives in
// adapters/, ports/, and domain/; this file declares the ProviderSet.
package user

import (
	"github.com/google/wire"

	httpadapter "local/art-web/api/internal/user/adapters/http"
	"local/art-web/api/internal/user/adapters/postgres"
	"local/art-web/api/internal/user/ports"
)

// ProviderSet bundles the user slice's constructors + interface bindings.
var ProviderSet = wire.NewSet(
	postgres.NewRepo,
	httpadapter.NewHandler,
	httpadapter.NewRouter,
	wire.Bind(new(ports.UserRepository), new(*postgres.Repo)),
)
