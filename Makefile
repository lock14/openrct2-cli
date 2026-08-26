PREFIX ?= $(HOME)
USER_NAME ?= $(USER)
UNAME_S := $(shell uname -s)

ifeq ($(UNAME_S),Darwin)
    # macOS standard Application Support path
    OPENRCT2_DIR ?= $(PREFIX)/Library/Application Support/OpenRCT2
else
    # Linux standard XDG paths
    OPENRCT2_DIR     ?= $(PREFIX)/.config/OpenRCT2
    DESKTOP_DIR      ?= $(PREFIX)/.local/share/applications
    ICON_DIR         ?= $(PREFIX)/.local/share/icons
    HICOLOR_ICON_DIR ?= $(PREFIX)/.local/share/icons/hicolor/256x256/apps
    BIN_DIR          ?= $(PREFIX)/.local/bin
endif

APPIMAGE_PATH ?= /opt/OpenRCT2/OpenRCT2.AppImage

.PHONY: all install install-configs install-config install-shortcuts \
        install-desktop install-icon install-symlink uninstall \
        sync diff help check test

all: install

help:
	@echo "OpenRCT2 Configs - Available Makefile targets ($(UNAME_S)):"
	@echo "  make install           - Install all configurations, desktop entries, and icons"
	@echo "  make install-configs   - Install only config.ini and shortcuts.json"
	@echo "  make install-config    - Install only config.ini"
	@echo "  make install-shortcuts - Install only shortcuts.json"
	@echo "  make install-desktop   - Install OpenRCT2.desktop (Linux only)"
	@echo "  make install-icon      - Install icons into standard XDG & hicolor paths (Linux only)"
	@echo "  make install-symlink   - Create ~/.local/bin/openrct2 symlink to AppImage (Linux only)"
	@echo "  make uninstall         - Remove deployed configs, desktop entries, icons, and symlinks"
	@echo "  make sync              - Copy active system configs back into repo (sanitizing personal paths)"
	@echo "  make diff              - Compare repo configs against installed system configs"
	@echo "  make check             - Run validation suite (syntax, invariants, idempotent install, uninstall)"

ifeq ($(UNAME_S),Darwin)
install: install-configs
else
install: install-configs install-desktop install-icon install-symlink
	@if command -v update-desktop-database >/dev/null 2>&1; then \
		update-desktop-database "$(DESKTOP_DIR)" 2>/dev/null || true; \
	fi
	@if command -v gtk-update-icon-cache >/dev/null 2>&1; then \
		gtk-update-icon-cache -q -t "$(PREFIX)/.local/share/icons/hicolor" 2>/dev/null || true; \
	fi
endif
	@echo "All OpenRCT2 configurations successfully installed!"

install-configs: install-config install-shortcuts

# Config installation targets with idempotent backup checks
install-config:
	@mkdir -p "$(OPENRCT2_DIR)"
	@TMP_FILE=$$(mktemp); \
	sed -e "s|__HOME__|$(PREFIX)|g" -e "s|__USER__|$(USER_NAME)|g" config/config.ini > "$$TMP_FILE"; \
	if [ -f "$(OPENRCT2_DIR)/config.ini" ]; then \
		if ! cmp -s "$$TMP_FILE" "$(OPENRCT2_DIR)/config.ini"; then \
			echo "Backing up existing config.ini..."; \
			cp "$(OPENRCT2_DIR)/config.ini" "$(OPENRCT2_DIR)/config.ini.bak.$$(date +%Y%m%d%H%M%S)"; \
			echo "Installing config/config.ini -> $(OPENRCT2_DIR)/config.ini"; \
			cp "$$TMP_FILE" "$(OPENRCT2_DIR)/config.ini"; \
		else \
			echo "config.ini is already up to date."; \
		fi; \
	else \
		echo "Installing config/config.ini -> $(OPENRCT2_DIR)/config.ini"; \
		cp "$$TMP_FILE" "$(OPENRCT2_DIR)/config.ini"; \
	fi; \
	rm -f "$$TMP_FILE"

install-shortcuts:
	@mkdir -p "$(OPENRCT2_DIR)"
	@if [ -f "$(OPENRCT2_DIR)/shortcuts.json" ]; then \
		if ! cmp -s config/shortcuts.json "$(OPENRCT2_DIR)/shortcuts.json"; then \
			echo "Backing up existing shortcuts.json..."; \
			cp "$(OPENRCT2_DIR)/shortcuts.json" "$(OPENRCT2_DIR)/shortcuts.json.bak.$$(date +%Y%m%d%H%M%S)"; \
			echo "Installing config/shortcuts.json -> $(OPENRCT2_DIR)/shortcuts.json"; \
			cp config/shortcuts.json "$(OPENRCT2_DIR)/shortcuts.json"; \
		else \
			echo "shortcuts.json is already up to date."; \
		fi; \
	else \
		echo "Installing config/shortcuts.json -> $(OPENRCT2_DIR)/shortcuts.json"; \
		cp config/shortcuts.json "$(OPENRCT2_DIR)/shortcuts.json"; \
	fi

install-desktop:
ifeq ($(UNAME_S),Linux)
	@mkdir -p "$(DESKTOP_DIR)"
	@if [ -f "$(DESKTOP_DIR)/OpenRCT2.desktop" ]; then \
		if ! cmp -s desktop/OpenRCT2.desktop "$(DESKTOP_DIR)/OpenRCT2.desktop"; then \
			echo "Backing up existing OpenRCT2.desktop..."; \
			cp "$(DESKTOP_DIR)/OpenRCT2.desktop" "$(DESKTOP_DIR)/OpenRCT2.desktop.bak.$$(date +%Y%m%d%H%M%S)"; \
			echo "Installing desktop/OpenRCT2.desktop -> $(DESKTOP_DIR)/OpenRCT2.desktop"; \
			cp desktop/OpenRCT2.desktop "$(DESKTOP_DIR)/OpenRCT2.desktop"; \
		else \
			echo "OpenRCT2.desktop is already up to date."; \
		fi; \
	else \
		echo "Installing desktop/OpenRCT2.desktop -> $(DESKTOP_DIR)/OpenRCT2.desktop"; \
		cp desktop/OpenRCT2.desktop "$(DESKTOP_DIR)/OpenRCT2.desktop"; \
	fi
	@chmod +x "$(DESKTOP_DIR)/OpenRCT2.desktop"
endif

install-icon:
ifeq ($(UNAME_S),Linux)
	@mkdir -p "$(ICON_DIR)" "$(HICOLOR_ICON_DIR)"
	@if [ -f "$(HICOLOR_ICON_DIR)/openrct2.png" ]; then \
		if ! cmp -s desktop/openrct2.png "$(HICOLOR_ICON_DIR)/openrct2.png"; then \
			echo "Backing up existing hicolor icon..."; \
			cp "$(HICOLOR_ICON_DIR)/openrct2.png" "$(HICOLOR_ICON_DIR)/openrct2.png.bak.$$(date +%Y%m%d%H%M%S)"; \
			cp desktop/openrct2.png "$(HICOLOR_ICON_DIR)/openrct2.png"; \
		fi; \
	else \
		echo "Installing desktop/openrct2.png -> $(HICOLOR_ICON_DIR)/openrct2.png"; \
		cp desktop/openrct2.png "$(HICOLOR_ICON_DIR)/openrct2.png"; \
	fi
	@cp desktop/openrct2.png "$(ICON_DIR)/openrct2.png"
	@cp desktop/openrct2.png "$(ICON_DIR)/OpenRCT2.png"
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

# Clean uninstallation
uninstall:
	@echo "Uninstalling OpenRCT2 configurations and integrations..."
	@rm -f "$(OPENRCT2_DIR)/config.ini" "$(OPENRCT2_DIR)/shortcuts.json"
ifeq ($(UNAME_S),Linux)
	@rm -f "$(DESKTOP_DIR)/OpenRCT2.desktop"
	@rm -f "$(ICON_DIR)/openrct2.png" "$(ICON_DIR)/OpenRCT2.png" "$(HICOLOR_ICON_DIR)/openrct2.png"
	@if [ -L "$(BIN_DIR)/openrct2" ]; then rm -f "$(BIN_DIR)/openrct2"; fi
	@if command -v update-desktop-database >/dev/null 2>&1; then \
		update-desktop-database "$(DESKTOP_DIR)" 2>/dev/null || true; \
	fi
endif
	@echo "Uninstallation complete!"

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
		cp "$(DESKTOP_DIR)/OpenRCT2.desktop" desktop/OpenRCT2.desktop; \
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
	@-diff -u desktop/OpenRCT2.desktop "$(DESKTOP_DIR)/OpenRCT2.desktop" || true
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
	@echo "=== Testing Isolated Installation & Idempotence for $(UNAME_S) ==="
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
		test -f "$$TEST_DIR/.local/share/icons/openrct2.png" && \
		test -f "$$TEST_DIR/.local/share/icons/hicolor/256x256/apps/openrct2.png" && \
		! grep -q '__HOME__' "$$TEST_DIR/.config/OpenRCT2/config.ini"; \
	fi && \
	$(MAKE) PREFIX="$$TEST_DIR" install > /dev/null && \
	if [ "$(UNAME_S)" = "Darwin" ]; then \
		! ls "$$TEST_DIR/Library/Application Support/OpenRCT2/" | grep -q 'config.ini.bak.'; \
	else \
		! ls "$$TEST_DIR/.config/OpenRCT2/" | grep -q 'config.ini.bak.'; \
	fi && \
	echo "=== Testing Uninstallation ===" && \
	$(MAKE) PREFIX="$$TEST_DIR" uninstall > /dev/null && \
	if [ "$(UNAME_S)" = "Darwin" ]; then \
		! test -f "$$TEST_DIR/Library/Application Support/OpenRCT2/config.ini"; \
	else \
		! test -f "$$TEST_DIR/.config/OpenRCT2/config.ini" && \
		! test -f "$$TEST_DIR/.local/share/applications/OpenRCT2.desktop"; \
	fi && \
	echo "All validation checks passed successfully for $(UNAME_S)!"
