# Audicia Operator Makefile

# Build variables
BINARY_NAME := audicia
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

# Container image
IMG ?= ghcr.io/audicia/audicia-operator:$(VERSION)

# Tools
CONTROLLER_GEN ?= $(shell which controller-gen 2>/dev/null)
GOLANGCI_LINT ?= $(shell which golangci-lint 2>/dev/null)

# Go
GOFLAGS ?=
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: test
test: ## Run unit tests.
	go test -race -coverprofile=coverage.txt -covermode=atomic ./...

.PHONY: test-e2e
test-e2e: ## Run end-to-end tests (requires running cluster).
	go test -tags=e2e -race -timeout 20m ./tests/e2e/...

##@ Build

.PHONY: build
build: fmt vet ## Build the operator binary.
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/audicia/

.PHONY: run
run: fmt vet ## Run the operator locally (outside cluster).
	go run -ldflags "$(LDFLAGS)" ./cmd/audicia/

.PHONY: docker-build
docker-build: ## Build the container image (multi-stage, no local Go needed).
	docker build -t $(IMG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-f build/Dockerfile .

.PHONY: build-azure
build-azure: fmt vet ## Build with Azure Event Hub support.
	go build -tags azure -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/audicia/

.PHONY: docker-build-azure
docker-build-azure: ## Build the container image with Azure support.
	docker build -t $(IMG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		--build-arg GO_BUILD_TAGS=azure \
		-f build/Dockerfile .

.PHONY: build-aws
build-aws: fmt vet ## Build with AWS CloudWatch support.
	go build -tags aws -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/audicia/

.PHONY: docker-build-aws
docker-build-aws: ## Build the container image with AWS support.
	docker build -t $(IMG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		--build-arg GO_BUILD_TAGS=aws \
		-f build/Dockerfile .

.PHONY: build-gcp
build-gcp: fmt vet ## Build with GCP Pub/Sub support.
	go build -tags gcp -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) ./cmd/audicia/

.PHONY: docker-build-gcp
docker-build-gcp: ## Build the container image with GCP support.
	docker build -t $(IMG) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		--build-arg GO_BUILD_TAGS=gcp \
		-f build/Dockerfile .

.PHONY: docker-push
docker-push: ## Push the container image.
	docker push $(IMG)

##@ Code Generation

.PHONY: generate
generate: ## Generate DeepCopy methods and CRD manifests.
	controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./pkg/apis/audicia.io/v1alpha1"
	controller-gen crd paths="./pkg/apis/audicia.io/v1alpha1" output:crd:artifacts:config=../deploy/helm/crds

.PHONY: manifests
manifests: generate ## Alias for generate.

##@ Deployment

.PHONY: install-crds
install-crds: generate ## Install CRDs into the cluster.
	kubectl apply -f ../deploy/helm/crds/

.PHONY: uninstall-crds
uninstall-crds: ## Uninstall CRDs from the cluster.
	kubectl delete -f ../deploy/helm/crds/

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart.
	helm lint ../deploy/helm/

.PHONY: helm-template
helm-template: ## Render the Helm chart templates.
	helm template audicia ../deploy/helm/

.PHONY: helm-install
helm-install: ## Install the Helm chart into the cluster.
	helm install audicia ../deploy/helm/ --namespace audicia-system --create-namespace

.PHONY: helm-uninstall
helm-uninstall: ## Uninstall the Helm chart from the cluster.
	helm uninstall audicia --namespace audicia-system

##@ Local Development

.PHONY: kind-cluster
kind-cluster: ## Create a kind cluster with audit logging enabled.
	kind create cluster --config hack/kind-config.yaml --name audicia-dev

.PHONY: kind-delete
kind-delete: ## Delete the kind development cluster.
	kind delete cluster --name audicia-dev

.PHONY: kind-load
kind-load: docker-build ## Load the container image into kind.
	kind load docker-image $(IMG) --name audicia-dev

##@ Cleanup

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf bin/ dist/ coverage.txt coverage.html
