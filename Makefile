REGISTRY=github/alibaba
ARCHS?=amd64 arm64
DEV_TAG?=dev
RELEASE_TAG?=release

INIT_YAML_FILE=yamls/hybridnet-init.yaml
RBAC_YAML_FILE=yamls/rbac/rbac.yaml
CRD_YAML_DIR=yamls/crd

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


init-yaml: crd-yamls
	cat ${RBAC_YAML_FILE} > ${INIT_YAML_FILE}
	@for f in $(shell ls ${CRD_YAML_DIR}); do cat ${CRD_YAML_DIR}/$${f} >> ${INIT_YAML_FILE}; done


crd-yamls: controller-gen ## Generate CustomResourceDefinition objects.
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=hybridnet webhook paths="./..." output:crd:artifacts:config=${CRD_YAML_DIR}

generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.1)

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
