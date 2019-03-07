TAG ?= v0.4.0
IMAGE_NAME = 3scale-istio-adapter:$(TAG)
REGISTRY ?= quay.io/3scale
LISTEN_ADDR ?= 3333
PROJECT_PATH := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

DEP_LOCK = $(PROJECT_PATH)/Gopkg.lock
SOURCES := $(shell find $(PROJECT_PATH)/pkg -name '*.go')

## Build targets ##

3scale-istio-adapter: update-dependencies $(DEP_LOCK) $(PROJECT_PATH)/cmd/server/main.go $(SOURCES) ## Build the adapter binary
	go build -o _output/3scale-istio-adapter cmd/server/main.go

3scale-config-gen: update-dependencies $(DEP_LOCK) $(PROJECT_PATH)/cmd/cli/main.go $(SOURCES) ## Build the config generator cli
	go build -ldflags="-s -w" -o _output/3scale-config-gen cmd/cli/main.go

.PHONY: build-adapter
build-adapter: 3scale-istio-adapter ## Alias to build the adapter binary

.PHONY: build-cli
build-cli: 3scale-config-gen ## Alias to build the config generator cli

## Testing targets ##

.PHONY: unit
unit: ## Run unit tests
	mkdir -p "$(PROJECT_PATH)/_output"
	go test ./... -covermode=count -test.coverprofile="$(PROJECT_PATH)/_output/unit.cov"

.PHONY: integration
integration: ## Run integration tests
	go test -covermode=count -tags integration -test.coverprofile="$(PROJECT_PATH)/_output/integration.cov" -run=TestAuthorizationCheck ./...

.PHONY: test
test: unit integration ## Runs all tests

.PHONY: unit_coverage
unit_coverage: unit ## Runs unit tests and generates a html coverage report
	go tool cover -html="$(PROJECT_PATH)/_output/unit.cov"

.PHONY: integration_coverage
integration_coverage: integration ## Runs integration tests and generates a html coverage report
	go tool cover -html="$(PROJECT_PATH)/_output/integration.cov"

## Docker targets ##

.PHONY: docker-build
docker-build: ## Build builder image
	docker build -f $(PROJECT_PATH)/Dockerfile --tag $(REGISTRY)/$(IMAGE_NAME) .

.PHONY: docker-test
docker-test: ## Runs the adapter - useful for smoke testing
	docker build -f $(PROJECT_PATH)/Dockerfile --tag $(IMAGE_NAME)-test .
	docker run -e THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} -ti $(IMAGE_NAME)-test

.PHONY: docker-push
docker-push: docker-build ## Build and push the adapter image to the docker registry
	docker push $(REGISTRY)/$(IMAGE_NAME)

.PHONY: debug-image
debug-image: ## Builds a debuggable image which is started via Delve
	docker build -f $(PROJECT_PATH)/Dockerfile.dev --tag $(REGISTRY)/$(IMAGE_NAME) .

## Misc ##

.PHONY: generate-config
generate-config: ## Generates required artifacts for an out-of-process adapter based on the current .proto file
	$(PROJECT_PATH)/scripts/generate-config.sh

.PHONY: update-dependencies
update-dependencies:
	dep ensure

.PHONY: build-adapter
run-adapter: ## Run the adapter
	THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} "$(PROJECT_PATH)/_output/3scale-istio-adapter"

.PHONY: run-mixer-server
run-mixer-server: ## Run the mixer server with test configuration
	mixs server --configStoreURL=fs://$(PROJECT_PATH)/testdata
