DOCKER = $(shell which docker)
MOCKS_DIR=$(CURDIR)/testutil/mocks
MOCKGEN_REPO=github.com/golang/mock/mockgen
MOCKGEN_VERSION=v1.6.0
MOCKGEN_CMD=go run ${MOCKGEN_REPO}@${MOCKGEN_VERSION}
BUILDDIR ?= $(CURDIR)/build

BABYLON_PKG := github.com/babylonlabs-io/babylon/cmd/babylond

GO_BIN := ${GOPATH}/bin

ldflags := $(LDFLAGS)
build_tags := $(BUILD_TAGS)
build_args := $(BUILD_ARGS)

PACKAGES_E2E=$(shell go list ./... | grep '/e2e')

ifeq ($(LINK_STATICALLY),true)
	ldflags += -linkmode=external -extldflags "-Wl,-z,muldefs -static" -v
endif

ifeq ($(VERBOSE),true)
	build_args += -v
endif

BUILD_TARGETS := build install
BUILD_FLAGS := --tags "$(build_tags)" --ldflags '$(ldflags)'

# Update changelog vars
ifneq (,$(SINCE_TAG))
       sinceTag := --since-tag $(SINCE_TAG)
endif
ifneq (,$(UPCOMING_TAG))
       upcomingTag := --future-release $(UPCOMING_TAG)
endif

all: build install

build: BUILD_ARGS := $(build_args) -o $(BUILDDIR)

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ -mod=readonly $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

test:
	go test -race ./...

test-e2e:
	go test -mod=readonly -failfast -timeout=15m -v $(PACKAGES_E2E) -count=1 --parallel 12 --tags=e2e

build-docker:
	$(DOCKER) build --tag babylonlabs-io/vigilante -f Dockerfile \
		$(shell git rev-parse --show-toplevel)

rm-docker:
	$(DOCKER) rmi babylonlabs-io/vigilante 2>/dev/null; true

mocks:
	mkdir -p $(MOCKS_DIR)
	$(MOCKGEN_CMD) -source=btcclient/interface.go -package mocks -destination $(MOCKS_DIR)/btcclient.go
	$(MOCKGEN_CMD) -source=submitter/poller/expected_babylon_client.go -package poller -destination submitter/poller/mock_babylon_client.go
	$(MOCKGEN_CMD) -source=submitter/expected_babylon_client.go -package submitter -destination submitter/mock_babylon_client.go
	$(MOCKGEN_CMD) -source=reporter/expected_babylon_client.go -package reporter -destination reporter/mock_babylon_client.go
	$(MOCKGEN_CMD) -source=monitor/expected_babylon_client.go -package monitor -destination monitor/mock_babylon_client.go
	$(MOCKGEN_CMD) -source=btcstaking-tracker/btcslasher/expected_babylon_client.go -package btcslasher -destination btcstaking-tracker/btcslasher/mock_babylon_client.go
	$(MOCKGEN_CMD) -source=btcstaking-tracker/atomicslasher/expected_babylon_client.go -package atomicslasher -destination btcstaking-tracker/atomicslasher/mock_babylon_client.go
	$(MOCKGEN_CMD) -source=btcstaking-tracker/stakingeventwatcher/expected_babylon_client.go -package stakingeventwatcher -destination btcstaking-tracker/stakingeventwatcher/mock_babylon_client.go

update-changelog:
	@echo ./scripts/update_changelog.sh $(sinceTag) $(upcomingTag)
	./scripts/update_changelog.sh $(sinceTag) $(upcomingTag)

.PHONY: build test test-e2e build-docker rm-docker mocks update-changelog

proto-gen:
	@$(call print, "Compiling protos.")
	cd ./proto; ./gen_protos_docker.sh

.PHONY: proto-gen