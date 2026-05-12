.PHONY: build run test clean install web-build web-deps

BINARY=every-sync
VERSION?=dev
BUILD_DIR=./bin
GO=go
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

web-deps:
	cd web && npm install

web-build: web-deps
	cd web && npm run build
	rm -rf internal/server/static/*
	cp -r web/dist/* internal/server/static/

build: web-build
	$(GO) build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY) ./cmd/every-sync

run: build
	$(BUILD_DIR)/$(BINARY) serve

test:
	$(GO) test -v -race ./...

clean:
	rm -rf $(BUILD_DIR)

install: build
	cp $(BUILD_DIR)/$(BINARY) /usr/local/bin/

deps:
	$(GO) mod tidy

lint:
	golangci-lint run ./...

fmt:
	$(GO) fmt ./...

.PHONY: docker-build
docker-build:
	docker build -t every-sync:$(VERSION) .
