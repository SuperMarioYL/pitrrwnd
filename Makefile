VERSION := $(shell cat VERSION)
BIN     := pitr
GOFLAGS := -trimpath
LDFLAGS := -X main.version=$(VERSION)

.PHONY: all build test demo install clean tidy fmt vet

all: test build

## build: compile the pitr binary into ./bin
build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o bin/$(BIN) ./cmd/pitr

## install: install the pitr binary into $$GOBIN
install:
	go install $(GOFLAGS) -ldflags "$(LDFLAGS)" ./cmd/pitr

## test: run the test suite
test:
	go test ./...

## demo: render the end-to-end demo via vhs (requires vhs + a built binary)
demo: build
	vhs docs/demo.tape

## tidy: tidy + verify modules
tidy:
	go mod tidy
	go mod verify

## fmt/vet: format + lint
fmt:
	gofmt -s -w .

vet:
	go vet ./...

## clean: remove build artefacts
clean:
	rm -rf bin/
