BINARY_NAME := uptimyctl
PKG := github.com/uptimy/uptimyctl
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) \
           -X $(PKG)/internal/version.Commit=$(COMMIT) \
           -X $(PKG)/internal/version.BuildDate=$(BUILD_DATE)

.PHONY: all build test lint fmt vet clean tidy coverage

all: lint test build

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY_NAME) .

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin/ dist/ coverage.out coverage.html

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

tidy:
	go mod tidy

.DEFAULT_GOAL := build
