BIN ?= bin/rct
PREFIX ?= $(HOME)/.local/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS ?= -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build install test bench check lint format tidy clean help format-check tidy-check

all: build

help:
	@echo "openrct2-cli developer targets:"
	@echo "  make build    - Compile bin/rct static binary"
	@echo "  make install  - Install binary to $(PREFIX)/rct"
	@echo "  make test     - Run full Go test suite with -race and -shuffle=on"
	@echo "  make bench    - Run performance benchmarks with allocation tracking"
	@echo "  make lint     - Run go vet, revive, and desktop-file-validate"
	@echo "  make format   - Run gofmt -s to format all Go source files"
	@echo "  make tidy     - Tidy and verify go.mod / go.sum"
	@echo "  make check    - Run formatting, module hygiene, lint, tests, and path audit"
	@echo "  make clean    - Remove build artifacts"
	@echo ""
	@echo "For OpenRCT2 setup and playing, use the CLI:"
	@echo "  rct setup     - Full automated setup (RCT1/RCT2 scan, configs, engine, asset packs, plugins)"
	@echo "  rct           - Launch OpenRCT2 directly (or 'rct <park-or-save>')"
	@echo "  rct addons    - Manage official audio/music packs and JavaScript plugins"
	@echo "  rct --help    - View all available commands and flags"

build:
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/rct

install: build
	@mkdir -p $(PREFIX)
	cp $(BIN) $(PREFIX)/rct
	@echo "✓ Successfully installed rct to $(PREFIX)/rct"

format:
	@echo "Formatting Go source files..."
	@gofmt -s -w .

tidy:
	@echo "Verifying module hygiene..."
	@go mod verify
	@go mod download

lint:
	@echo "Running go vet..."
	@go vet ./...
	@if command -v revive >/dev/null 2>&1; then \
		echo "Running revive..."; \
		revive -config revive.toml -formatter friendly -set_exit_status ./...; \
	fi
	@if command -v desktop-file-validate >/dev/null 2>&1; then \
		echo "Validating FreeDesktop launcher..."; \
		desktop-file-validate configs/desktop/OpenRCT2.desktop; \
	fi

test:
	go test -v -race -shuffle=on ./...

bench:
	go test -run='^$$' -bench=. -benchmem ./...

check: format-check tidy-check lint test
	@echo "Auditing path invariants..."
	@! grep -rE '/home/[a-zA-Z0-9_-]+' internal/ catalog/ configs/ cmd/
	@echo "✓ All checks, linters, and tests passed!"

format-check:
	@echo "Checking formatting..."
	@if [ -n "$$(gofmt -s -l .)" ]; then \
		echo "The following files are not formatted properly:"; \
		gofmt -s -l .; \
		echo "Run 'make format' or 'gofmt -s -w .' locally."; \
		exit 1; \
	fi

tidy-check:
	@echo "Checking go mod tidy..."
	@go mod verify
	@go mod tidy
	@git diff --exit-code go.mod go.sum

clean:
	rm -rf bin/
