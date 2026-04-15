.PHONY: lint test generate deps docs docs-serve build

VERSION ?= dev
COMMIT  ?= $(shell git rev-parse --short HEAD)
LDFLAGS  = -X github.com/wblech/wmux/internal/platform/buildinfo.Version=$(VERSION) \
           -X github.com/wblech/wmux/internal/platform/buildinfo.Commit=$(COMMIT)

build:
	go build -ldflags "$(LDFLAGS)" -o wmux ./cmd/wmux
	go build -ldflags "$(LDFLAGS)" -o wmux-tmux ./cmd/wmux-tmux

lint:
	golangci-lint run && goframe check

test:
	go test -race -shuffle=on ./...

generate:
	go generate ./...

deps:
	go mod tidy

docs:
	mkdocs build --strict

docs-serve:
	mkdocs serve
