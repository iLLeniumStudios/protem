# VERSION is the release version, without the leading v: the bundle's CSV version and the tag of
# the operator, bundle and catalog images (e.g. make bundle VERSION=0.1.0).
VERSION ?= 0.0.1

# The bundle's only channel, which is also its default.
CHANNELS ?= alpha
DEFAULT_CHANNEL ?= alpha
BUNDLE_METADATA_OPTS ?= --channels=$(CHANNELS) --default-channel=$(DEFAULT_CHANNEL)

# IMAGE_TAG_BASE is the registry and name the operator, bundle and catalog images share.
IMAGE_TAG_BASE ?= ghcr.io/illeniumstudios/protem

# BUNDLE_IMG defines the image:tag used for the bundle.
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# The bundle's icon, embedded into its ClusterServiceVersion by make bundle.
BUNDLE_ICON ?= brand/web/protem-symbol-app-icon.svg

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Set the Operator SDK version to use. By default, what is installed on the system is used.
# This is useful for CI or a project to utilize a specific version of the operator-sdk toolkit.
OPERATOR_SDK_VERSION ?= v1.42.3
# Image URL to use all building/pushing image targets
IMG ?= $(IMAGE_TAG_BASE):v$(VERSION)
# Image for the agent that runs next to each target cluster (cmd/enforcer).
AGENT_IMG ?= protem-agent:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen generate-ts ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: generate-ts
generate-ts: ## Generate web/src/generated TypeScript types from internal/api/views.go.
	go tool tygo generate

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./internal/... -v -coverprofile cover.out

# TODO(user): To use a different vendor for e2e tests, modify the setup under 'tests/e2e'.
# The default setup assumes Kind is pre-installed and builds/loads the Manager Docker image locally.
# CertManager is installed by default; skip with:
# - CERT_MANAGER_INSTALL_SKIP=true
KIND_CLUSTER ?= protem-test-e2e

.PHONY: setup-test-e2e
setup-test-e2e: ## Set up a Kind cluster for e2e tests if it does not exist
	@command -v $(KIND) >/dev/null 2>&1 || { \
		echo "Kind is not installed. Please install Kind manually."; \
		exit 1; \
	}
	@case "$$($(KIND) get clusters)" in \
		*"$(KIND_CLUSTER)"*) \
			echo "Kind cluster '$(KIND_CLUSTER)' already exists. Skipping creation." ;; \
		*) \
			echo "Creating Kind cluster '$(KIND_CLUSTER)'..."; \
			$(KIND) create cluster --name $(KIND_CLUSTER) ;; \
	esac

.PHONY: test-e2e
test-e2e: setup-test-e2e manifests generate fmt vet ## Run the e2e tests. Expected an isolated environment using Kind.
	KIND_CLUSTER=$(KIND_CLUSTER) go test ./test/e2e/ -v -ginkgo.v
	$(MAKE) cleanup-test-e2e

.PHONY: cleanup-test-e2e
cleanup-test-e2e: ## Tear down the Kind cluster used for e2e tests
	@$(KIND) delete cluster --name $(KIND_CLUSTER)

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

.PHONY: lint-config
lint-config: golangci-lint ## Verify golangci-lint linter configuration
	$(GOLANGCI_LINT) config verify

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-build-agent
docker-build-agent: ## Build docker image with the agent for target clusters.
	$(CONTAINER_TOOL) build --target enforcer -t ${AGENT_IMG} .

.PHONY: docker-push-agent
docker-push-agent: ## Push docker image with the agent.
	$(CONTAINER_TOOL) push ${AGENT_IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name protem-builder
	$(CONTAINER_TOOL) buildx use protem-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm protem-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Helm

HELM ?= helm
HELM_CHART ?= deploy/helm/protem

.PHONY: helm-sync
helm-sync: ## Copy the CRDs and the manager ClusterRole from config/ into the Helm chart.
	mkdir -p $(HELM_CHART)/crds
	rm -f $(HELM_CHART)/crds/*.yaml
	cp config/crd/bases/*.yaml $(HELM_CHART)/crds/
	{ cat hack/helm/manager-clusterrole-header.yaml; sed -n '/^rules:/,$$p' config/rbac/role.yaml; } > $(HELM_CHART)/templates/manager-clusterrole.yaml

.PHONY: helm-lint
helm-lint: ## Lint the Helm chart with its default values and with ci/full-values.yaml.
	$(HELM) lint --strict $(HELM_CHART)
	$(HELM) lint --strict $(HELM_CHART) -f $(HELM_CHART)/ci/full-values.yaml

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint

## Tool Versions
KUSTOMIZE_VERSION ?= v5.6.0
CONTROLLER_TOOLS_VERSION ?= v0.23.0
#ENVTEST_VERSION is the version of controller-runtime release branch to fetch the envtest setup script (i.e. release-0.20)
ENVTEST_VERSION ?= $(shell go list -m -f "{{ .Version }}" sigs.k8s.io/controller-runtime | awk -F'[v.]' '{printf "release-%d.%d", $$2, $$3}')
#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')
GOLANGCI_LINT_VERSION ?= v2.1.0

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: setup-envtest
setup-envtest: envtest ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/v2/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef

.PHONY: operator-sdk
OPERATOR_SDK ?= $(LOCALBIN)/operator-sdk
operator-sdk: ## Download operator-sdk locally if necessary.
ifeq (,$(wildcard $(OPERATOR_SDK)))
ifeq (, $(shell which operator-sdk 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPERATOR_SDK)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPERATOR_SDK) https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$${OS}_$${ARCH} ;\
	chmod +x $(OPERATOR_SDK) ;\
	}
else
OPERATOR_SDK = $(shell which operator-sdk)
endif
endif

# Builds from a copy of config/ so the tree is left as it was. The CSV base is kept by hand:
# operator-sdk generate kustomize manifests would replace its CRD descriptions.
.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle/ and bundle.Dockerfile for IMG, with the icon from BUNDLE_ICON, then validate them.
	rm -rf bundle
	icon=$$(base64 < $(BUNDLE_ICON) | tr -d '\n') && tmp=$$(mktemp -d) && trap 'rm -rf "$$tmp"' EXIT && \
	cp -R config "$$tmp/" && \
	(cd "$$tmp/config/manager" && $(KUSTOMIZE) edit set image controller=$(IMG)) && \
	(cd "$$tmp/config/manifests" && $(KUSTOMIZE) edit add patch --kind ClusterServiceVersion --patch \
		'[{"op":"replace","path":"/metadata/annotations/containerImage","value":"$(IMG)"},{"op":"replace","path":"/spec/icon/0/base64data","value":"'"$$icon"'"}]') && \
	$(KUSTOMIZE) build "$$tmp/config/manifests" | $(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate ./bundle --select-optional suite=operatorframework

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	$(CONTAINER_TOOL) build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = $(LOCALBIN)/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.55.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# Extra flags for opm render, e.g. --use-http for a local registry without TLS.
OPM_RENDER_FLAGS ?=

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# The catalog holds one version: skipRange lets OLM upgrade any earlier version straight to it.
# opm pulls with containers/image, which needs a policy.json; it gets its own, keeping docker's credentials.
.PHONY: catalog-build
catalog-build: opm ## Render the pushed BUNDLE_IMG into a file-based catalog in catalog/, validate it and build CATALOG_IMG.
	rm -rf catalog catalog.Dockerfile
	mkdir -p catalog/protem
	$(OPM) init protem --default-channel=$(DEFAULT_CHANNEL) --icon=$(BUNDLE_ICON) --output=yaml > catalog/protem/catalog.yaml
	home=$$(mktemp -d) && trap 'rm -rf "$$home"' EXIT && mkdir -p "$$home/.config/containers" && \
	echo '{"default":[{"type":"insecureAcceptAnything"}]}' > "$$home/.config/containers/policy.json" && \
	DOCKER_CONFIG="$${DOCKER_CONFIG:-$$HOME/.docker}" HOME="$$home" \
		$(OPM) render $(BUNDLE_IMG) $(OPM_RENDER_FLAGS) --output=yaml >> catalog/protem/catalog.yaml
	printf -- '---\nschema: olm.channel\npackage: protem\nname: %s\nentries:\n- name: protem.v%s\n  skipRange: "<%s"\n' \
		$(DEFAULT_CHANNEL) $(VERSION) $(VERSION) >> catalog/protem/catalog.yaml
	$(OPM) validate catalog
	$(OPM) generate dockerfile catalog
	$(CONTAINER_TOOL) build -f catalog.Dockerfile -t $(CATALOG_IMG) .

.PHONY: catalog-push
catalog-push: ## Push the catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)

##@ Documentation

.PHONY: docs-reference
docs-reference: ## Regenerate docs/reference/ from the CRDs, --help output and chart values.
	go run ./hack/docs/...

##@ Website

SITE_VENV ?= $(LOCALBIN)/site-venv
SITE_PORT ?= 8000
# Mermaid is served from the site instead of unpkg.com; the tarball is checked against the npm registry's integrity hash.
MERMAID_VERSION ?= 11.17.2
MERMAID_SHA256 ?= 6ad2f42c3fc26bbf9e45cbb6d11898972573ea52b33a5f4ff51952899f950ffd
MERMAID_DIR ?= $(LOCALBIN)/mermaid-$(MERMAID_VERSION)

.PHONY: site
site: $(SITE_VENV)/bin/mkdocs $(MERMAID_DIR) ## Build the website into _site/: the landing page, the docs in _site/docs/, CNAME, .nojekyll and 404.html.
	rm -rf _site
	NO_MKDOCS_2_WARNING=1 $(SITE_VENV)/bin/mkdocs build
	mkdir -p _site/docs/assets/javascripts
	cp $(MERMAID_DIR)/dist/mermaid.min.js _site/docs/assets/javascripts/mermaid.min.js
	cp $(MERMAID_DIR)/LICENSE _site/docs/assets/javascripts/LICENSE-mermaid.txt
	# The CDN in front of the site caches styles.css for hours, so its URL carries a hash of its content.
	if [ -d site/landing ]; then cp -R site/landing/. _site/ && \
		v=$$(shasum -a 256 _site/styles.css | cut -c1-10) && \
		sed -i.bak "s|href=\"styles.css\"|href=\"styles.css?v=$$v\"|" _site/index.html && rm _site/index.html.bak; fi
	cp site/docs/404.html _site/404.html
	echo protem.dev > _site/CNAME
	touch _site/.nojekyll

.PHONY: site-serve
site-serve: site ## Build the website and serve _site/ on http://localhost:$(SITE_PORT).
	$(SITE_VENV)/bin/python -m http.server $(SITE_PORT) --bind 127.0.0.1 --directory _site

$(SITE_VENV)/bin/mkdocs: site/docs/requirements.txt | $(LOCALBIN)
	python3 -m venv $(SITE_VENV)
	$(SITE_VENV)/bin/pip install --quiet --disable-pip-version-check -r site/docs/requirements.txt
	touch $@

$(MERMAID_DIR): | $(LOCALBIN)
	curl -sSfL -o $@.tgz https://registry.npmjs.org/mermaid/-/mermaid-$(MERMAID_VERSION).tgz
	echo "$(MERMAID_SHA256)  $@.tgz" | shasum -a 256 -c -
	mkdir -p $@.tmp
	tar -xzf $@.tgz -C $@.tmp --strip-components=1 package/dist/mermaid.min.js package/LICENSE
	rm $@.tgz
	mv $@.tmp $@
