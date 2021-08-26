REGISTRY=github/alibaba
ARCHS?=amd64 arm64
DEV_TAG?=dev
RELEASE_TAG?=release

.PHONY: build-dev-images release

build-dev-images:
	@for arch in ${ARCHS} ; do \
    	docker build -t ${REGISTRY}/hybridnet:${DEV_TAG}-$$arch -f Dockerfile.$$arch ./; \
    done

release:
	@for arch in ${ARCHS} ; do \
		docker build -t ${REGISTRY}/hybridnet:${RELEASE_TAG}-$$arch -f Dockerfile.$$arch ./; \
	done
