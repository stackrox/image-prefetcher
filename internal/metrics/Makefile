# Makefile for generating protobuf and gRPC-related files.
# Much of this file is borrowed from https://github.com/stackrox/stackrox/blob/master/make/protogen.mk

BASE_PATH ?= $(CURDIR)
SILENT ?= @

.PHONY: all
all: generate

include $(BASE_PATH)/../../make/gotools.mk
include $(BASE_PATH)/../../make/github.mk

$(call go-tool, PROTOC_GEN_GO_BIN, google.golang.org/protobuf/cmd/protoc-gen-go, tools)
$(call go-tool, PROTOC_GEN_GO_GRPC_BIN, google.golang.org/grpc/cmd/protoc-gen-go-grpc, tools)

PROTOC_VERSION := 26.1
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
PROTOC_OS = linux
endif
ifeq ($(UNAME_S),Darwin)
PROTOC_OS = osx
endif
PROTOC_ARCH=$(shell case $$(uname -m) in (arm64) echo aarch_64 ;; (s390x) echo s390_64 ;; (*) uname -m ;; esac)

PROTO_PRIVATE_DIR := $(BASE_PATH)/tools

PROTOC_DIR := $(PROTO_PRIVATE_DIR)/protoc-$(PROTOC_OS)-$(PROTOC_ARCH)-$(PROTOC_VERSION)

PROTOC := $(PROTOC_DIR)/bin/protoc

PROTOC_DOWNLOADS_DIR := $(PROTO_PRIVATE_DIR)/.downloads

.PHONY: generate
generate: $(PROTOC) $(PROTOC_GEN_GO_BIN) $(PROTOC_GEN_GO_GRPC_BIN)
	mkdir -p gen
	export PATH=$(BASE_PATH)/tools:$(BASE_PATH)/.gotools/bin:$$PATH; \
	$(PROTOC) --go_out=gen --go_opt=paths=source_relative --go-grpc_out=gen --go-grpc_opt=paths=source_relative metrics.proto

$(PROTOC_DOWNLOADS_DIR):
	@echo "+ $@"
	$(SILENT)mkdir -p "$@"

PROTOC_ZIP := protoc-$(PROTOC_VERSION)-$(PROTOC_OS)-$(PROTOC_ARCH).zip
PROTOC_FILE := $(PROTOC_DOWNLOADS_DIR)/$(PROTOC_ZIP)

$(PROTOC_FILE): $(PROTOC_DOWNLOADS_DIR)
	@$(GET_GITHUB_RELEASE_FN); \
	get_github_release "$@" "https://github.com/protocolbuffers/protobuf/releases/download/v$(PROTOC_VERSION)/$(PROTOC_ZIP)"

.PRECIOUS: $(PROTOC_FILE)

$(PROTOC):
	@echo "+ $@"
	$(SILENT)$(MAKE) "$(PROTOC_FILE)"
	$(SILENT)mkdir -p "$(PROTOC_DIR)"
	$(SILENT)unzip -q -o -d "$(PROTOC_DIR)" "$(PROTOC_FILE)"
	$(SILENT)test -x "$@"

