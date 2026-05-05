//go:build tools

// Package tools anchors Phase 1 dependencies in go.mod so `go mod tidy`
// does not strip them before later tasks import them in production code.
// The build tag excludes this file from production builds.
package tools

import (
	_ "github.com/go-playground/validator/v10"
	_ "github.com/google/wire"
	_ "github.com/rs/zerolog"
	_ "gorm.io/driver/postgres"
	_ "gorm.io/gorm"
)
