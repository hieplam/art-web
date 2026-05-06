// cmd/banned_imports enforces the layered hexagonal-architecture import rules
// per spec §10.1:
//
//	domain/  : no imports of local/art-web/api/internal/* (must be pure;
//	           domain may only depend on stdlib + external value-types).
//	ports/   : no framework imports (gorm.io, chi, jwt, validator, etc.).
//	           Same-slice domain imports are fine; cross-slice ports/domain
//	           imports are also allowed.
//	service/ : no slice-adapter imports (anything matching
//	           local/art-web/api/internal/*/adapters/...). Cross-cutting
//	           infrastructure packages (config, storage, logger, httperr)
//	           are allowed because they are framework-shaped ports defined
//	           at the infrastructure boundary, not slice adapters.
//
// Only production files are checked; *_test.go files are ignored because
// integration tests routinely import adapters to compose realistic seams.
//
// Usage:
//
//	go run ./internal/infrastructure/server/cmd/banned_imports [root]
//
// Defaults to scanning ./internal. Exit code 1 means at least one violation.
package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

const (
	internalPrefix = "local/art-web/api/internal/"
)

// frameworkPrefixes lists import-path roots that ports MAY NOT import. These
// are concrete-framework dependencies that belong in adapters or service.
// The list is conservative — add new entries only when a real violation lands.
var frameworkPrefixes = []string{
	"gorm.io/",
	"github.com/go-chi/",
	"github.com/jackc/pgx",
	"github.com/golang-jwt/",
	"github.com/go-playground/validator",
	"github.com/buckket/go-blurhash",
	"github.com/google/uuid",
	"github.com/aws/aws-sdk-go",
	"github.com/minio/minio-go",
	"github.com/rs/zerolog",
	"net/http",
}

type violation struct {
	file   string
	layer  string
	imp    string
	reason string
}

func main() {
	root := "./internal"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	var violations []violation
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		layer, ok := classify(path)
		if !ok {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, im := range file.Imports {
			imp := strings.Trim(im.Path.Value, `"`)
			if r := check(layer, imp); r != "" {
				violations = append(violations, violation{file: path, layer: layer, imp: imp, reason: r})
			}
		}
		return nil
	})
	for _, v := range violations {
		fmt.Fprintf(os.Stderr, "%s [%s]: forbidden import %q — %s\n", v.file, v.layer, v.imp, v.reason)
	}
	if len(violations) > 0 {
		os.Exit(1)
	}
}

// classify returns the layer name (domain/ports/service) for a path under a
// slice tree, or ("", false) if the path is not in any layer we police.
func classify(path string) (string, bool) {
	clean := filepath.ToSlash(path)
	switch {
	case strings.Contains(clean, "/domain/") || strings.HasSuffix(clean, "/domain.go"):
		return "domain", true
	case strings.Contains(clean, "/ports/"):
		return "ports", true
	case strings.Contains(clean, "/service/"):
		return "service", true
	default:
		return "", false
	}
}

// check returns a non-empty reason string when the import violates the layer's
// rule. An empty return means the import is allowed.
func check(layer, imp string) string {
	switch layer {
	case "domain":
		if strings.HasPrefix(imp, internalPrefix) {
			return "domain must be pure (no local/art-web/api/internal/* imports)"
		}
	case "ports":
		for _, fw := range frameworkPrefixes {
			if strings.HasPrefix(imp, fw) {
				return "ports must be framework-free (matches " + fw + ")"
			}
		}
	case "service":
		if rest, ok := strings.CutPrefix(imp, internalPrefix); ok {
			parts := strings.Split(rest, "/")
			if len(parts) >= 2 && parts[1] == "adapters" {
				return "service must not import slice adapters (" + parts[0] + "/adapters/...)"
			}
		}
	}
	return ""
}
