.PHONY: help tidy build test test-race lint test-contract test-cover test-cover-slices test-cover-html test-all json-field-order

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
	@echo ""
	@echo "Note: every test target needs Docker running (testcontainers Postgres + MinIO)."
	@echo "First run boots containers cold (~10-30s); subsequent runs reuse via sync.Once."

tidy:               ; go mod tidy
build:              ; go build -o bin/api ./cmd/api
test:               ; go test -coverprofile=/tmp/coverage.out ./...  && go tool cover --func=/tmp/coverage.out
test-race:          ; go test -race ./...
lint:               ; go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...

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
	go run honnef.co/go/tools/cmd/staticcheck@latest ./...
	go test -race -coverprofile=/tmp/coverage.out ./...
	@echo "---"
	go tool cover -func=/tmp/coverage.out | tail -1

# AST gate: every struct in every dto.go must have fields in alphabetical
# JSON-tag order. Required because encoding/json marshals map keys
# alphabetically; struct field order must match for byte-identical output.
json-field-order:
	go run ./internal/infrastructure/server/cmd/json_field_order

.PHONY: wire wire-verify mocks

# Regenerate cmd/api/wire_gen.go from wire.go.
wire:
	cd cmd/api && go run github.com/google/wire/cmd/wire@latest ./...

# Fail if wire_gen.go is stale relative to wire.go (CI gate per spec §6.6).
wire-verify:
	cd cmd/api && go run github.com/google/wire/cmd/wire@latest ./...
	git diff --exit-code cmd/api/wire_gen.go

# Regenerate ports/<slice>/mocks via mockery v3.
# NOTE: .mockery.yaml lands in Task 7; running this target before then
# fails immediately with a config-not-found error from mockery.
mocks:
	go run github.com/vektra/mockery/v3@latest --config .mockery.yaml
