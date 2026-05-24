.PHONY: build build-sandbox test test-sandbox test-all vet lint clean

build:
	go build ./...
	find . -mindepth 2 -name go.mod -execdir go build ./... \;

build-sandbox:
	go build -tags with_sandbox ./...
	find . -mindepth 2 -name go.mod -execdir go build -tags with_sandbox ./... \;

test:
	go test ./...
	find . -mindepth 2 -name go.mod -execdir go test ./... \;

test-sandbox:
	go test -tags with_sandbox ./...
	find . -mindepth 2 -name go.mod -execdir go test -tags with_sandbox ./... \;

test-all: test test-sandbox

vet:
	go vet ./...
	find . -mindepth 2 -name go.mod -execdir go vet ./... \;

lint:
	golangci-lint run ./...
	find . -mindepth 2 -name go.mod -execdir golangci-lint run ./... \;

clean:
	go clean -testcache
	go clean -cache
