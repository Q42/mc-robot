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
test: ## Run the script check-everything.sh which will check all
	GO111MODULE=on TRACE=1 go test -v ./pkg/controller/servicesync/

.PHONY: build
build:
	operator-sdk build $$REGISTRY/mc-robot:$$VERSION --verbose

.PHONY: deploy
deploy:
	docker push $$REGISTRY/mc-robot:$$VERSION; \
	sed "s|REPLACE_IMAGE|$$REGISTRY/mc-robot:$$VERSION|g" deploy/operator.yaml | kubectl apply -f -
