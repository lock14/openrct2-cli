PREFIX ?= $(HOME)
USER_NAME ?= $(USER)
UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
    # macOS standard Application Support path
    OPENRCT2_DIR ?= $(PREFIX)/Library/Application Support/OpenRCT2
else
    # Linux standard XDG paths
    OPENRCT2_DIR ?= $(PREFIX)/.config/OpenRCT2
    DESKTOP_DIR  ?= $(PREFIX)/.local/share/applications
    ICON_DIR     ?= $(PREFIX)/.local/share/icons
    BIN_DIR      ?= $(PREFIX)/.local/bin
endif

APPIMAGE_PATH ?= /opt/OpenRCT2/OpenRCT2.AppImage

.PHONY: all install install-configs install-config install-shortcuts \
        install-desktop install-icon install-symlink sync diff help check test

all: install

help:
	@echo "OpenRCT2 Configs - Available Makefile targets ($(UNAME_S)):"
	@echo "  make install           - Install all configurations, desktop entries, and icons"
	@echo "  make install-configs   - Install only config.ini and shortcuts.json"
	@echo "  make install-config    - Install only config.ini"
	@echo "  make install-shortcuts - Install only shortcuts.json"
	@echo "  make install-desktop   - Install OpenRCT2.desktop (Linux only)"
	@echo "  make install-icon      - Install OpenRCT2.png icon (Linux only)"
	@echo "  make install-symlink   - Create ~/.local/bin/openrct2 symlink to AppImage (Linux only)"
	@echo "  make sync              - Copy active system configs back into repo (sanitizing personal paths)"
	@echo "  make diff              - Compare repo configs against installed system configs"
	@echo "  make check             - Run validation suite (syntax, invariants, dry install)"

ifeq ($(UNAME_S),Darwin)
install: install-configs
else
install: install-configs install-desktop install-icon install-symlink
endif
	@echo "All OpenRCT2 configurations successfully installed!"

install-configs: install-config install-shortcuts

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

install-desktop:
ifeq ($(UNAME_S),Linux)
	@mkdir -p "$(DESKTOP_DIR)"
	@if [ -f "$(DESKTOP_DIR)/OpenRCT2.desktop" ]; then \
		echo "Backing up existing OpenRCT2.desktop..."; \
		cp "$(DESKTOP_DIR)/OpenRCT2.desktop" "$(DESKTOP_DIR)/OpenRCT2.desktop.bak.$$(date +%Y%m%d%H%M%S)"; \
	fi
	@echo "Installing desktop/OpenRCT2.desktop -> $(DESKTOP_DIR)/OpenRCT2.desktop"
	@sed "s|__HOME__|$(PREFIX)|g" desktop/OpenRCT2.desktop > "$(DESKTOP_DIR)/OpenRCT2.desktop"
	@chmod +x "$(DESKTOP_DIR)/OpenRCT2.desktop"
endif

install-icon:
ifeq ($(UNAME_S),Linux)
	@mkdir -p "$(ICON_DIR)"
	@if [ -f "$(ICON_DIR)/OpenRCT2.png" ]; then \
		echo "Backing up existing OpenRCT2.png..."; \
		cp "$(ICON_DIR)/OpenRCT2.png" "$(ICON_DIR)/OpenRCT2.png.bak.$$(date +%Y%m%d%H%M%S)"; \
	fi
	@echo "Installing desktop/OpenRCT2.png -> $(ICON_DIR)/OpenRCT2.png"
	@cp desktop/OpenRCT2.png "$(ICON_DIR)/OpenRCT2.png"
endif

install-symlink:
ifeq ($(UNAME_S),Linux)
	@mkdir -p "$(BIN_DIR)"
	@if [ -e "$(APPIMAGE_PATH)" ]; then \
		if [ -f "$(BIN_DIR)/openrct2" ] && [ ! -L "$(BIN_DIR)/openrct2" ]; then \
			echo "Backing up legacy binary wrapper $(BIN_DIR)/openrct2..."; \
			mv "$(BIN_DIR)/openrct2" "$(BIN_DIR)/openrct2.bak.$$(date +%Y%m%d%H%M%S)"; \
		fi; \
		echo "Creating symlink $(BIN_DIR)/openrct2 -> $(APPIMAGE_PATH)"; \
		ln -sf "$(APPIMAGE_PATH)" "$(BIN_DIR)/openrct2"; \
	fi
endif

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
ifeq ($(UNAME_S),Linux)
	@if [ -f "$(DESKTOP_DIR)/OpenRCT2.desktop" ]; then \
		echo "Syncing $(DESKTOP_DIR)/OpenRCT2.desktop -> desktop/OpenRCT2.desktop"; \
		sed "s|$(PREFIX)|__HOME__|g" "$(DESKTOP_DIR)/OpenRCT2.desktop" > desktop/OpenRCT2.desktop; \
	fi
endif

# Show differences between repo configs and installed system configs
diff:
	@echo "=== OpenRCT2 config.ini Diff ==="
	@-sed -e "s|__HOME__|$(PREFIX)|g" -e "s|__USER__|$(USER_NAME)|g" config/config.ini | diff -u - "$(OPENRCT2_DIR)/config.ini" || true
	@echo "=== OpenRCT2 shortcuts.json Diff ==="
	@-diff -u config/shortcuts.json "$(OPENRCT2_DIR)/shortcuts.json" || true
ifeq ($(UNAME_S),Linux)
	@echo "=== OpenRCT2.desktop Diff ==="
	@-sed "s|__HOME__|$(PREFIX)|g" desktop/OpenRCT2.desktop | diff -u - "$(DESKTOP_DIR)/OpenRCT2.desktop" || true
endif

# Test & Validation targets
check: test

test:
	@echo "=== Validating Shell Scripts ==="
	@bash -n setup.sh
	@echo "=== Validating JSON Files ==="
	@jq . config/shortcuts.json > /dev/null
	@echo "=== Checking Path Invariants ==="
	@if grep -E '/home/[a-zA-Z0-9_-]+' config/config.ini config/shortcuts.json desktop/OpenRCT2.desktop; then \
		echo "Error: Hardcoded personal path found in tracked files"; \
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
		test -f "$$TEST_DIR/.local/share/applications/OpenRCT2.desktop" && \
		test -f "$$TEST_DIR/.local/share/icons/OpenRCT2.png" && \
		! grep -q '__HOME__' "$$TEST_DIR/.config/OpenRCT2/config.ini" && \
		! grep -q '__HOME__' "$$TEST_DIR/.local/share/applications/OpenRCT2.desktop"; \
	fi && \
	$(MAKE) PREFIX="$$TEST_DIR" install > /dev/null && \
	if [ "$(UNAME_S)" = "Darwin" ]; then \
		ls "$$TEST_DIR/Library/Application Support/OpenRCT2/" | grep -q 'config.ini.bak.'; \
	else \
		ls "$$TEST_DIR/.config/OpenRCT2/" | grep -q 'config.ini.bak.' && \
		ls "$$TEST_DIR/.local/share/applications/" | grep -q 'OpenRCT2.desktop.bak.'; \
	fi && \
	echo "All validation checks passed successfully for $(UNAME_S)!"
