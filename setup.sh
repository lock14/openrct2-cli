#!/bin/bash
set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

OS="$(uname -s)"
if [ "$OS" = "Darwin" ]; then
    OPENRCT2_DIR="$HOME/Library/Application Support/OpenRCT2"
else
    OPENRCT2_DIR="$HOME/.config/OpenRCT2"
    DESKTOP_DIR="$HOME/.local/share/applications"
    ICON_DIR="$HOME/.local/share/icons"
    HICOLOR_ICON_DIR="$HOME/.local/share/icons/hicolor/256x256/apps"
    BIN_DIR="$HOME/.local/bin"
fi

APPIMAGE_PATH="/opt/OpenRCT2/OpenRCT2.AppImage"

echo "Setting up OpenRCT2 configurations for $OS..."

echo "Creating directory: $OPENRCT2_DIR"
mkdir -p "$OPENRCT2_DIR"

copy_with_idempotent_backup() {
    local src="$1"
    local dest="$2"
    local substitute_user="${3:-false}"

    local tmp_file
    tmp_file=$(mktemp)

    if [ "$substitute_user" = "true" ]; then
        sed -e "s|__HOME__|$HOME|g" -e "s|__USER__|$USER|g" "$src" > "$tmp_file"
    else
        sed "s|__HOME__|$HOME|g" "$src" > "$tmp_file"
    fi

    if [ -f "$dest" ]; then
        if ! cmp -s "$tmp_file" "$dest"; then
            local timestamp
            timestamp=$(date +%Y%m%d%H%M%S)
            echo "Backing up existing $(basename "$dest") to ${dest}.bak.${timestamp}"
            cp "$dest" "${dest}.bak.${timestamp}"
            echo "Installing $(basename "$dest")..."
            cp "$tmp_file" "$dest"
        else
            echo "$(basename "$dest") is already up to date."
        fi
    else
        echo "Installing $(basename "$dest")..."
        cp "$tmp_file" "$dest"
    fi

    rm -f "$tmp_file"
}

copy_with_idempotent_backup "$SCRIPT_DIR/config/config.ini" "$OPENRCT2_DIR/config.ini" "true"
copy_with_idempotent_backup "$SCRIPT_DIR/config/shortcuts.json" "$OPENRCT2_DIR/shortcuts.json" "false"

if [ "$OS" = "Linux" ]; then
    mkdir -p "$DESKTOP_DIR" "$ICON_DIR" "$HICOLOR_ICON_DIR" "$BIN_DIR"

    copy_with_idempotent_backup "$SCRIPT_DIR/desktop/OpenRCT2.desktop" "$DESKTOP_DIR/OpenRCT2.desktop" "false"
    chmod +x "$DESKTOP_DIR/OpenRCT2.desktop"

    if [ -f "$HICOLOR_ICON_DIR/openrct2.png" ]; then
        if ! cmp -s "$SCRIPT_DIR/desktop/openrct2.png" "$HICOLOR_ICON_DIR/openrct2.png"; then
            timestamp=$(date +%Y%m%d%H%M%S)
            cp "$HICOLOR_ICON_DIR/openrct2.png" "$HICOLOR_ICON_DIR/openrct2.png.bak.${timestamp}"
            cp "$SCRIPT_DIR/desktop/openrct2.png" "$HICOLOR_ICON_DIR/openrct2.png"
        fi
    else
        cp "$SCRIPT_DIR/desktop/openrct2.png" "$HICOLOR_ICON_DIR/openrct2.png"
    fi
    cp "$SCRIPT_DIR/desktop/openrct2.png" "$ICON_DIR/openrct2.png"
    cp "$SCRIPT_DIR/desktop/openrct2.png" "$ICON_DIR/OpenRCT2.png"

    if [ -e "$APPIMAGE_PATH" ]; then
        if [ -f "$BIN_DIR/openrct2" ] && [ ! -L "$BIN_DIR/openrct2" ]; then
            timestamp=$(date +%Y%m%d%H%M%S)
            echo "Backing up legacy binary wrapper to $BIN_DIR/openrct2.bak.${timestamp}"
            mv "$BIN_DIR/openrct2" "$BIN_DIR/openrct2.bak.${timestamp}"
        fi
        echo "Creating symlink $BIN_DIR/openrct2 -> $APPIMAGE_PATH"
        ln -sf "$APPIMAGE_PATH" "$BIN_DIR/openrct2"
    fi

    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$DESKTOP_DIR" 2>/dev/null || true
    fi
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        gtk-update-icon-cache -q -t "$HOME/.local/share/icons/hicolor" 2>/dev/null || true
    fi
fi

echo "OpenRCT2 setup complete!"
