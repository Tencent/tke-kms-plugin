# Copyright (C) 2020 THL A29 Limited, a Tencent company.  All rights reserved.
#
# TKE KMS Plugin 腾讯云TKE KMS插件 is licensed under the Apache License Version 2.0.

REGISTRY ?= ""
VERSION ?= 1.0.0

ifeq ($(REGISTRY), "")
	IMAGE=tke-kms-plugin:$(VERSION)
else
	IMAGE=$(REGISTRY)/tke-kms-plugin:$(VERSION)
endif

OUTPUT_DIR = _output
BINARY = tke-kms-plugin

all: fmt vet
	@echo "building $(BINARY)..."
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(OUTPUT_DIR)/$(BINARY) .

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Build the docker image
image: Dockerfile all
	@docker build -f Dockerfile -t ${IMAGE} $(OUTPUT_DIR)

clean:
	@echo "cleaning..."
	@rm -rf $(OUTPUT_DIR)

