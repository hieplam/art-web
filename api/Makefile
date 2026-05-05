.PHONY: help tidy build test test-race lint test-contract test-cover test-cover-slices test-cover-html test-all

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

# Per-slice coverage on the four business slices. Phase 0 spec §8.3 hard gate
# requires each ≥ 80% statement coverage before Phase 1 starts.
test-cover-slices:
	@for pkg in auth user artwork image; do \
	  echo "=== $$pkg ===" ; \
	  go test -coverprofile=/tmp/cov-$$pkg.out ./internal/$$pkg/... ; \
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
