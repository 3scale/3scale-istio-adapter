TAG ?= 0.1
IMAGE_NAME = 3scaleistioadapter:$(TAG)
REGISTRY ?= quay.io/3scale
PORT ?= 3333

build: ## Build builder image
	docker build --tag $(REGISTRY)/$(IMAGE_NAME) .

test: ## Runs the adapter
	docker build --tag $(IMAGE_NAME)-test .
	docker run -e PORT=${PORT} -p ${PORT}:${PORT} -ti $(IMAGE_NAME)-test

push: ## Push both builder and runtime image to the docker registry
	docker push $(REGISTRY)/$(IMAGE_NAME)
