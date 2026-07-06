IMAGE      ?= ghcr.io/mohamediag/crossplane-console
TAG        ?= $(shell git rev-parse --short HEAD)
# Point at your cluster's kubeconfig, e.g.
#   make dev KUBECONFIG=/path/to/kubeconfig-local.yml
# Left unset, the binary falls back to in-cluster config, then ~/.kube/config.
KUBECONFIG ?=
# Cluster nodes are amd64 (Hetzner/KVM VMs); dev machines are often arm64
# (Apple Silicon). Native `docker build` produces a host-arch binary that
# fails at runtime on the cluster with "exec format error", so cross-build
# via buildx instead of the plain docker builder.
PLATFORM   ?= linux/amd64

.PHONY: dev
dev: ## Run backend against $(KUBECONFIG) (see note above); run 'npm run dev' in web/ separately
	go run ./cmd/console --kubeconfig "$(KUBECONFIG)" --log-level debug

.PHONY: test
test: ## Backend + frontend tests
	go test ./...
	go vet ./...
	cd web && npm test

.PHONY: build
build: ## Production binary with embedded UI (requires web/dist)
	cd web && npm run build
	go build -tags embedui -o bin/console ./cmd/console

.PHONY: docker-build
docker-build: ## Cross-build for $(PLATFORM) and load into the local docker daemon
	docker buildx build --platform $(PLATFORM) --build-arg VERSION=$(TAG) -t $(IMAGE):$(TAG) --load .

.PHONY: docker-push
docker-push: ## Cross-build for $(PLATFORM) and push directly (no local --load needed)
	docker buildx build --platform $(PLATFORM) --build-arg VERSION=$(TAG) -t $(IMAGE):$(TAG) --push .
	@echo "Pushed $(IMAGE):$(TAG) ($(PLATFORM)) — pin this tag in gitops/crossplane-console/deployment.yaml"

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-14s\033[0m %s\n", $$1, $$2}'
