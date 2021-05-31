GOFILES_NOVENDOR=$(shell find . -type f -name '*.go' -not -path "./vendor/*")
GO_VERSION?=1.13.8
GOLANG_IMAGE=golang

REGISTRY=github/oecp
ROLES=daemon manager webhook
DEV_TAG=dev
ARCH?=amd64
RELEASE_TAG=$(shell cat VERSION)

.PHONY: build-dev-images build-go build-bin test lint

build-dev-images: build-bin
	@for role in ${ROLES} ; do \
		docker build -t ${REGISTRY}/rama-$$role:${DEV_TAG}-$(ARCH) -f dist/images/Dockerfile-$$role.$(ARCH) dist/images/; \
	done

build-go:
	GO111MODULE=on GOARCH=$(ARCH) CGO_ENABLED=0 GOOS=linux go build -o $(PWD)/dist/images/rama -ldflags "-w -s" -v ./cmd/cni
	GO111MODULE=on GOARCH=$(ARCH) CGO_ENABLED=0 GOOS=linux go build -o $(PWD)/dist/images/rama-daemon -ldflags "-w -s" -v ./cmd/daemon
	GO111MODULE=on GOARCH=$(ARCH) CGO_ENABLED=0 GOOS=linux go build -o $(PWD)/dist/images/rama-manager -v ./cmd/manager
	GO111MODULE=on GOARCH=$(ARCH) CGO_ENABLED=0 GOOS=linux go build -o $(PWD)/dist/images/rama-webhook -v ./cmd/webhook

release: build-bin
	@for role in ${ROLES} ; do \
		docker build -t ${REGISTRY}/rama-$$role:${RELEASE_TAG}-$(ARCH) -f dist/images/Dockerfile-$$role.$(ARCH) dist/images/; \
	done

#lint:
#	@gofmt -d ${GOFILES_NOVENDOR}
#	@gofmt -l ${GOFILES_NOVENDOR} | read && echo "Code differs from gofmt's style" 1>&2 && exit 1 || true
#	@GOOS=linux go vet ./...

test:
	GOOS=linux go test -cover -v ./...

build-bin:
	docker run --rm -e GOOS=linux -e GOCACHE=/tmp \
		-u $(shell id -u):$(shell id -g) \
		-v $(CURDIR):/go/src/github.com/oecp/rama:ro \
		-v $(CURDIR)/dist:/go/src/github.com/oecp/rama/dist/ \
		$(GOLANG_IMAGE):$(GO_VERSION) /bin/bash -c '\
		cd /go/src/github.com/oecp/rama && \
		ARCH=$(ARCH) make build-go '
