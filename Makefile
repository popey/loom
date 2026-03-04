.PHONY: all build build-all start stop restart run prune bootstrap status \
       test test-docker test-api coverage test-coverage fmt vet \
       lint lint-go lint-js lint-yaml lint-docs lint-api \
       deps clean distclean install config help \
       release release-patch release-minor release-major _release-preflight \
       agents scale-coders scale-reviewers scale-qa scale-agents logs-agents stop-agents \
       k8s-apply k8s-delete linkerd-setup linkerd-check linkerd-dashboard linkerd-tap \
       proto-gen docs docs-serve docs-deploy \
       helm-install helm-upgrade helm-uninstall helm-deps helm-lint helm-template helm-package helm-secrets \
       builder

# ──────────────────────────────────────────────────────────────────────────────
# Prerequisites: Docker, Make.  All Go operations run inside containers.
# Only targets that genuinely need host access (install, bootstrap, release,
# k8s/helm, docs, test-api) touch the host.
# ──────────────────────────────────────────────────────────────────────────────

# Build variables
BINARY_NAME := loom
BIN_DIR     := bin
OBJ_DIR     := obj
VERSION     ?= dev
LDFLAGS     := -ldflags "-X main.version=$(VERSION)"
INSTALL_DIR ?= $(HOME)/.local/bin

# Builder image — lightweight Go + build tools, no source baked in.
BUILDER_IMAGE  := loom-builder:dev
GO_MOD_CACHE   := loom-go-mod
GO_BUILD_CACHE := loom-go-build

# --load ensures the image is available to the local docker daemon.
DOCKER_BUILD := $(shell docker buildx version >/dev/null 2>&1 && echo "docker buildx build --load" || echo "docker build")

DOCKER_RUN := docker run --rm \
	-v $(CURDIR):/build \
	-v $(GO_MOD_CACHE):/go/pkg \
	-v $(GO_BUILD_CACHE):/root/.cache/go-build \
	-w /build \
	$(BUILDER_IMAGE)

# TokenHub auto-detection for start/restart/run.
TOKENHUB_RUNNING := $(shell curl -sf --connect-timeout 2 --max-time 3 http://localhost:8090/healthz > /dev/null 2>&1 && echo yes || echo no)
EXTERNAL_PROVIDER_URL := http://host.docker.internal:8090/v1

all: build

# ──── Builder image (cached) ────────────────────────────────────────────────

builder:
	@if docker image inspect $(BUILDER_IMAGE) >/dev/null 2>&1; then \
		true; \
	else \
		echo "Building builder image (first time)..."; \
		$(DOCKER_BUILD) -t $(BUILDER_IMAGE) -f Dockerfile.dev .; \
	fi

# ──── Build ─────────────────────────────────────────────────────────────────

build: builder
	@mkdir -p $(BIN_DIR)
	@echo "Building loom server..."
	@$(DOCKER_RUN) go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/loom
	@echo "Building loomctl CLI..."
	@$(DOCKER_RUN) sh -c 'CGO_ENABLED=1 go build $(LDFLAGS) -o $(BIN_DIR)/loomctl ./cmd/loomctl'
	@echo "Building loom-project-agent..."
	@$(DOCKER_RUN) sh -c 'CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN_DIR)/loom-project-agent ./cmd/loom-project-agent'
	@echo "Building connectors-service..."
	@$(DOCKER_RUN) sh -c 'CGO_ENABLED=0 go build $(LDFLAGS) -o $(BIN_DIR)/connectors-service ./cmd/connectors-service'
	@echo "Build complete: bin/loom, bin/loomctl, bin/loom-project-agent, bin/connectors-service"

build-all: builder
	@mkdir -p $(BIN_DIR)
	@echo "Cross-compiling for multiple platforms..."
	@$(DOCKER_RUN) sh -c '\
		GOOS=linux  GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-linux-amd64  ./cmd/loom && \
		GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/loom && \
		GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/loom && \
		GOOS=linux  GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/loom-project-agent-linux-amd64 ./cmd/loom-project-agent && \
		GOOS=linux  GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/loom-project-agent-linux-arm64 ./cmd/loom-project-agent && \
		GOOS=linux  GOARCH=amd64 go build $(LDFLAGS) -o $(BIN_DIR)/connectors-service-linux-amd64 ./cmd/connectors-service && \
		GOOS=linux  GOARCH=arm64 go build $(LDFLAGS) -o $(BIN_DIR)/connectors-service-linux-arm64 ./cmd/connectors-service'
	@echo "Build complete: $(BIN_DIR)/*"

# ──── Service lifecycle ─────────────────────────────────────────────────────
# start/restart/run no longer depend on a native Go build — docker compose
# --build rebuilds the production image via the Dockerfile.

define COMPOSE_UP
	@if [ "$(TOKENHUB_RUNNING)" = "yes" ]; then \
		echo "Found running TokenHub at localhost:8090 — skipping embedded startup."; \
		echo "  Provider URL for containers: $(EXTERNAL_PROVIDER_URL)"; \
		LOOM_PROVIDER_URL="$(EXTERNAL_PROVIDER_URL)" docker compose up -d --build; \
	else \
		echo "No TokenHub detected — starting embedded instance."; \
		docker compose --profile embedded-tokenhub up -d --build; \
	fi
endef

run:
	$(COMPOSE_UP)
	@$(MAKE) -s bootstrap
	@echo ""
	@echo "Loom is running:"
	@echo "  UI:    http://localhost:8081"
	@echo "  API:   http://localhost:8081/api/v1/system/state"
	@echo "  Logs:  make logs"
	@echo "  Stop:  make stop"

start:
	$(COMPOSE_UP)
	@$(MAKE) -s bootstrap

stop:
	docker compose --profile embedded-tokenhub down --remove-orphans

restart:
	docker compose --profile embedded-tokenhub down
	$(COMPOSE_UP)
	@$(MAKE) -s bootstrap

bootstrap:
	@if [ -f bootstrap.local ]; then \
		echo "Waiting for loom to be healthy..."; \
		for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do \
			status=$$(curl -s --connect-timeout 2 --max-time 5 http://localhost:8081/health 2>/dev/null | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4); \
			if [ "$$status" = "healthy" ]; then \
				echo "Loom is healthy, running bootstrap.local..."; \
				chmod +x bootstrap.local && ./bootstrap.local; \
				exit 0; \
			fi; \
			echo "  Attempt $$i/20: waiting (status=$$status)..."; \
			sleep 5; \
		done; \
		echo "WARNING: Loom did not become healthy in 100s, skipping bootstrap.local"; \
		echo "         Run 'make bootstrap' manually once loom is ready."; \
	else \
		echo "No bootstrap.local found (copy bootstrap.local.example to create one)"; \
	fi

prune:
	@echo "Removing stopped containers..."
	docker container prune -f
	@echo "Removing dangling images..."
	docker image prune -f
	@echo "Removing unused build cache..."
	docker builder prune -f
	@echo "Prune complete. Volumes (databases) preserved."

logs:
	docker compose logs -f loom

status:
	@docker compose ps -a --format '{{.Name}}\t{{.State}}\t{{.Status}}\t{{.Health}}' 2>/dev/null | \
	while IFS='	' read -r name state status health; do \
		[ -z "$$name" ] && continue; \
		label="$$status"; \
		[ -n "$$health" ] && label="$$health"; \
		if [ "$$state" = "running" ]; then sym="✔"; else sym="✗"; fi; \
		printf " %s %-35s %s\n" "$$sym" "$$name" "$$label"; \
	done || echo " ✗ No containers found. Run 'make start' to launch loom."
	@echo ""
	@health=$$(curl -s --connect-timeout 2 --max-time 5 http://localhost:8081/health 2>/dev/null); \
	if [ -n "$$health" ]; then \
		status=$$(echo "$$health" | python3 -c "import sys,json; print(json.load(sys.stdin).get('status','unknown'))" 2>/dev/null || echo "unknown"); \
		if [ "$$status" = "healthy" ]; then \
			echo " ✔ Loom API                         healthy"; \
		else \
			echo " ✗ Loom API                         $$status"; \
		fi; \
	else \
		echo " ✗ Loom API                         not responding"; \
	fi

# ──── Testing ───────────────────────────────────────────────────────────────

test: builder
	@echo "=== Running tests ==="
	@$(DOCKER_RUN) go test ./... -count=1 -timeout 600s

test-docker:
	docker compose up -d --build temporal-postgresql temporal temporal-ui
	docker compose run --rm loom-test
	docker compose down

test-api:
	@echo "Running post-flight API tests..."
	@./test/postflight/api_test.sh $(BASE_URL)

coverage: builder
	@mkdir -p $(OBJ_DIR)
	@$(DOCKER_RUN) sh -c '\
		go test -coverprofile=$(OBJ_DIR)/coverage.out -covermode=atomic ./... && \
		go tool cover -html=$(OBJ_DIR)/coverage.out -o $(OBJ_DIR)/coverage.html'
	@echo "Coverage report: $(OBJ_DIR)/coverage.html"

test-coverage: builder
	@$(DOCKER_RUN) ./scripts/test-coverage.sh

# ──── Code quality ──────────────────────────────────────────────────────────

fmt: builder
	@$(DOCKER_RUN) go fmt ./...

vet: builder
	@$(DOCKER_RUN) go vet ./...

lint: lint-go lint-js lint-yaml lint-docs lint-api

lint-go: builder
	@echo "=== Go Linting ==="
	@$(DOCKER_RUN) sh -c '\
		if command -v golangci-lint >/dev/null 2>&1; then \
			golangci-lint run --timeout 5m; \
		else \
			echo "  (golangci-lint not in builder — running go fmt + go vet)"; \
			go fmt ./...; \
			go vet ./...; \
		fi'

lint-js:
	@echo ""
	@echo "=== JavaScript Linting ==="
	@if command -v eslint >/dev/null 2>&1; then \
		eslint web/static/js/*.js || true; \
	else \
		echo "  (eslint not found, skipping JS linting)"; \
	fi

lint-yaml: builder
	@echo ""
	@echo "=== YAML Linting ==="
	@$(DOCKER_RUN) go run ./cmd/yaml-lint

lint-docs:
	@echo ""
	@echo "=== Documentation Linting ==="
	@bash scripts/check-docs-structure.sh

lint-api:
	@echo ""
	@echo "=== API/Frontend Validation ==="
	@bash scripts/check-api-frontend.sh

# ──── Install ───────────────────────────────────────────────────────────────
# Builds natively on the host (requires Go) and installs to ~/.local/bin.
# This is the one target that legitimately needs Go on the host — the
# resulting binaries talk to the containerised services.

install:
	@mkdir -p $(BIN_DIR)
	go build $(LDFLAGS) -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/loom
	CGO_ENABLED=1 go build $(LDFLAGS) -o $(BIN_DIR)/loomctl ./cmd/loomctl
	@mkdir -p $(INSTALL_DIR)
	cp $(BIN_DIR)/$(BINARY_NAME) $(BIN_DIR)/loomctl $(INSTALL_DIR)/
	@echo "Installed loom + loomctl to $(INSTALL_DIR)/"
	@if echo $$PATH | grep -q "$$HOME/.local/bin"; then \
		echo "  ~/.local/bin is already in your PATH"; \
	else \
		echo "  Add to PATH: export PATH=\"\$$HOME/.local/bin:\$$PATH\""; \
	fi

# ──── Config ────────────────────────────────────────────────────────────────

config:
	@if [ ! -f config.yaml ]; then \
		cp config.yaml.example config.yaml; \
		echo "Created config.yaml from example"; \
	else \
		echo "config.yaml already exists"; \
	fi

# ──── Clean ─────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BIN_DIR)
	rm -rf $(OBJ_DIR)
	rm -rf site/

distclean: clean
	@docker compose --profile embedded-tokenhub down -v --remove-orphans 2>/dev/null || true
	@docker rmi loom:latest loom-loom-test:latest $(BUILDER_IMAGE) 2>/dev/null || true
	@docker volume rm $(GO_MOD_CACHE) $(GO_BUILD_CACHE) 2>/dev/null || true
	@docker image prune -f

# ──── Dependencies ──────────────────────────────────────────────────────────
# Everything runs in Docker — the only host prerequisites are Docker and Make.

deps:
	@ok=true; \
	if ! command -v docker >/dev/null 2>&1; then \
		echo "Docker is required. Install from https://docs.docker.com/get-docker/"; \
		ok=false; \
	fi; \
	if ! docker compose version >/dev/null 2>&1; then \
		echo "docker compose plugin is required. See https://docs.docker.com/compose/install/"; \
		ok=false; \
	fi; \
	if [ "$$ok" = true ]; then \
		echo "Prerequisites satisfied: docker, docker compose, make"; \
		echo "Run 'make start' to start loom."; \
	else \
		exit 1; \
	fi

# ──── Release ───────────────────────────────────────────────────────────────

release: _release-preflight
	@BATCH=$(BATCH) ./scripts/release.sh patch

release-patch: _release-preflight
	@BATCH=$(BATCH) ./scripts/release.sh patch

release-minor: _release-preflight
	@BATCH=$(BATCH) ./scripts/release.sh minor

release-major: _release-preflight
	@BATCH=$(BATCH) ./scripts/release.sh major

_release-preflight: lint-go lint-yaml build-all docs
	@echo ""
	@echo "=== Release preflight complete ==="
	@echo "  All binaries compiled for all platforms"
	@echo "  Linters passed"
	@echo "  Documentation built"
	@echo ""

# ──── Agent swarm ───────────────────────────────────────────────────────────

agents: start
	@echo "Scaling agent swarm..."
	docker compose up -d loom-agent-coder loom-agent-reviewer loom-agent-qa
	@echo "Agent swarm running."

scale-coders:
	docker compose up -d --scale loom-agent-coder=$(N) --no-recreate

scale-reviewers:
	docker compose up -d --scale loom-agent-reviewer=$(N) --no-recreate

scale-qa:
	docker compose up -d --scale loom-agent-qa=$(N) --no-recreate

scale-agents:
	docker compose up -d \
		--scale loom-agent-coder=$(CODERS) \
		--scale loom-agent-reviewer=$(REVIEWERS) \
		--scale loom-agent-qa=$(QA) \
		--no-recreate

logs-agents:
	docker compose logs -f loom-agent-coder loom-agent-reviewer loom-agent-qa

stop-agents:
	docker compose stop loom-agent-coder loom-agent-reviewer loom-agent-qa

# ──── Kubernetes / Linkerd ──────────────────────────────────────────────────

k8s-apply:
	kubectl apply -k deploy/k8s/overlays/local

k8s-delete:
	kubectl delete -k deploy/k8s/overlays/local --ignore-not-found

linkerd-setup:
	@./scripts/setup-linkerd.sh

linkerd-check:
	linkerd -n loom check --proxy

linkerd-dashboard:
	linkerd viz dashboard

linkerd-tap:
	linkerd -n loom tap deploy/loom

proto-gen:
	@PROTOC=$$(which protoc || echo /tmp/protoc-arm/bin/protoc); \
	export PATH=$$(go env GOPATH)/bin:$$PATH; \
	$$PROTOC \
	  --proto_path=api/proto/connectors \
	  --go_out=api/proto/connectors \
	  --go_opt=paths=source_relative \
	  --go-grpc_out=api/proto/connectors \
	  --go-grpc_opt=paths=source_relative \
	  api/proto/connectors/connectors.proto
	@echo "Proto generation complete."

# ──── Helm ──────────────────────────────────────────────────────────────────

HELM_RELEASE   ?= loom
HELM_NAMESPACE ?= loom
HELM_CHART      = deploy/helm/loom
HELM_VALUES    ?= $(HELM_CHART)/values.yaml

helm-deps:
	helm dependency update $(HELM_CHART)

helm-install: helm-deps
	helm install $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) --create-namespace --values $(HELM_VALUES)

helm-upgrade: helm-deps
	helm upgrade $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) --create-namespace --install --values $(HELM_VALUES)

helm-uninstall:
	helm uninstall $(HELM_RELEASE) --namespace $(HELM_NAMESPACE)

helm-lint:
	helm lint $(HELM_CHART) --values $(HELM_VALUES)

helm-template:
	helm template $(HELM_RELEASE) $(HELM_CHART) \
		--namespace $(HELM_NAMESPACE) --values $(HELM_VALUES)

helm-package: helm-deps
	@mkdir -p $(BIN_DIR)
	helm package $(HELM_CHART) --destination $(BIN_DIR)

helm-secrets:
	@echo "Creating Loom secrets in namespace $(HELM_NAMESPACE)..."
	@kubectl create namespace $(HELM_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@kubectl create secret generic loom-postgresql-secret \
		--namespace $(HELM_NAMESPACE) \
		--from-literal=username=loom \
		--from-literal=password=$$(openssl rand -base64 24) \
		--dry-run=client -o yaml | kubectl apply -f -
	@kubectl create secret generic loom-secret \
		--namespace $(HELM_NAMESPACE) \
		--from-literal=password=$$(openssl rand -base64 24) \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "Secrets created. Run 'make helm-install' to deploy."

# ──── Documentation ─────────────────────────────────────────────────────────

docs:
	@echo "=== Building Documentation ==="
	@if command -v mkdocs >/dev/null 2>&1; then \
		mkdocs build; \
	else \
		echo "  (mkdocs not installed — pip install mkdocs-material to enable)"; \
	fi

docs-serve:
	@pip install --quiet mkdocs-material 2>/dev/null || pip install --quiet --user mkdocs-material
	mkdocs serve

docs-deploy:
	@pip install --quiet mkdocs-material 2>/dev/null || pip install --quiet --user mkdocs-material
	mkdocs gh-deploy --force

# ──── Help ──────────────────────────────────────────────────────────────────

help:
	@echo "Loom - Makefile Commands"
	@echo ""
	@echo "Prerequisites: Docker, Make (all Go builds run inside containers)"
	@echo ""
	@echo "Service (Docker Compose):"
	@echo "  make run          - Build and launch full stack + bootstrap"
	@echo "  make start        - Start in background + bootstrap"
	@echo "  make stop         - Stop all containers"
	@echo "  make restart      - Rebuild and restart + bootstrap"
	@echo "  make prune        - Remove stale Docker images (preserves volumes)"
	@echo "  make bootstrap    - Run bootstrap.local (registers providers)"
	@echo "  make status       - Show container and loom health status"
	@echo "  make logs         - Follow loom container logs"
	@echo ""
	@echo "Development (all in Docker):"
	@echo "  make build        - Build all 4 Go binaries"
	@echo "  make build-all    - Cross-compile for linux/darwin"
	@echo "  make test         - Run test suite"
	@echo "  make test-docker  - Run tests with Temporal"
	@echo "  make test-api     - Run post-flight API tests"
	@echo "  make coverage     - Run tests with HTML coverage report"
	@echo "  make lint         - Run all linters (Go, JS, YAML, docs, API)"
	@echo "  make fmt          - Format Go code"
	@echo "  make vet          - Run go vet"
	@echo "  make clean        - Remove build artifacts"
	@echo "  make distclean    - Deep clean (containers + images + caches)"
	@echo ""
	@echo "Host (requires Go):"
	@echo "  make install      - Install loom + loomctl to ~/.local/bin"
	@echo ""
	@echo "Setup:"
	@echo "  make deps         - Verify host prerequisites (Docker, Make)"
	@echo "  make config       - Create config.yaml from example"
	@echo ""
	@echo "Agent Swarm:"
	@echo "  make agents               - Start all agent containers"
	@echo "  make scale-coders N=3     - Scale coder agents"
	@echo "  make scale-reviewers N=2  - Scale reviewer agents"
	@echo "  make scale-qa N=2         - Scale QA agents"
	@echo "  make logs-agents          - Follow agent logs"
	@echo "  make stop-agents          - Stop agent containers only"
	@echo ""
	@echo "Documentation:"
	@echo "  make docs         - Build docs (mkdocs)"
	@echo "  make docs-serve   - Serve docs locally"
	@echo "  make docs-deploy  - Deploy to GitHub Pages"
	@echo ""
	@echo "Helm / Kubernetes:"
	@echo "  make helm-install / helm-upgrade / helm-uninstall"
	@echo "  make helm-lint / helm-template / helm-package / helm-secrets"
	@echo "  make k8s-apply / k8s-delete"
	@echo "  make linkerd-setup / linkerd-check / linkerd-dashboard"
	@echo ""
	@echo "Release:"
	@echo "  make release       - Patch release (x.y.Z)"
	@echo "  make release-minor - Minor release (x.Y.0)"
	@echo "  make release-major - Major release (X.0.0)"
