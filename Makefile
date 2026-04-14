.PHONY: lint test generate deps docs docs-serve

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
