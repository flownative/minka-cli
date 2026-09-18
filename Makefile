VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X github.com/flownative/minka-cli/internal/cli.version=$(VERSION)

.PHONY: build install test check dist

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

# The archives of a release and their checksums, in dist/. VERSION is the
# version without a leading v, as it stands in the tag behind it.
DIST_TARGETS := darwin/arm64 darwin/amd64 linux/amd64

dist:
	rm -rf dist && mkdir -p dist
	@for target in $(DIST_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		name=minka_$(VERSION)_$${os}_$${arch}; \
		echo "building $$name"; \
		mkdir -p dist/$$name; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags "-s -w $(LDFLAGS)" -o dist/$$name/minka ./cmd/minka || exit 1; \
		cp README.md dist/$$name/; \
		tar -C dist -czf dist/$$name.tar.gz $$name || exit 1; \
		rm -r dist/$$name; \
	done
	cd dist && shasum -a 256 *.tar.gz > checksums.txt
