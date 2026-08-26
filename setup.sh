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
    BIN_DIR="$HOME/.local/bin"
fi

APPIMAGE_PATH="/opt/OpenRCT2/OpenRCT2.AppImage"

echo "Setting up OpenRCT2 configurations for $OS..."

echo "Creating directory: $OPENRCT2_DIR"
mkdir -p "$OPENRCT2_DIR"

copy_with_backup() {
    local src="$1"
    local dest="$2"
    local substitute_user="${3:-false}"

    if [ -f "$dest" ]; then
        local timestamp
        timestamp=$(date +%Y%m%d%H%M%S)
        echo "Backing up existing $(basename "$dest") to ${dest}.bak.${timestamp}"
        cp "$dest" "${dest}.bak.${timestamp}"
    fi

    echo "Installing $(basename "$dest")..."
    if [ "$substitute_user" = "true" ]; then
        sed -e "s|__HOME__|$HOME|g" -e "s|__USER__|$USER|g" "$src" > "$dest"
    else
        sed "s|__HOME__|$HOME|g" "$src" > "$dest"
    fi
}

copy_with_backup "$SCRIPT_DIR/config/config.ini" "$OPENRCT2_DIR/config.ini" "true"
copy_with_backup "$SCRIPT_DIR/config/shortcuts.json" "$OPENRCT2_DIR/shortcuts.json" "false"

if [ "$OS" = "Linux" ]; then
    mkdir -p "$DESKTOP_DIR"
    mkdir -p "$ICON_DIR"
    mkdir -p "$BIN_DIR"

    copy_with_backup "$SCRIPT_DIR/desktop/OpenRCT2.desktop" "$DESKTOP_DIR/OpenRCT2.desktop" "false"
    chmod +x "$DESKTOP_DIR/OpenRCT2.desktop"

    if [ -f "$ICON_DIR/OpenRCT2.png" ]; then
        timestamp=$(date +%Y%m%d%H%M%S)
        cp "$ICON_DIR/OpenRCT2.png" "$ICON_DIR/OpenRCT2.png.bak.${timestamp}"
    fi
    cp "$SCRIPT_DIR/desktop/OpenRCT2.png" "$ICON_DIR/OpenRCT2.png"

    if [ -e "$APPIMAGE_PATH" ]; then
        if [ -f "$BIN_DIR/openrct2" ] && [ ! -L "$BIN_DIR/openrct2" ]; then
            timestamp=$(date +%Y%m%d%H%M%S)
            echo "Backing up legacy binary wrapper to $BIN_DIR/openrct2.bak.${timestamp}"
            mv "$BIN_DIR/openrct2" "$BIN_DIR/openrct2.bak.${timestamp}"
        fi
        echo "Creating symlink $BIN_DIR/openrct2 -> $APPIMAGE_PATH"
        ln -sf "$APPIMAGE_PATH" "$BIN_DIR/openrct2"
    fi
fi

echo "OpenRCT2 setup complete!"
