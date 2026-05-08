// Package auth is the slice's Wire entry point. The actual auth logic lives in
// adapters/, ports/, service/, and domain/; this file just declares the
// ProviderSet that cmd/api/wire.go pulls in.
package auth

import (
	"github.com/google/wire"

	httpadapter "local/art-web/api/internal/auth/adapters/http"
	oauthadapter "local/art-web/api/internal/auth/adapters/oauth"
	"local/art-web/api/internal/auth/ports"
	"local/art-web/api/internal/auth/service"
)

// ProviderSet bundles the auth slice's constructors + interface bindings so
// cmd/api/wire.go can pull them in with a single line.
var ProviderSet = wire.NewSet(
	service.NewJWT,
	httpadapter.NewMiddleware,
	httpadapter.NewHandler,
	httpadapter.NewRouter,
	oauthadapter.NewGoogleProvider,
	wire.Bind(new(ports.JWTIssuer), new(*service.JWT)),
)
