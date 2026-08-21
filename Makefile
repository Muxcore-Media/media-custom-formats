.PHONY: build test lint clean fmt tidy proto docker docker-push ci help trash-guides

GO ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
LDFLAGS ?= -s -w -X main.version=$(VERSION)
BINARY ?= media-custom-formats
TRASH_GUIDES_DIR ?= .cache/trash-guides

build:
	$(GO) build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd/module

test:
	$(GO) test -race -count=1 -timeout 60s ./...

lint:
	golangci-lint run --timeout 120s ./...

clean:
	rm -f $(BINARY)
	rm -f cmd/module/module
	rm -rf dist/

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

proto:
	PATH="$$HOME/go/bin:$$PATH" protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative proto/formatsv1/formats.proto

# Sparse-clone official TRaSH Guides JSON for offline FORMATS_TRASH_GUIDES_PATH sync.
trash-guides:
	@mkdir -p "$(dir $(TRASH_GUIDES_DIR))"
	@if [ -d "$(TRASH_GUIDES_DIR)/.git" ]; then \
		git -C "$(TRASH_GUIDES_DIR)" pull --ff-only; \
	else \
		rm -rf "$(TRASH_GUIDES_DIR)"; \
		git clone --depth 1 --filter=blob:none --sparse https://github.com/TRaSH-Guides/Guides.git "$(TRASH_GUIDES_DIR)"; \
		git -C "$(TRASH_GUIDES_DIR)" sparse-checkout set --no-cone 'docs/json/**' 'metadata.json'; \
	fi
	@echo "Set FORMATS_TRASH_GUIDES_PATH=$(CURDIR)/$(TRASH_GUIDES_DIR)"

ci: lint test build

help:
	@echo "Targets:"
	@echo "  build         - compile the module binary"
	@echo "  test          - run tests with race detection"
	@echo "  lint          - golangci-lint"
	@echo "  clean         - remove build artifacts"
	@echo "  fmt           - format Go source"
	@echo "  tidy          - go mod tidy"
	@echo "  proto         - regenerate protobuf code"
	@echo "  trash-guides  - sparse-clone TRaSH Guides into $(TRASH_GUIDES_DIR)"
	@echo "  ci            - lint + test + build"
