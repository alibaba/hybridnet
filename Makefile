REGISTRY=github/alibaba
ARCHS?=amd64 arm64
DEV_TAG?=dev
RELEASE_TAG?=release
GOOS=`go env GOOS`
GOARCH=`go env GOARCH`

CRD_YAML_DIR=charts/hybridnet/crds

# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true,preserveUnknownFields=false"

.PHONY: build-dev-images release

build-dev-images:
	@for arch in ${ARCHS} ; do \
    	docker build -t ${REGISTRY}/hybridnet:${DEV_TAG}-$$arch -f Dockerfile.$$arch ./; \
    done

release:
	@for arch in ${ARCHS} ; do \
		docker build -t ${REGISTRY}/hybridnet:${RELEASE_TAG}-$$arch -f Dockerfile.$$arch ./; \
	done

code-gen:
	cd hack && chmod u+x ./update-codegen.sh && ./update-codegen.sh

crd-yamls: controller-gen ## Generate CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=hybridnet webhook paths="./..." output:crd:artifacts:config=${CRD_YAML_DIR} && rm -rf ./config

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

# use command of "./bin/kubebuilder create api --group multicluster --kind RemoteXXX --version v1 --namespaced=false" to generate crd types
KUBEBUILDER_BIN = $(shell pwd)/bin/kubebuilder
kubebuilder: ## Download kubebuilder binary locally if necessary.
	$(call curl-get-tool,$(KUBEBUILDER_BIN),https://github.com/kubernetes-sigs/kubebuilder/releases/download/v3.1.0/kubebuilder_${GOOS}_${GOARCH})

# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

define curl-get-tool
@[ -f $(1) ] || { \
echo "Downloading $(2)" ;\
curl -L -o $(1) $(2) ;\
chmod +x $(1) ;\
}
endef
