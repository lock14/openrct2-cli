# Repository design principles

* **Declarative entity catalog and single source of truth**:
  * `catalog/*.json` (`channel.<id>.json` and `addon.<id>.json`) is the sole source of truth for all curated OpenRCT2
    release channels (`release` and `develop`), official audio/music/title-sequence asset packs, and community
    JavaScript plugins.
  * **Single root embed**: All embedded assets (`catalog/*.json`, `configs/openrct2/config.ini`,
    `configs/openrct2/shortcuts.json`, `configs/desktop/OpenRCT2.desktop`, and `configs/desktop/openrct2.png`) are
    embedded once at the repository root in `embed.go` (`openrct2cli.FS`). Do not create duplicate asset copies inside
    `internal/`.
  * **Synchronize and lint**: Run `rct addons build` to synchronize `catalog/*.json` into `README.md`, and run
    `rct addons lint` to validate strict JSON schemas, SHA-256 digests, placeholder tokens, personal path checks,
    documentation typography/links, and `README.md` table parity.
  * **Parity verification**: Run `make check` to verify formatting, module hygiene, `go vet`, `revive`,
    `desktop-file-validate`, race-enabled unit tests, and path invariants.

* **Portability and path invariants**:
  * **Never commit personal user paths, display resolutions, or player names**: Never commit absolute personal paths
    such as `/home/<username>/`, `/Users/<username>/`, or `C:\Users\<username>\`, hardcoded local monitor resolutions,
    or personal player names into configuration templates, catalog JSONs, documentation, or Go code.
  * **Use standard placeholder tokens**:
    * In `configs/openrct2/config.ini`, use `__FULLSCREEN_WIDTH__`, `__FULLSCREEN_HEIGHT__`, `__WINDOW_WIDTH__`,
      `__WINDOW_HEIGHT__`, `__WINDOW_SCALE__`, `"__GAME_PATH__"`, `"__RCT1_PATH__"`, `"__SAVE_DIR__"`, and `"__USER__"`.
    * In `configs/desktop/OpenRCT2.desktop`, use `Exec=__OPENRCT2_EXEC__ %u` and `Icon=openrct2`.
    * Deployment (`rct config install`, `rct setup`) dynamically substitutes placeholders via display probing and
      Steam/GOG discovery, and reverse synchronization (`rct config sync`) sanitizes live values back to placeholders.
  * **Respect platform standard directories (XDG Base Dir v0.8, Apple, and Microsoft `FOLDERID_UserProgramFiles`)**:
    * **OpenRCT2 user directory**: `~/.config/OpenRCT2/` on Linux (`$XDG_CONFIG_HOME/OpenRCT2`),
      `~/Library/Application Support/OpenRCT2/` on macOS, and `%USERPROFILE%\Documents\OpenRCT2\` on Windows.
    * **CLI configuration and user catalog (human-edited, dotfile-safe)**: `~/.config/openrct2-cli/` on Linux
      (`$XDG_CONFIG_HOME/openrct2-cli`), `~/Library/Application Support/openrct2-cli/` on macOS, and
      `%LOCALAPPDATA%\openrct2-cli\` on Windows (overridable via `RCT_CONFIG_DIR`), holding `config.json` and
      `catalog.d/*.json`. Never write machine-generated install receipts into `ConfigDir`.
    * **CLI state and receipts (machine-generated)**: `~/.local/state/openrct2-cli/` on Linux
      (`$XDG_STATE_HOME/openrct2-cli`), `~/Library/Application Support/openrct2-cli/state/` on macOS, and
      `%LOCALAPPDATA%\openrct2-cli\state\` on Windows (overridable via `RCT_STATE_DIR`), holding `engine-receipt.json`
      and `addon-receipts.json`.
    * **Engine payloads vs PATH entrypoints**:
      * **Linux**: Store the AppImage payload in `~/.local/share/openrct2-cli/engine/OpenRCT2.AppImage`
        (`$XDG_DATA_HOME/openrct2-cli/engine/`) and expose `~/.local/bin/openrct2` as an atomic symlink alongside
        `~/.local/bin/rct` so `.bak.*` binaries never bloat `~/.local/bin` or pollute `$PATH` tab-completion.
      * **macOS**: Extract `~/Applications/OpenRCT2.app` and expose `~/.local/bin/openrct2` as a POSIX wrapper alongside
        `~/.local/bin/rct`.
      * **Windows**: Use Microsoft's `FOLDERID_UserProgramFiles` standard (`%LOCALAPPDATA%\Programs\OpenRCT2\engine\`
        and `%LOCALAPPDATA%\Programs\OpenRCT2\bin\`) rather than creating custom folders in `%USERPROFILE%`.
    * **FreeDesktop integration (Linux)**: `~/.local/share/applications/OpenRCT2.desktop` and
      `~/.local/share/icons/hicolor/256x256/apps/openrct2.png` (`$XDG_DATA_HOME`), without writing duplicate icons to the
      top-level `icons/` root.

* **Destructive safety, idempotence, and security policy**:
  * **Mandatory atomic writes and change-detected backups**: All configuration, catalog, and receipt writes must use
    `fsutil.WriteFileAtomic`, `fsutil.WriteFileAtomicIfChanged`, `fsutil.WriteTextAtomicIfChanged`, or
    `fsutil.WriteJSONAtomicIfChanged`. Backups (`.bak.<timestamp>`, retaining at most `MaxBackupsPerFile = 3` per file)
    must only be generated when the existing regular file content actually differs, preventing backup accumulation
    across repeated identical runs.
  * **Non-clobbering INI updates**: When `config.ini` already exists and `--force` is not passed, `rctcfg.Deploy`
    preserves user settings and surgically backfills missing or invalid `game_path` and `rct1_path` entries rather than
    clobbering in-game customizations. `config.json` preserves unknown top-level JSON keys across `SaveConfig` calls.
  * **Clean uninstallation**: `rct config uninstall` removes deployed `config.ini`, `shortcuts.json`,
    `OpenRCT2.desktop`, and icons while preserving user saves (`save/`), scenarios (`scenario/`), tracks (`track/`),
    and screenshots (`--all` additionally delegates managed engine cleanup to `engine.Uninstall`).
  * **Verified HTTP downloads and archive jailing**: Outbound downloads in `engine.Installer` and `content.Installer`
    must use `fsutil.DownloadVerifiedContext` with `fsutil.UserAgent`, enforcing bounded stream sizes
    (`MaxDownloadBytes`), rejecting HTTPS downgrade redirects, and verifying 64-character hex SHA-256 checksums before
    renaming over any target. ZIP extractions (`macos-universal.zip`, `windows-portable-*.zip`, asset packs, and zipped
    plugins) must use `fsutil.ExtractZip` / `fsutil.ExtractZipFiles` inside `os.OpenRoot` (enforcing `CleanRel`,
    per-entry `MaxExtractFileBytes`, total-archive `MaxExtractTotalBytes`, and `MaxZipEntries`).

* **OpenRCT2 configuration and add-on standards**:
  * **Control scheme**: Preserve curated WASD viewport panning, Q/E camera rotation, and PageUp/PageDown zoom bindings
    in `configs/openrct2/shortcuts.json`.
  * **Visual and audio defaults**: Maintain `drawing_engine = SOFTWARE_HWD`, `uncap_fps = true`, `use_vsync = true`,
    `landscape_smoothing = true`, and `enabled_asset_packs = ""` in `configs/openrct2/config.ini` so additive
    `OpenMusic` bonus ride tracks work out of the box while authentic RCT1/RCT2 base audio remains active until toggled
    in-game.
  * **Binary naming**: Upstream OpenRCT2 ships `openrct2` (GUI) and `openrct2-cli` (headless tool). The CLI binary is
    named `rct` so it never collides with upstream executables on `PATH`.

* **Documentation boundaries and continuous learning**:
  * **Mandatory synchronization**: Whenever CLI commands, shortcuts, catalog items, engine settings, or directory
    structures change, update `README.md`, `CONTRIBUTING.md`, `AGENTS.md`, and `llms.txt` in the same change.
  * **`README.md` is for users**: Focus purely on installation, `rct setup`, CLI reference, keybindings, curated
    add-ons, directory layouts, and user extensibility. Never leak internal agent rules into `README.md`.
  * **Typography hygiene**: Avoid decorative emojis, horizontal rule dividers, bolded first-column table cells in
    `README.md` and `CONTRIBUTING.md`, stacked bold spans, and raw ASCII arrows outside inline/fenced code.
  * **Persist corrections**: Whenever a user or reviewer provides a repository convention correction, encode the
    underlying principle into `AGENTS.md` before concluding the task.

# Package structure and architecture conventions

* `embed.go`: Root `embed.FS` (`openrct2cli.FS`) embedding `catalog/*.json`, `configs/openrct2/*`, and
  `configs/desktop/*`.
* `catalog/`: Curated JSON entities (`channel.release.json`, `channel.develop.json`, `addon.*.json`).
* `configs/`: Tokenized `config.ini`, WASD `shortcuts.json`, `OpenRCT2.desktop`, and `openrct2.png`.
* `cmd/rct/`: Modular Cobra CLI implementation (`main.go`, `completions.go`, `setup.go`, `engine.go`, `games.go`,
  `addons.go`, `config.go`).
* `internal/fsutil/`: Atomic file/JSON/symlink writes, idempotent change-detected backups, `CleanRel`/`SafeJoin`,
  `os.OpenRoot` bounded ZIP extraction, and SHA-256 verified HTTP downloads.
* `internal/config/`: Platform path resolution (`ResolveFor`) and schema-driven `config.json` persistence (`AppConfig`).
* `internal/display/`: Injectable primary monitor resolution probing and HiDPI `window_scale` calculation.
* `internal/catalog/`: Strict entity validator/codec, two-layer `catalog.d` loader with quarantine, defensive-copy query
  resolver, and user catalog store.
* `internal/gamedata/`: Steam `libraryfolders.vdf` parser, GOG/Wine/local scanner, and case-insensitive RCT2/Classic
  (`g1.dat`) and RCT1 (`csg1.dat`) directory validators.
* `internal/rctcfg/`: Deterministic section-preserving INI parser/merger, template renderer, FreeDesktop deployer, LCS
  line diff, reverse sanitizer (`SyncConfigs`), and safe config uninstaller.
* `internal/engine/`: GitHub Releases resolver (`OpenRCT2/OpenRCT2` and `OpenRCT2/OpenRCT2-binaries`), `os.OpenRoot`
  macOS `.app` and Windows portable `.zip` installer/uninstaller, receipt tracker, and process launcher (`PrepareLaunch`).
* `internal/content/`: SHA-256 verified asset pack and JavaScript plugin installer/uninstaller with per-addon file
  receipts and superseded file pruning on upgrade.
* `internal/repolint/`: Catalog, template, documentation, link/anchor, and `README.md` table parity linter.

# Verification checklist for agents

Before completing any changes:

1. Run `./bin/rct addons build` if any `catalog/addon.*.json` metadata changed.
2. Run `make check` (`gofmt`, `go mod tidy`, `go vet`, `revive`, `desktop-file-validate`, race-enabled `go test`, and
   path-invariant audit).
3. Run `./bin/rct addons lint` to verify catalog digests, template tokens, documentation links, and `README.md` parity.
4. Verify `git diff` contains zero hardcoded personal paths or usernames.
