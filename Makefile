.PHONY: check test test-int test-golden build images dev deploy-gen api-gen fmt lint lint-logs cover web test-web test-e2e preflight backup bundle security
PODMAN ?= podman
GO ?= go

check: lint test web
security:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./cmd/... ./internal/...
	npm audit --prefix web --audit-level=moderate
test:
	$(GO) test -race -count=1 ./...
test-int:
	python scripts/integration.py
test-golden: test-int
cover:
	$(GO) test -race -count=1 -coverprofile=coverage.out ./internal/...
fmt:
	gofmt -w cmd internal
lint:
	$(GO) vet ./...
	python scripts/check.py
lint-logs:
	python scripts/check.py --logs
web:
	npm run build --prefix web
test-web:
	npm run test --prefix web
test-e2e:
	npm run test:e2e --prefix web
build:
	$(GO) build -trimpath -o dist/audit ./cmd/audit
images:
	$(PODMAN) build -t localhost/autodit:0.1.0 -f Containerfile .
	$(PODMAN) build -t localhost/autodit-proxy:0.1.0 -f Containerfile.proxy .
dev:
	./scripts/install.sh --no-build
preflight:
	./scripts/install.sh --preflight
deploy-gen:
	python scripts/deploy-gen.py
api-gen:
	python scripts/generate-api.py
	npm run api-gen --prefix web
backup:
	./scripts/backup.sh
bundle:
	python scripts/bundle.py
