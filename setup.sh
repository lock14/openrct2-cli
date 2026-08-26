#!/bin/bash
set -e

# Get the directory where this script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

OS="$(uname -s)"
if [ "$OS" = "Darwin" ]; then
    OPENRCT2_DIR="$HOME/Library/Application Support/OpenRCT2"
else
    OPENRCT2_DIR="$HOME/.config/OpenRCT2"
fi

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

echo "OpenRCT2 setup complete!"
