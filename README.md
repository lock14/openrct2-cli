# openrct2-cli

[![Go Version](https://img.shields.io/github/go-mod/go-version/lock14/openrct2-cli)](https://go.dev/)
[![CI](https://img.shields.io/github/actions/workflow/status/lock14/openrct2-cli/ci.yml?branch=main&label=CI)](https://github.com/lock14/openrct2-cli/actions/workflows/ci.yml)
[![Benchmarks](https://img.shields.io/github/actions/workflow/status/lock14/openrct2-cli/benchmark.yml?branch=main&label=Benchmarks)](https://github.com/lock14/openrct2-cli/actions/workflows/benchmark.yml)
[![Security](https://img.shields.io/github/actions/workflow/status/lock14/openrct2-cli/security.yml?branch=main&label=Security)](https://github.com/lock14/openrct2-cli/actions/workflows/security.yml)
[![Coverage](https://img.shields.io/codecov/c/github/lock14/openrct2-cli)](https://codecov.io/gh/lock14/openrct2-cli)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A unified, cross-platform CLI tool (`rct`) and curated collection of configurations, WASD keybindings, engine lifecycle
tools, official audio/music packs, and community JavaScript plugins for [OpenRCT2](https://openrct2.io/) across Linux,
macOS, and Windows.

## Table of contents

- [Quick start](#quick-start)
- [Installation](#installation)
  - [Linux and macOS](#linux-and-macos)
  - [Windows](#windows)
- [Automated setup (`rct setup`)](#automated-setup-rct-setup)
- [CLI command reference](#cli-command-reference)
  - [Core commands](#core-commands)
  - [Engine lifecycle and channels](#engine-lifecycle-and-channels)
  - [Base game asset discovery (`rct games`)](#base-game-asset-discovery-rct-games)
  - [Asset packs and plugins (`rct addons`)](#asset-packs-and-plugins-rct-addons)
  - [Configuration management (`rct config`)](#configuration-management-rct-config)
  - [Shell autocompletion](#shell-autocompletion)
- [Keybindings cheat sheet](#keybindings-cheat-sheet)
- [Engine profile and visual defaults](#engine-profile-and-visual-defaults)
- [Curated asset packs and plugins](#curated-asset-packs-and-plugins)
- [Extensibility](#extensibility)
  - [CLI settings (`config.json`)](#cli-settings-configjson)
  - [Add custom plugins and asset packs (`catalog.d/*.json`)](#add-custom-plugins-and-asset-packs-catalogdjson)
- [Environment variables](#environment-variables)
- [Directory layouts](#directory-layouts)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

## Quick start

Get from zero to playing OpenRCT2 in three commands:

```bash
# 1. Install rct CLI to ~/.local/bin/rct
make install

# 2. Run turnkey setup (discovers Steam/GOG RCT1+2, installs configs, OpenRCT2 engine, asset packs, and plugins)
rct setup

# 3. Launch OpenRCT2 directly (or pass a .park / .sv6 file)
rct
```

## Installation

> [!NOTE]
> The CLI binary is named `rct` so it never collides with upstream's `openrct2` GUI binary or `openrct2-cli` headless
> server executable on your `$PATH`.

### Linux and macOS

```bash
# Clone the repository
git clone https://github.com/lock14/openrct2-cli.git
cd openrct2-cli

# Compile and install the static rct binary to ~/.local/bin/rct
make install
```

Alternatively, install directly with Go:

```bash
go install github.com/lock14/openrct2-cli/cmd/rct@latest
```

> [!TIP]
> Ensure `~/.local/bin` is in your `$PATH` (for example, in `~/.bashrc` or `~/.zshrc`):
> ```bash
> export PATH="$HOME/.local/bin:$PATH"
> ```

### Windows

In PowerShell or Windows Terminal:

```powershell
# Clone the repository
git clone https://github.com/lock14/openrct2-cli.git
cd openrct2-cli

# Compile rct.exe to your Go bin path
go install ./cmd/rct

# Or compile directly to the standard Windows per-user application binary folder:
go build -o "$env:LOCALAPPDATA\Programs\OpenRCT2\bin\rct.exe" ./cmd/rct
```

## Automated setup (`rct setup`)

Configure base game paths, display-tuned templates, the OpenRCT2 engine, audio packs, and plugins in a single step (or
pass `rct setup --force` to re-download and overwrite existing files):

```bash
rct setup
```

`rct setup` performs five automated steps:

1. **Discovers official RCT2 and RCT1 installations**: Scans Steam (`libraryfolders.vdf`), GOG, Wine prefixes, and local
   game folders across drives for *RollerCoaster Tycoon 2* (*Triple Thrill Pack*), *RollerCoaster Tycoon Classic*, and
   *RollerCoaster Tycoon Deluxe*, validating `Data/g1.dat` and `Data/csg1.dat`.
2. **Deploys OpenRCT2 configurations and desktop integration**: Installs curated `config.ini` and WASD `shortcuts.json`
   files, auto-detecting your primary monitor resolution and HiDPI `window_scale` while preserving existing user
   `config.ini` tweaks unless `--force` is passed. On Linux, installs a FreeDesktop compliant `OpenRCT2.desktop`
   launcher (`PrefersNonDefaultGPU=true`, `openrct2://` URI handler) and `hicolor` icons.
3. **Installs portable OpenRCT2 in user-space**: Downloads and SHA-256-verifies the latest portable OpenRCT2 build
   (`.AppImage` on Linux, universal `OpenRCT2.app` on macOS, portable `.zip` bundle on Windows) without requiring `sudo`
   or `/opt` permissions. Pass `--channel develop` to install continuous development builds instead of stable releases.
4. **Installs official OpenRCT2 asset packs**: Downloads and extracts verified `OpenMusic` (`v1.6.1`),
   `OpenSoundEffects` (`v1.0.6`), and official `title-sequences` (`v0.5.6`).
5. **Installs curated JavaScript plugins**: Downloads and verifies top community OpenRCT2 plugins into your `plugin/`
   directory.

## CLI command reference

### Core commands

| Command | Description |
| :--- | :--- |
| `rct [file\|uri] [flags]` | Launches OpenRCT2 directly (forwarding extra engine flags) |
| `rct --headless <park> [flags]` | Launches OpenRCT2 in headless server mode |
| `rct --dry-run [file] [flags]` | Prints the synthesized OpenRCT2 command without launching |
| `rct setup` | Runs 5-step turnkey setup (Steam/GOG scan, configs, engine, asset packs, plugins) |
| `rct setup --force` | Re-downloads and overwrites existing engine, configurations, and add-ons |
| `rct setup --channel develop` | Runs turnkey setup targeting the continuous `develop` build channel |

### Engine lifecycle and channels

| Command | Description |
| :--- | :--- |
| `rct engine` | Displays installed OpenRCT2 binary status, receipt tag, and active channel |
| `rct engine status` | Displays installed OpenRCT2 binary status, receipt tag, and active channel |
| `rct engine install` | Downloads and installs portable OpenRCT2 for the active channel |
| `rct engine install --channel develop` | Installs the latest `develop` build from `OpenRCT2/OpenRCT2-binaries` |
| `rct engine update` | Compares local receipt against GitHub Releases and updates only when a newer build exists |
| `rct engine channel [release\|develop]` | Displays or switches the active release channel (pass `--install` to apply immediately) |

### Base game asset discovery (`rct games`)

| Command | Description |
| :--- | :--- |
| `rct games` | Displays configured `game_path` (RCT2) and `rct1_path` (RCT1) with asset validation status |
| `rct games status` | Displays configured `game_path` (RCT2) and `rct1_path` (RCT1) with asset validation status |
| `rct games scan` | Scans Steam, GOG, Wine, and local libraries and links discovered RCT2/RCT1 paths in `config.ini` |
| `rct games scan --dry-run` | Reports discovered RCT2 and RCT1 installations without modifying `config.ini` |
| `rct games set --rct2 <dir> [--rct1 <dir>]` | Validates and sets custom RCT2 and/or RCT1 directories |

### Asset packs and plugins (`rct addons`)

`rct plugins` is available as a full alias for `rct addons`:

| Command | Description |
| :--- | :--- |
| `rct addons` | Lists all curated and custom asset packs and plugins with `[✓ Installed]` / `[✗ Missing]` status |
| `rct addons list [--type plugin\|assetpack]` | Lists add-ons filtered by type |
| `rct addons show <id>` | Displays metadata, homepage, download URL, SHA-256 digest, and target paths (accepts aliases such as `rve`) |
| `rct addons install [id\|all]` | Downloads, verifies SHA-256, and installs one or all add-ons |
| `rct addons install --type plugin` | Installs all JavaScript plugins |
| `rct addons remove <id>` | Removes installed add-on files cleanly using tracked file receipts |
| `rct addons add <id> --url <url> [flags]` | Registers a custom plugin or asset pack in `<ConfigDir>/catalog.d/addon.<id>.json` |
| `rct addons lint [root]` | Validates catalog schemas, SHA-256 digests, templates, docs, and `README.md` parity |

### Configuration management (`rct config`)

| Command | Description |
| :--- | :--- |
| `rct config` | Displays resolved platform paths and active CLI settings with provenance |
| `rct config show` | Displays resolved platform paths and active CLI settings with provenance |
| `rct config get [key]` | Prints an effective CLI configuration value (`channel`, `openrct2_dir`, `bin_dir`, etc.) |
| `rct config set <key> <val>` | Persists a CLI setting in `<ConfigDir>/config.json` |
| `rct config install [--force]` | Deploys `config.ini`, WASD `shortcuts.json`, and Linux FreeDesktop launcher/icons |
| `rct config install --only shortcuts` | Deploys a specific component (`all`, `configs`, `config`, `shortcuts`, `desktop`, `icons`) |
| `rct config diff` | Compares live system OpenRCT2 configurations against rendered templates |
| `rct config sync` | Pulls live in-game tweaks back into repository `configs/`, sanitizing personal paths to tokens |
| `rct config uninstall [--all]` | Removes deployed configs and desktop entries while preserving saves, scenarios, and tracks |

### Shell autocompletion

```bash
# Bash
rct completion bash | sudo tee /etc/bash_completion.d/rct > /dev/null

# Zsh
rct completion zsh > "${fpath[1]}/_rct"

# Fish
rct completion fish > ~/.config/fish/completions/rct.fish

# PowerShell
rct completion powershell | Out-String | Invoke-Expression
```

## Keybindings cheat sheet

| Category | Shortcut | Action |
| :--- | :--- | :--- |
| Camera | `W` / `A` / `S` / `D` | Pan camera Up / Left / Down / Right |
| Camera | `Q` / `E` | Rotate view Anti-clockwise / Clockwise |
| Camera | `PageDown` / `PageUp` | Zoom in / Zoom out |
| Windows | `Tab` | Park window |
| Windows | `F1` / `SHIFT+R` | New Ride construction / Rides overview |
| Windows | `F4` / `F5` / `F6` / `F7` | Scenery / Footpaths / Land / Water tools |
| Windows | `F` / `R` / `G` / `Z` / `M` | Finances / Research / Guests / Staff / Map |
| Windows | `CTRL+ALT+C` | Cheats menu |
| View Modes | `C` / `I` / `X` | Underground view / Hide supports / Path issues |
| View Modes | `B` / `N` / `V` | Transparent paths / Transparent rides / Transparent scenery |
| Game Speed | `0` / `1` / `2` | Pause game / Decrease speed / Increase speed |
| System | `CTRL+S` / `CTRL+L` | Save game / Load game |
| System | `F11` / `SHIFT+F11` | Screenshot / Giant screenshot |

## Engine profile and visual defaults

* **Smooth frame pacing**: `uncap_fps = true`, `use_vsync = true`, `multithreading = true`, and hardware-accelerated
  software drawing (`drawing_engine = SOFTWARE_HWD`) for crisp isometric rendering without tearing.
* **Automatic HiDPI scaling**: Native monitor probing configures `fullscreen_width`, `fullscreen_height`, and
  `window_scale` (`1.0` up to 1080p, `1.5` for 1200p to 1600p, `2.0` above 1600p).
* **Builder quality-of-life**: `always_show_gridlines = true`, `landscape_smoothing = true`, `zoom_to_cursor = true`,
  `invisible_supports = true` toggleable on `I`, `debugging_tools = true`, and `autosave = 1` (retaining 10 autosaves).
* **Authentic base audio plus bonus ride music**: `OpenMusic` installs additive bonus ride music styles (*Galaxy*,
  *Acid*, *Blizzard*, *Progressive*, *Ragtime 2*, and OpenRCT2 title music) that work immediately, while
  `enabled_asset_packs` defaults to `""` so original RCT1/RCT2 audio plays unless toggled in **Options > Asset Packs**.

## Curated asset packs and plugins

Every curated asset pack and JavaScript plugin in `catalog/addon.*.json` is verified by SHA-256 checksum and installed automatically during `rct setup` (or individually with `rct addons install <id>`):

| Add-on | ID | Type | Category | Author | Version | Description |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| [OpenMusic](https://github.com/OpenRCT2/OpenMusic) | `openmusic` | Asset Pack | Audio & Music | OpenRCT2 Team | `v1.6.1` | Official OpenRCT2 bonus ride music styles and copyright-safe soundtrack packs |
| [OpenSoundEffects](https://github.com/OpenRCT2/OpenSoundEffects) | `opensound` | Asset Pack | Audio & Music | OpenRCT2 Team | `v1.0.6` | Official OpenRCT2 replacement sound effects asset pack and audio objects |
| [Official Title Sequences](https://github.com/OpenRCT2/title-sequences) | `title-sequences` | Asset Pack | Presentation | OpenRCT2 Team | `v0.5.6` | Official OpenRCT2, RCT1, Corkscrew Follies, Loopy Landscapes, and RCT2 title sequences |
| [Advanced Track](https://github.com/oli414/AdvancedTrack) | `advanced-track` | Plugin | Rides & Track | oli414 | `1.3.6` | Transform, curve, bank, and fine-tune custom coaster track segments freely |
| [Benchwarmer](https://github.com/tubbo/openrct2-benchwarmer) | `benchwarmer` | Plugin | Footpaths & Guests | tubbo | `v1.2.0` | Automatically populate footpaths with benches, bins, lamps, and queue TVs |
| [Park Rating Inspector](https://github.com/Basssiiie/OpenRCT2-ParkRatingInspector) | `park-rating-inspector` | Plugin | Park Management | Basssiiie | `v0.3` | Inspect the exact bonuses and penalties contributing to your current park rating |
| [Peep Editor](https://github.com/Manticore-007/OpenRCT2-PeepEditor) | `peep-editor` | Plugin | Footpaths & Guests | Manticore-007 | `v26.9.18` | Inspect and customize guest and staff stats, inventory, clothing, and behavior |
| [Ride Price Manager](https://github.com/mgovea/openrct2-ride-price-manager) | `ride-price-manager` | Plugin | Park Management | mgovea | `v1.3.2` | Automatically calculate and maintain optimal ticket prices as rides age |
| [Ride Vehicle Editor](https://github.com/Basssiiie/OpenRCT2-RideVehicleEditor) | `ride-vehicle-editor` | Plugin | Rides & Track | Basssiiie | `v2.2` | Edit ride vehicles, capacities, masses, spacing, and ratings live in-game |
| [Scenery Manager](https://github.com/Sadret/openrct2-scenery-manager) | `scenery-manager` | Plugin | Scenery & Building | Sadret | `v2.0.9` | Copy, paste, scatter, replace, and organize custom scenery across parks |
| [Soft Guest Cap Calculator](https://github.com/Sadret/openrct2-soft-guest-cap-calculator) | `soft-guest-cap-calculator` | Plugin | Park Management | Sadret | `v2.0.3` | Display per-ride soft guest cap contributions and total guest generation thresholds |

## Extensibility

### CLI settings (`config.json`)

Persistent CLI overrides live in `<ConfigDir>/config.json` and can be edited via `rct config set <key> <value>`:

```json
{
  "channel": "develop",
  "openrct2_dir": "/mnt/games/OpenRCT2User",
  "rct2_path": "/mnt/games/SteamLibrary/steamapps/common/Roller Coaster Tycoon 2",
  "rct1_path": "/mnt/games/SteamLibrary/steamapps/common/RollerCoaster Tycoon Deluxe"
}
```

### Add custom plugins and asset packs (`catalog.d/*.json`)

Register third-party plugins or `.parkobj` / `.parkap` archives via `rct addons add` or by placing a JSON file in
`<ConfigDir>/catalog.d/addon.<id>.json`:

```bash
rct addons add ride-painter \
  --name "Ride Painter" \
  --url "https://example.com/ride-painter.js" \
  --version-tag "v1.0.0"

rct addons install ride-painter
```

## Environment variables

| Variable | Description |
| :--- | :--- |
| `RCT_CONFIG_DIR` | Overrides the CLI configuration directory (`config.json` and `catalog.d/`) |
| `RCT_STATE_DIR` | Overrides the CLI state directory (`engine-receipt.json` and `addon-receipts.json`) |
| `XDG_CONFIG_HOME` | Respected on Linux when resolving default `~/.config/OpenRCT2` and `~/.config/openrct2-cli` |
| `XDG_DATA_HOME` | Respected on Linux when resolving `~/.local/share/openrct2-cli/engine`, `applications`, and `icons` |
| `XDG_STATE_HOME` | Respected on Linux when resolving default `~/.local/state/openrct2-cli` |

## Directory layouts

| Component | Linux (XDG) | macOS | Windows |
| :--- | :--- | :--- | :--- |
| OpenRCT2 user data | `~/.config/OpenRCT2/` | `~/Library/Application Support/OpenRCT2/` | `%USERPROFILE%\Documents\OpenRCT2\` |
| Installed plugins | `<OpenRCT2Dir>/plugin/*.js` | `<OpenRCT2Dir>/plugin/*.js` | `<OpenRCT2Dir>\plugin\*.js` |
| Installed asset packs | `<OpenRCT2Dir>/assetpack/*.parkap` | `<OpenRCT2Dir>/assetpack/*.parkap` | `<OpenRCT2Dir>\assetpack\*.parkap` |
| CLI settings & `catalog.d` | `~/.config/openrct2-cli/` | `~/Library/Application Support/openrct2-cli/` | `%LOCALAPPDATA%\openrct2-cli\` |
| CLI install receipts | `~/.local/state/openrct2-cli/` | `~/Library/Application Support/openrct2-cli/state/` | `%LOCALAPPDATA%\openrct2-cli\state\` |
| CLI & PATH entrypoints | `~/.local/bin/{rct,openrct2}` | `~/.local/bin/{rct,openrct2}` | `%LOCALAPPDATA%\Programs\OpenRCT2\bin\` |
| Portable OpenRCT2 engine | `~/.local/share/openrct2-cli/engine/` | `~/Applications/OpenRCT2.app` | `%LOCALAPPDATA%\Programs\OpenRCT2\engine\` |
| FreeDesktop launcher & icon | `~/.local/share/{applications,icons/hicolor}` | N/A (`OpenRCT2.app` bundle) | N/A |

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) for local build instructions, catalog schemas, and
linting rules.

## Security

To report a vulnerability privately, follow the instructions in [SECURITY.md](SECURITY.md).

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for full text.
