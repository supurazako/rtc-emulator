GO ?= go
BIN_DIR ?= bin
BIN_NAME ?= rtc-emulator
CMD_PATH ?= ./cmd/rtc-emulator
GOOS ?= linux
GOARCH ?= amd64

.DEFAULT_GOAL := help

.PHONY: help fmt build

help:
	@echo "Available targets:"
	@echo "  make fmt    - Run go fmt"
	@echo "  make build  - Build rtc-emulator for linux/amd64"

fmt:
	$(GO) fmt ./...

# Supported build target is linux/amd64 only.
build:
	GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -o $(BIN_DIR)/$(BIN_NAME) $(CMD_PATH)
