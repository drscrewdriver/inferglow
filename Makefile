.PHONY: build build-sandbox build-seatbelt-loader test test-sandbox test-all vet lint clean

build:
	go build ./...
	find . -mindepth 2 -name go.mod -execdir go build ./... \;

build-sandbox:
	go build -tags with_sandbox ./...
	find . -mindepth 2 -name go.mod -execdir go build -tags with_sandbox ./... \;

# Build the macOS seatbelt-loader binary (macOS only; the package is a stub
# elsewhere). It must be shipped next to the inferglow binary or on PATH,
# see sandbox/README.md.
build-seatbelt-loader:
	@if [ "$$(go env GOOS)" != "darwin" ]; then echo "error: build-seatbelt-loader is macOS-only"; exit 1; fi
	mkdir -p bin
	cd sandbox && go build -o ../bin/seatbelt-loader ./seatbelt_loader

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
