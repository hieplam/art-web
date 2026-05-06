.PHONY: help tidy build test test-race lint test-contract test-cover test-cover-slices test-cover-html test-all json-field-order banned-imports cover-threshold

# Default target — `make` with no args lists every target.
help:
	@echo "Available targets:"
	@echo "  build              build the api binary into ./bin/api"
	@echo "  tidy               go mod tidy"
	@echo "  lint               go vet + staticcheck"
	@echo ""
	@echo "  test               full suite + per-function coverage summary"
	@echo "  test-race          full suite with -race detector"
	@echo "  test-cover         full suite, prints last 20 lines of per-function coverage"
	@echo "  test-cover-slices  per-slice coverage on auth/user/artwork/image (Phase 0 ≥80% gate)"
	@echo "  test-cover-html    full suite, opens HTML drill-down in browser"
	@echo "  test-contract      run only the byte-strict HTTP contract suite"
	@echo "  test-all           lint + test-race + coverage (full pre-merge check, slowest)"
	@echo "  json-field-order   AST check: dto.go structs must have alphabetical JSON-tag field order"
	@echo "  banned-imports     AST check: domain has no internal imports; ports no framework; service no adapter"
	@echo "  cover-threshold    fail if any slice (auth/user/artwork/image) drops below 80% coverage"
	@echo ""
	@echo "Note: every test target needs Docker running (testcontainers Postgres + MinIO)."
	@echo "First run boots containers cold (~10-30s); subsequent runs reuse via sync.Once."

STATICCHECK_VER = v0.6.1

tidy:               ; go mod tidy
build:              ; go build -o bin/api ./cmd/api
test:               ; go test -coverprofile=/tmp/coverage.out ./...  && go tool cover --func=/tmp/coverage.out
test-race:          ; go test -race ./...
lint:               ; go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VER) ./...

# Byte-strict HTTP contract suite. Replays 56 captured goldens from PR 0.8.
# Failures here mean Phase 1 changed an observable HTTP byte — see
# internal/httpapi/contract/error_codes_observed.md for the locked codes.
test-contract:      ; go test ./internal/httpapi/contract/... -v

# Per-slice coverage on the four business slices. Spec §10.1 merge gate
# requires each ≥ 80% statement coverage. The -coverpkg flag instruments
# every package in the slice tree so cross-sub-package execution (e.g., a
# test in adapters/http/ exercising adapters/oauth/) is attributed to the
# called package — required after Task 2's hexagonal split.
test-cover-slices:
	@for pkg in auth user artwork image; do \
	  echo "=== $$pkg ===" ; \
	  go test -coverpkg=./internal/$$pkg/... -coverprofile=/tmp/cov-$$pkg.out ./internal/$$pkg/... ; \
	  go tool cover -func=/tmp/cov-$$pkg.out | tail -1 ; \
	done

# Whole-module coverage; prints last 20 lines (total + most-uncovered functions).
test-cover:
	go test -coverprofile=/tmp/coverage.out ./...
	go tool cover -func=/tmp/coverage.out | tail -20

# HTML per-line drill-down. Opens browser; click a package to see red/green
# annotated source. Best for finding specific uncovered branches.
test-cover-html:
	go test -coverprofile=/tmp/coverage.out ./...
	go tool cover -html=/tmp/coverage.out

# Comprehensive pre-merge gate: lint + race-detector tests + coverage. Slowest
# target (~90s warm). Run before opening a PR or merging to master.
test-all:
	go vet ./...
	go run honnef.co/go/tools/cmd/staticcheck@$(STATICCHECK_VER) ./...
	go test -race -coverprofile=/tmp/coverage.out ./...
	@echo "---"
	go tool cover -func=/tmp/coverage.out | tail -1

# AST gate: every struct in every dto.go must have fields in alphabetical
# JSON-tag order. Required because encoding/json marshals map keys
# alphabetically; struct field order must match for byte-identical output.
json-field-order:
	go run ./internal/infrastructure/server/cmd/json_field_order

# AST gate: enforces hexagonal-architecture import boundaries (spec §10.1).
#   domain  : no internal/* imports (pure)
#   ports   : no framework imports (gorm/chi/jwt/validator/etc.)
#   service : no slice-adapter imports (<slice>/adapters/...)
banned-imports:
	go run ./internal/infrastructure/server/cmd/banned_imports

# Coverage gate (spec §10.1): each business slice must hit ≥80% statement
# coverage. Reuses test-cover-slices' per-slice -coverpkg profile and parses
# go tool cover's "total: ... %" line for a numeric comparison.
#
# -count=1 is required: go test's coverage cache otherwise returns stale
# per-package profiles after edits to production files, which produced the
# spurious "artwork dropped from 82% to 64%" failures during Task 8/9 review.
cover-threshold:
	@threshold=80.0 ; \
	failed=0 ; \
	for pkg in auth user artwork image; do \
	  go test -count=1 -coverpkg=./internal/$$pkg/... -coverprofile=/tmp/cov-$$pkg.out ./internal/$$pkg/... >/dev/null || { echo "  $$pkg: test failed" >&2 ; failed=1 ; continue ; } ; \
	  pct=$$(go tool cover -func=/tmp/cov-$$pkg.out | awk '/^total:/{gsub("%","",$$3); print $$3}') ; \
	  if awk -v p="$$pct" -v t="$$threshold" 'BEGIN{ exit (p+0 < t+0) }' ; then \
	    echo "  $$pkg: $$pct% OK" ; \
	  else \
	    echo "  $$pkg: $$pct% < $$threshold% FAIL" >&2 ; \
	    failed=1 ; \
	  fi ; \
	done ; \
	exit $$failed

.PHONY: wire wire-verify

# Pinned tool versions: keep these in sync with go.mod's require block so CI
# is reproducible. The legacy targets used @latest, which silently broke wire
# when v0.8 shipped a backwards-incompatible change.
WIRE_VER = v0.7.0

# Regenerate cmd/api/wire_gen.go from wire.go.
wire:
	cd cmd/api && go run github.com/google/wire/cmd/wire@$(WIRE_VER) ./...

# Fail if wire_gen.go is stale relative to wire.go (CI gate per spec §6.6).
wire-verify:
	cd cmd/api && go run github.com/google/wire/cmd/wire@$(WIRE_VER) ./...
	git diff --exit-code cmd/api/wire_gen.go
