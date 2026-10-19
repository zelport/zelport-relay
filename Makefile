GO ?= go
VERSION ?= dev
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
.DEFAULT_GOAL := check
.PHONY: help check verify build check-integration

help:
	@printf '%s\n' 'make check' 'make verify' 'make build' 'make check-integration'

check:
	python3 scripts/test_integration_harness.py
	GOWORK=off $(GO) test -race ./...
	GOWORK=off $(GO) vet ./...

verify: check build

build:
	GOWORK=off $(GO) build -ldflags "-X main.version=$(VERSION) -X 'main.builtAt=$(BUILD_TIME)'" -o bin/zelport-relay ./cmd/zelport-relay
check-integration:
	GO='$(GO)' sh scripts/check-sdk-integration.sh
