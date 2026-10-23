GO      ?= go
BIN     := weibo-rss
PREFIX  ?= $(HOME)/.local
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt check install clean run

all: check build

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/weibo-rss

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
