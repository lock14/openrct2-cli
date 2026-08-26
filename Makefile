PREFIX ?= $(HOME)
USER_NAME ?= $(USER)
UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
    # macOS standard Application Support path
    OPENRCT2_DIR ?= $(PREFIX)/Library/Application Support/OpenRCT2
else
    # Linux standard XDG path
    OPENRCT2_DIR ?= $(PREFIX)/.config/OpenRCT2
endif

.PHONY: all install install-config install-shortcuts sync diff help check test

all: install

help:
	@echo "OpenRCT2 Configs - Available Makefile targets ($(UNAME_S)):"
	@echo "  make install           - Install all configurations with automatic backups"
	@echo "  make install-config    - Install only config.ini"
	@echo "  make install-shortcuts - Install only shortcuts.json"
	@echo "  make sync              - Copy active system configs back into repo (sanitizing personal paths)"
	@echo "  make diff              - Compare repo configs against installed system configs"
	@echo "  make check             - Run validation suite (syntax, invariants, dry install)"

install: install-config install-shortcuts
	@echo "All OpenRCT2 configurations successfully installed to $(OPENRCT2_DIR)!"

# Config installation targets
install-config:
	@mkdir -p "$(OPENRCT2_DIR)"
	@if [ -f "$(OPENRCT2_DIR)/config.ini" ]; then \
		echo "Backing up existing config.ini..."; \
		cp "$(OPENRCT2_DIR)/config.ini" "$(OPENRCT2_DIR)/config.ini.bak.$$(date +%Y%m%d%H%M%S)"; \
	fi
	@echo "Installing config/config.ini -> $(OPENRCT2_DIR)/config.ini"
	@sed -e "s|__HOME__|$(PREFIX)|g" -e "s|__USER__|$(USER_NAME)|g" config/config.ini > "$(OPENRCT2_DIR)/config.ini"

install-shortcuts:
	@mkdir -p "$(OPENRCT2_DIR)"
	@if [ -f "$(OPENRCT2_DIR)/shortcuts.json" ]; then \
		echo "Backing up existing shortcuts.json..."; \
		cp "$(OPENRCT2_DIR)/shortcuts.json" "$(OPENRCT2_DIR)/shortcuts.json.bak.$$(date +%Y%m%d%H%M%S)"; \
	fi
	@echo "Installing config/shortcuts.json -> $(OPENRCT2_DIR)/shortcuts.json"
	@cp config/shortcuts.json "$(OPENRCT2_DIR)/shortcuts.json"

# Copy live changes made in-game back into this repository
sync:
	@if [ -f "$(OPENRCT2_DIR)/config.ini" ]; then \
		echo "Syncing $(OPENRCT2_DIR)/config.ini -> config/config.ini"; \
		sed -e "s|$(PREFIX)|__HOME__|g" -e "s|player_name = \"$(USER_NAME)\"|player_name = \"__USER__\"|g" "$(OPENRCT2_DIR)/config.ini" > config/config.ini; \
	fi
	@if [ -f "$(OPENRCT2_DIR)/shortcuts.json" ]; then \
		echo "Syncing $(OPENRCT2_DIR)/shortcuts.json -> config/shortcuts.json"; \
		cp "$(OPENRCT2_DIR)/shortcuts.json" config/shortcuts.json; \
	fi

# Show differences between repo configs and installed system configs
diff:
	@echo "=== OpenRCT2 config.ini Diff ==="
	@-sed -e "s|__HOME__|$(PREFIX)|g" -e "s|__USER__|$(USER_NAME)|g" config/config.ini | diff -u - "$(OPENRCT2_DIR)/config.ini" || true
	@echo "=== OpenRCT2 shortcuts.json Diff ==="
	@-diff -u config/shortcuts.json "$(OPENRCT2_DIR)/shortcuts.json" || true

# Test & Validation targets
check: test

test:
	@echo "=== Validating Shell Scripts ==="
	@bash -n setup.sh
	@echo "=== Validating JSON Files ==="
	@jq . config/shortcuts.json > /dev/null
	@echo "=== Checking Path Invariants ==="
	@if grep -E '/home/[a-zA-Z0-9_-]+' config/config.ini config/shortcuts.json; then \
		echo "Error: Hardcoded personal path found in tracked configs"; \
		exit 1; \
	fi
	@echo "=== Testing Isolated Installation for $(UNAME_S) ==="
	@TEST_DIR=$$(mktemp -d); \
	trap 'rm -rf "$$TEST_DIR"' EXIT; \
	$(MAKE) PREFIX="$$TEST_DIR" install > /dev/null && \
	if [ "$(UNAME_S)" = "Darwin" ]; then \
		test -f "$$TEST_DIR/Library/Application Support/OpenRCT2/config.ini" && \
		test -f "$$TEST_DIR/Library/Application Support/OpenRCT2/shortcuts.json" && \
		! grep -q '__HOME__' "$$TEST_DIR/Library/Application Support/OpenRCT2/config.ini"; \
	else \
		test -f "$$TEST_DIR/.config/OpenRCT2/config.ini" && \
		test -f "$$TEST_DIR/.config/OpenRCT2/shortcuts.json" && \
		! grep -q '__HOME__' "$$TEST_DIR/.config/OpenRCT2/config.ini"; \
	fi && \
	$(MAKE) PREFIX="$$TEST_DIR" install > /dev/null && \
	if [ "$(UNAME_S)" = "Darwin" ]; then \
		ls "$$TEST_DIR/Library/Application Support/OpenRCT2/" | grep -q 'config.ini.bak.'; \
	else \
		ls "$$TEST_DIR/.config/OpenRCT2/" | grep -q 'config.ini.bak.'; \
	fi && \
	echo "All validation checks passed successfully for $(UNAME_S)!"
