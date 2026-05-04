.PHONY: tidy build test test-race lint test-contract test-cover test-cover-slices

tidy:               ; go mod tidy
build:              ; go build -o bin/api ./cmd/api
test:               ; go test -coverprofile=/tmp/coverage.out ./...  && go tool cover --func=/tmp/coverage.out
test-race:          ; go test -race ./...
lint:               ; go vet ./... && go run honnef.co/go/tools/cmd/staticcheck@latest ./...

# Phase 0 contract suite. Runs only the contract package; no goldens until PR 0.8.
test-contract:      ; go test ./internal/httpapi/contract/... -v

# Phase 0 coverage on the four business slices (gate: ≥ 80% statement coverage each).
test-cover-slices:
	@for pkg in auth user artwork image; do \
	  echo "=== $$pkg ===" ; \
	  go test -coverprofile=/tmp/cov-$$pkg.out ./internal/$$pkg/... ; \
	  go tool cover -func=/tmp/cov-$$pkg.out | tail -1 ; \
	done

test-cover:
	go test -coverprofile=/tmp/coverage.out ./...
	go tool cover -func=/tmp/coverage.out | tail -20
