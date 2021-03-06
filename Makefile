#!/usr/bin/env bash
# Makefile with some common workflow for dev, build and test

##@ General

# The help will print out all targets with their descriptions organized bellow their categories. The categories are represented by `##@` and the target descriptions by `##`.
# The awk commands is responsable to read the entire set of makefiles included in this invocation, looking for lines of the file as xyz: ## something, and then pretty-format the target and help. Then, if there's a line with ##@ something, that gets pretty-printed as a category.
# More info over the usage of ANSI control characters for terminal formatting: https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info over awk command: http://linuxcommand.org/lc3_adv_awk.php
.PHONY: help
help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Tests

.PHONY: test
test: ## Run the tests
	GO111MODULE=on TRACE=1 go test -v ./pkg/controller/servicesync/

.PHONY: test-kube
test-kube: ## Run the tests with Kubernetes
	GOFLAGS="-tags=testkube" GO111MODULE=on TRACE=1 go test -v ./pkg/controller/servicesync/

##@ Build

# [build] is a convenience method. Take a look at the implementation to know why ;)
# Getting the /Users/you directory & the go module directory to be removed was a bit of a hassle,
# for more info about trimpath: https://github.com/golang/go/commit/4891a3b66c482b42fdc74ae382e0cf4817d0fda2
.PHONY: build
build:
	echo "Building operator (-trimpath=$$(dirname $$PWD);$$HOME)"; \
GOOS=linux CGO_ENABLED=0 go build -o build/_output/bin/mc-robot \
-gcflags "all=-trimpath=$$(dirname $$PWD);$$HOME" \
-asmflags "all=-trimpath=$$(dirname $$PWD);$$HOME" \
-ldflags=-buildid= \
q42/mc-robot/cmd/manager && \
echo "SHASUM:" && shasum build/_output/bin/mc-robot && \
docker build -f build/Dockerfile -t $$REGISTRY/mc-robot:$$VERSION .

.PHONY: install
install:
	kubectl apply -f deploy/0_mc.q42.nl_servicesyncs_crd.yaml

.PHONY: deploy
deploy:
	docker push $$REGISTRY/mc-robot:$$VERSION; \
kubectl apply -f deploy/1_rbac.yaml; \
sed "s|REPLACE_IMAGE|$$REGISTRY/mc-robot:$$VERSION|g" deploy/2_operator.yaml | kubectl apply -f -
