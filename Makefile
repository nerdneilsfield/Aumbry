projectname := aumbry
pkg := github.com/nerdneilsfield/aumbry
version := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
commit := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
date := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags := -X $(pkg)/internal/buildinfo.Version=$(version) -X $(pkg)/internal/buildinfo.Commit=$(commit) -X $(pkg)/internal/buildinfo.Date=$(date)

.DEFAULT_GOAL := help
.PHONY: help setup test-web frontend build build-server build-cli install run test clean fmt fmt-check vet lint check hooks release-check release-test skill-refs skill-check

help: ## list makefile targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-24s\033[0m %s\n", $$1, $$2}'

setup: ## install frontend dependencies
	cd web && npm ci

test-web: ## run frontend unit tests
	cd web && npm test

frontend: ## build the Vue frontend for embedding
	cd web && npm run build
	mkdir -p internal/webembed/dist
	cp -R web/dist/. internal/webembed/dist/

build: frontend ## build the server and CLI binaries
	CGO_ENABLED=0 go build -trimpath -ldflags '$(ldflags)' -o $(projectname) .
	CGO_ENABLED=0 go build -trimpath -ldflags '$(ldflags)' -o $(projectname)-server ./cmd/server

build-server: frontend ## build the server binary
	CGO_ENABLED=0 go build -trimpath -ldflags '$(ldflags)' -o $(projectname)-server ./cmd/server

build-cli: ## build the CLI binary
	CGO_ENABLED=0 go build -trimpath -ldflags '$(ldflags)' -o $(projectname) .

install: ## install the CLI
	go install -ldflags '$(ldflags)' .

run: ## run the server using .env
	set -a; . ./.env; set +a; go run ./cmd/server

test: ## run Go tests
	CGO_ENABLED=0 go test -parallel=1 ./...

clean: ## remove generated build output
	rm -rf coverage.out dist/ bin/ aumbry aumbry.exe aumbry-server aumbry-server.exe web/dist/ internal/webembed/dist/
	rm -f web/tsconfig.app.tsbuildinfo web/tsconfig.node.tsbuildinfo web/vite.config.js web/vite.config.d.ts
	mkdir -p internal/webembed/dist
	touch internal/webembed/dist/.gitkeep

fmt: ## format Go files
	go tool gofumpt -l -w .
	go tool goimports -w -local $(pkg) .
	gofmt -w .

fmt-check: ## check Go formatting
	@test -z "$$(go tool gofumpt -l .)" || { echo "gofumpt: files need formatting"; exit 1; }
	@test -z "$$(go tool goimports -l -local $(pkg) .)" || { echo "goimports: files need formatting"; exit 1; }
	@test -z "$$(gofmt -l .)" || { echo "gofmt: files need formatting"; exit 1; }

vet: ## run go vet
	go vet ./...

lint: ## run golangci-lint
	golangci-lint run -c .golangci.yml

hooks: ## install Git hooks
	go tool lefthook install

release-check: ## validate GoReleaser config
	goreleaser check

release-test: setup frontend ## build a local snapshot release
	goreleaser release --snapshot --clean

skill-refs: ## regenerate skill XML tag reference from CLI schema
	CGO_ENABLED=0 go run . schema --format md > skills/aumbry/references/xml-tags.md

skill-check: ## check generated skill reference and validate all skill examples
	@tmpbin=$$(mktemp) && trap 'rm -f "$$tmpbin"' EXIT && \
	CGO_ENABLED=0 go build -o "$$tmpbin" . && \
	"$$tmpbin" schema --format md > skills/aumbry/references/xml-tags.md && \
	git diff --exit-code -- skills/aumbry/references/xml-tags.md && \
	for f in skills/aumbry/assets/examples/*.xml; do "$$tmpbin" validate "$$f" > /dev/null || exit 1; done

check: fmt-check lint vet test skill-check release-check ## run all checks
