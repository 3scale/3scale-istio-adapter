TAG ?= 0.1-httpPlugin
IMAGE_NAME = 3scaleistioadapter:$(TAG)
REGISTRY ?= quay.io/3scale
LISTEN_ADDR ?= 3333

build: ## Build go binary
	go build -o _output/3scaleAdapter cmd/main.go

adapter: ## run the adapter
	THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} go run cmd/main.go

build_test_adapter: ## build the adapter with test coverage
	cd ./cmd && go test -c -tags integration -covermode=count \
	-coverpkg=.,github.com/3scale/istio-integration/3scaleAdapter/pkg/... \
	-o ../_output/3scale-adapter-integration-bin

mixer: ## Run the mixer server
	mixs server --configStoreURL=fs://$(CURDIR)/testdata

unit: ## Run golang tests
	mkdir -p "$(CURDIR)/_output"
	go test ./... -covermode=count -test.coverprofile="$(CURDIR)/_output/unit.cov"

integration: build_test_adapter
	./scripts/integration_tests.sh ${START_DEPENDENCIES}

test: unit integration

unit_coverage: unit
	go tool cover -html="$(CURDIR)/_output/unit.cov"

integration_coverage: integration
	go tool cover -html="$(CURDIR)/_output/integration.cov"

docker-build: ## Build builder image
	docker build --tag $(REGISTRY)/$(IMAGE_NAME) .

docker-test: ## Runs the adapter
	docker build --tag $(IMAGE_NAME)-test .
	docker run -e THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} -ti $(IMAGE_NAME)-test

docker-push: ## Push both builder and runtime image to the docker registry
	docker push $(REGISTRY)/$(IMAGE_NAME)
