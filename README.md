# OpenRCT2 Configs

A curated set of configuration files, modernized keybinding presets, desktop integrations, and deployment scripts for **[OpenRCT2](https://openrct2.io/)** across Linux, macOS, and Windows.

---

## Highlights & Features

- **Modernized Camera & Controls**: Full **WASD** viewport panning, **Q/E** rotation, and **PageUp/PageDown** zooming.
- **Enhanced Visual Profile**: Uncapped FPS with VSync enabled, hardware-accelerated software drawing (`SOFTWARE_HWD`), 1.5x UI scaling for high-DPI displays, and landscape smoothing.
- **Desktop & Launcher Integration**: Automatic deployment of `.desktop` application menu entry, high-resolution app icon, and CLI symlink to `/opt/OpenRCT2/OpenRCT2.AppImage`.
- **Safety & Quality of Life**: Autosaves enabled, cheat menu hotkey (`CTRL+ALT+C`), quick transparency toggles, and staff auto-hiring shortcuts.

---

## Keybindings Cheat Sheet

| Category | Shortcut | Action |
| :--- | :--- | :--- |
| **Camera** | `W` / `A` / `S` / `D` | Pan camera Up / Left / Down / Right |
| **Camera** | `Q` / `E` | Rotate view Anti-clockwise / Clockwise |
| **Camera** | `PageDown` / `PageUp` | Zoom in / Zoom out |
| **Windows** | `Tab` | Park window |
| **Windows** | `F1` / `SHIFT+R` | New Ride construction / Rides overview |
| **Windows** | `F4` / `F5` / `F6` / `F7` | Scenery / Footpaths / Land / Water tools |
| **Windows** | `F` / `R` / `G` / `Z` / `M` | Finances / Research / Guests / Staff / Map |
| **Windows** | `CTRL+ALT+C` | Cheats menu |
| **View Modes** | `C` / `I` / `X` | Underground view / Hide supports / Path issues |
| **View Modes** | `B` / `N` / `V` | Transparent paths / Transparent rides / Transparent scenery |
| **Game Speed** | `0` / `1` / `2` | Pause game / Decrease speed / Increase speed |
| **System** | `CTRL+S` / `CTRL+L` | Save game / Load game |
| **System** | `F11` / `SHIFT+F11` | Screenshot / Giant screenshot |

---

## Installation & Deployment

### Linux & macOS

The `Makefile` automatically detects your operating system and selects the proper destination:
- **Linux Destinations**:
  - Configs: `~/.config/OpenRCT2/`
  - Desktop Entry: `~/.local/share/applications/OpenRCT2.desktop`
  - Icon: `~/.local/share/icons/OpenRCT2.png`
  - CLI Symlink: `~/.local/bin/openrct2`
- **macOS Destinations**:
  - Configs: `~/Library/Application Support/OpenRCT2/`

#### Deploy All (Configs + Desktop Integration)
To deploy configurations, desktop entries, and icon assets with automatic `.bak.<timestamp>` backups:

```bash
make install
```

Or install individual components:
- `make install-configs` — Deploys `config.ini` and `shortcuts.json`
- `make install-config` — Deploys only `config.ini`
- `make install-shortcuts` — Deploys only `shortcuts.json`
- `make install-desktop` — Deploys `OpenRCT2.desktop` (Linux only)
- `make install-icon` — Deploys `OpenRCT2.png` (Linux only)
- `make install-symlink` — Creates `~/.local/bin/openrct2` symlink to AppImage (Linux only)

#### Managing In-Game & Desktop Changes
- `make diff` — Compare repository configs and desktop entries against active system files.
- `make sync` — Pull changes made in-game or desktop entries back into the repository (automatically sanitizing personal paths/usernames).

#### Standalone Shell Script
Alternatively, deploy without `make` using the POSIX setup script:
```bash
./setup.sh
```

---

### Windows

To install the OpenRCT2 configurations on Windows via PowerShell:

```powershell
.\setup.ps1
```

This deploys `config.ini` and `shortcuts.json` to `%USERPROFILE%\Documents\OpenRCT2\`, creating timestamped backups beforehand.

If your OpenRCT2 user folder is in a custom path, supply the `-CustomTargetDir` parameter:
```powershell
.\setup.ps1 -CustomTargetDir "D:\Games\OpenRCT2User"
```

---

## Makefile Quick Reference

| Target | Description |
| :--- | :--- |
| `make install` | Deploys all configuration files, desktop entries, icons, and symlinks |
| `make install-configs` | Installs only `config/config.ini` and `config/shortcuts.json` |
| `make install-config` | Installs only `config/config.ini` |
| `make install-shortcuts` | Installs only `config/shortcuts.json` |
| `make install-desktop` | Installs `desktop/OpenRCT2.desktop` to `~/.local/share/applications/` |
| `make install-icon` | Installs `desktop/OpenRCT2.png` to `~/.local/share/icons/` |
| `make install-symlink` | Creates `~/.local/bin/openrct2` symlink directly to `/opt/OpenRCT2/OpenRCT2.AppImage` |
| `make diff` | Shows diff between repo configs/desktop entries and live system files |
| `make sync` | Syncs live system configs/desktop entries back into the git repository |
| `make check` | Runs validation suite (syntax, JSON validation, invariants, test install) |

---

## Game Files

> [!NOTE]
> OpenRCT2 requires the original RollerCoaster Tycoon 2 (and optionally RollerCoaster Tycoon 1) asset files. Set your installation paths inside `config.ini` under `[general]` (`game_path` and `rct1_path`).
