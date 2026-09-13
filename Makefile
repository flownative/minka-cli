VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/flownative/minka-cli/internal/cli.version=$(VERSION)

.PHONY: build install test check

build:
	go build -ldflags "$(LDFLAGS)" -o bin/minka ./cmd/minka

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/minka

test:
	go test ./...

check:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...
	go test ./...
