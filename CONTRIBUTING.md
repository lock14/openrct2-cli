# Contributing to openrct2-cli

Thank you for contributing to `openrct2-cli` (`rct`). This guide covers building from source, repository layout,
curated catalog conventions, configuration template hygiene, and the local verification workflow.

## Prerequisites

* Go 1.27 or newer (`go version`)
* GNU Make (optional, for convenience targets on Linux and macOS)
* `revive` (optional locally, enforced in CI: `go install github.com/mgechev/revive@latest`)
* `desktop-file-validate` (optional locally on Linux, enforced in CI via `desktop-file-utils`)

## Building and testing locally

```bash
# Compile static binary to bin/rct
make build

# Run full unit test suite with race detector and test shuffling
make test

# Run benchmarks with allocation tracking
make bench

# Format, verify module hygiene, lint, test, and audit path invariants
make check
```

## Project architecture

| Path | Purpose |
| :--- | :--- |
| `embed.go` | Single root `embed.FS` bundling `catalog/*.json`, `configs/openrct2/*`, and `configs/desktop/*` |
| `catalog/` | Declarative JSON definitions for release channels (`channel.*.json`) and add-ons (`addon.*.json`) |
| `configs/openrct2/` | Curated `config.ini` template (with runtime tokens) and WASD `shortcuts.json` |
| `configs/desktop/` | FreeDesktop compliant `OpenRCT2.desktop` template and 256x256 `openrct2.png` icon |
| `cmd/rct/` | Cobra CLI command tree (`rct`, `setup`, `engine`, `games`, `addons`, `config`) |
| `internal/catalog/` | Strict JSON entity decoder, two-layer `catalog.d` loader, and shorthand query matcher |
| `internal/config/` | Platform XDG, macOS Application Support, and Windows path resolution plus `config.json` |
| `internal/content/` | Verified downloader and `os.OpenRoot` extractor for `.parkap`, `.parkobj`, `.parkseq`, and `.js` |
| `internal/display/` | Native display resolution detection and HiDPI `window_scale` calculation |
| `internal/engine/` | GitHub Releases installer (`release` and `develop`), `.app`/`.zip` extractor, and launcher |
| `internal/fsutil/` | Atomic file writes, idempotent timestamped backups, SHA-256 downloads, and `SafeJoin` |
| `internal/gamedata/` | Steam `libraryfolders.vdf` parser and case-insensitive RCT2/Classic/RCT1 asset validators |
| `internal/rctcfg/` | Non-clobbering INI editor, template renderer, FreeDesktop deployer, diff, and reverse sync |
| `internal/repolint/` | Catalog, template, documentation, and `README.md` table parity linter |

## Adding or updating curated add-ons

Each curated asset pack or JavaScript plugin lives in `catalog/addon.<id>.json` and must include a verified 64-character
lowercase SHA-256 hex digest:

```json
{
  "schema_version": 1,
  "kind": "addon",
  "id": "benchwarmer",
  "display_name": "Benchwarmer",
  "addon_type": "plugin",
  "category": "Footpaths & Guests",
  "author": "tubbo",
  "version": "v1.2.0",
  "description": "Automatically populate footpaths with benches, bins, lamps, and queue TVs",
  "aliases": [
    "benches"
  ],
  "homepage": "https://github.com/tubbo/openrct2-benchwarmer",
  "source": {
    "repo": "tubbo/openrct2-benchwarmer",
    "url": "https://github.com/tubbo/openrct2-benchwarmer/releases/download/v1.2.0/benchwarmer.js",
    "sha256": "8d2ffac0a2657ed1f16efb5bbd655293442795eb1e615ed6f095e6c8fb89fd74",
    "mode": "file",
    "target_path": "plugin/benchwarmer.js"
  },
  "targets": [
    "plugin/benchwarmer.js"
  ]
}
```

After adding or updating any `catalog/addon.*.json` file:

```bash
# 1. Rebuild the binary so embedded catalog assets include your changes
make build

# 2. Regenerate the curated add-ons table in README.md
./bin/rct addons build

# 3. Validate catalog schemas, SHA-256 digests, and README parity
./bin/rct addons lint
```

## Updating OpenRCT2 configuration templates

Never commit personal usernames, home directory paths, or monitor-specific resolutions into `configs/`. If you tweak
settings inside OpenRCT2 and want to pull them back into the repository:

```bash
# Pulls live config.ini, shortcuts.json, and OpenRCT2.desktop while sanitizing personal values to tokens
./bin/rct config sync

# Inspect git diff and run validation
git diff configs/
make check
```

## Coding standards

* Keep Go lines at or below 120 characters (`revive.toml`).
* Write all command output through `cmd.OutOrStdout()` and diagnostics through `cmd.ErrOrStderr()` so CLI commands are
  testable in-memory without subprocesses.
* Never perform unverified HTTP downloads or unjailed ZIP extractions; route downloads through
  `fsutil.DownloadVerifiedContext` and archive extraction through `os.OpenRoot` plus `fsutil.CleanRel`.
* Add table-driven unit tests alongside any new or modified package behavior using `t.TempDir()` and `httptest.Server`.
