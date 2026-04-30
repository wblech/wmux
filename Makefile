.PHONY: lint test bench generate deps docs docs-serve build

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

# bench runs the broadcast hot-path benchmarks documented in ADR-0031.
# Scoped to session+daemon packages where the perf-tracked benchmarks live.
bench:
	go test -bench=. -benchmem -benchtime=2s -run='^$$' \
	  ./internal/session/ ./internal/daemon/

generate:
	go generate ./...

deps:
	go mod tidy

docs:
	mkdocs build --strict

docs-serve:
	mkdocs serve
