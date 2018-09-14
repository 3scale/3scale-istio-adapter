TAG ?= 0.1-httpPlugin
IMAGE_NAME = 3scaleistioadapter:$(TAG)
REGISTRY ?= quay.io/3scale
LISTEN_ADDR ?= 3333

build: ## Build go binary
	go build -o 3scaleAdapter cmd/main.go

adapter: ## run the adapter
	THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} go run cmd/main.go

mixer: ## Run the mixer server
	mixs server --configStoreURL=fs://$(CURDIR)/testdata

test: ## Run golang tests
	go test ./... 

coverage: ## Show code coverage
	go test -coverprofile=c.out ./... && go tool cover -html=c.out

docker-build: ## Build builder image
	docker build --tag $(REGISTRY)/$(IMAGE_NAME) .

docker-test: ## Runs the adapter
	docker build --tag $(IMAGE_NAME)-test .
	docker run -e THREESCALE_LISTEN_ADDR=${LISTEN_ADDR} -ti $(IMAGE_NAME)-test

docker-push: ## Push both builder and runtime image to the docker registry
	docker push $(REGISTRY)/$(IMAGE_NAME)
