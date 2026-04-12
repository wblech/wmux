.PHONY: lint test generate deps

lint:
	golangci-lint run && goframe check

test:
	go test -race -shuffle=on ./...

generate:
	go generate ./...

deps:
	go mod tidy
