GO      ?= go
BIN     := weibo-rss-pro
PREFIX  ?= $(HOME)/.local
# CGO off keeps the binary static (pure-Go resolver), so it runs on any distro.
CGO_ENABLED ?= 0
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt check install clean run

all: check build

build:
	CGO_ENABLED=$(CGO_ENABLED) $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/weibo-rss-pro

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	gofmt -l -w .

check: vet test
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

install: build
	install -m 0755 $(BIN) $(PREFIX)/bin/$(BIN)

run: build
	./$(BIN) serve --listen 127.0.0.1:3000

clean:
	rm -f $(BIN)
