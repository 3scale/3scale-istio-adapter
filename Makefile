TAG ?= 0.2
IMAGE_NAME = 3scaleistioadapter:$(TAG)
REGISTRY ?= quay.io/3scale
LISTEN_ADDR ?= 3333
PROJECT_PATH := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))

build: ## Build the adapter binary
	dep ensure
	go build -o _output/3scaleAdapter cmd/main.go

run-adapter: ## Run the adapter
	THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} go run cmd/main.go

run-mixer-server: ## Run the mixer server with test configuration
	mixs server --configStoreURL=fs://$(PROJECT_PATH)/testdata

unit: ## Run unit tests
	mkdir -p "$(PROJECT_PATH)/_output"
	go test ./... -covermode=count -test.coverprofile="$(PROJECT_PATH)/_output/unit.cov"

integration: ## Run integration tests
	go test ./... -covermode=count -tags integration -test.coverprofile="$(PROJECT_PATH)/_output/integration.cov"

test: unit integration ## Runs all tests

unit_coverage: unit ## Runs unit tests and generates a html coverage report
	go tool cover -html="$(PROJECT_PATH)/_output/unit.cov"

integration_coverage: integration ## Runs integration tests and generates a html coverage report
	go tool cover -html="$(PROJECT_PATH)/_output/integration.cov"

debug-image: ## Builds a debuggable image which is started via Delve
	docker build -f $(PROJECT_PATH)/Dockerfile.dev --tag $(REGISTRY)/$(IMAGE_NAME) .

docker-build: ## Build builder image
	docker build -f $(PROJECT_PATH)/Dockerfile --tag $(REGISTRY)/$(IMAGE_NAME) .

docker-test: ## Runs the adapter
	docker build -f $(PROJECT_PATH)/Dockerfile --tag $(IMAGE_NAME)-test .
	docker run -e THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} -ti $(IMAGE_NAME)-test

docker-push: ## Push both builder and runtime image to the docker registry
	docker push $(REGISTRY)/$(IMAGE_NAME)

generate-config: ## Generates required artifacts for an out-of-process adapter based on the current .proto file
	$(PROJECT_PATH)/scripts/generate-config.sh
