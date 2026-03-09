VERSION := $(shell cat VERSION.md 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"
BINARY  := github-subdomains

.PHONY: build test lint cover install clean release fmt vet

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/github-subdomains

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

cover:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

install:
	go install $(LDFLAGS) ./cmd/github-subdomains

clean:
	rm -f $(BINARY) coverage.out coverage.html
	rm -f main

release:
	goreleaser release --snapshot --clean

fmt:
	gofmt -s -w .

vet:
	go vet ./...
