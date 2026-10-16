GO ?= go
VERSION ?= dev
BUILD_TIME := $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
.PHONY: check build check-integration
check:
	python3 scripts/test_integration_harness.py
	GOWORK=off $(GO) test -race ./...
	GOWORK=off $(GO) vet ./...
build:
	GOWORK=off $(GO) build -ldflags "-X main.version=$(VERSION) -X 'main.builtAt=$(BUILD_TIME)'" -o bin/zelport-relay ./cmd/zelport-relay
check-integration:
	GO='$(GO)' sh scripts/check-sdk-integration.sh
