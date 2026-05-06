// cmd/json_field_order checks that every struct in every dto.go file has its
// fields declared in alphabetical order by JSON tag. This enforces byte-for-byte
// output compatibility when switching from inline map[string]any to named structs
// (Go's encoding/json marshals map keys alphabetically; struct field order must
// match to preserve the same byte output).
//
// Usage:
//
//	go run ./internal/infrastructure/server/cmd/json_field_order [root]
//
// root defaults to "./internal". Exit code 1 means at least one violation.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	root := "./internal"
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	violations := 0
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !strings.HasSuffix(path, "/dto.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, sp := range gd.Specs {
				ts, ok := sp.(*ast.TypeSpec)
				if !ok {
					continue
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}
				var tags []string
				for _, f := range st.Fields.List {
					if f.Tag == nil {
						continue
					}
					tag := strings.Trim(f.Tag.Value, "`")
					if i := strings.Index(tag, `json:"`); i >= 0 {
						v := tag[i+6:]
						if j := strings.IndexAny(v, `,"`); j >= 0 {
							tags = append(tags, v[:j])
						}
					}
				}
				if !sort.StringsAreSorted(tags) {
					println(path+": "+ts.Name.Name+" fields not in alphabetical JSON-tag order:", strings.Join(tags, ", "))
					violations++
				}
			}
		}
		return nil
	})
	if violations > 0 {
		os.Exit(1)
	}
}
