# Agent Guidelines for openrct2-configs

This repository contains curated configuration files, keybinding presets, desktop integrations, and deployment scripts for **OpenRCT2** across Linux, macOS, and Windows.

Any agent modifying this repository must follow these core principles.

---

## 1. Portability & Path Invariants

- **Never Commit Personal User Paths**: Never commit absolute personal paths like `/home/<username>/` or `C:\Users\<username>\` into configuration files, desktop entries, or scripts.
- **Use `__HOME__` and `__USER__` Placeholders**:
  - In `config/config.ini`, user home paths use `__HOME__` and player names use `__USER__`.
  - In `desktop/OpenRCT2.desktop`, use standard FreeDesktop theme icon naming (`Icon=openrct2`) without absolute paths.
  - Deployment tooling (`Makefile`, `setup.sh`, `setup.ps1`) dynamically substitutes placeholders at installation time.
- **Respect Standard Directory Structures**:
  - **OpenRCT2 User Directory (Linux)**: `~/.config/OpenRCT2/`
  - **OpenRCT2 Desktop Entry (Linux)**: `~/.local/share/applications/OpenRCT2.desktop`
  - **OpenRCT2 Icons (Linux)**: `~/.local/share/icons/hicolor/256x256/apps/openrct2.png` & `~/.local/share/icons/openrct2.png`
  - **OpenRCT2 User Binary (Linux)**: `~/.local/bin/openrct2` (symlink to `/opt/OpenRCT2/OpenRCT2.AppImage`)
  - **OpenRCT2 User Directory (macOS)**: `~/Library/Application Support/OpenRCT2/`
  - **OpenRCT2 User Directory (Windows)**: `%USERPROFILE%\Documents\OpenRCT2\` (or `%APPDATA%\OpenRCT2\`)

---

## 2. Destructive Safety & Idempotent Backup Policy

- **Mandatory Non-Destructive Backups**: Any installation target or deployment script (`Makefile`, `setup.sh`, `setup.ps1`) must create timestamped backups (`.bak.<timestamp>`) before overwriting or modifying any existing user configuration, desktop, or script file.
- **Idempotency Requirement**: Backups must only be generated if the existing file content actually differs (`! cmp -s "$src" "$dest"`), preventing backup accumulation across identical re-installations.
- **Clean Uninstallation**: Maintain a working `make uninstall` target that removes deployed files, desktop entries, and symlinks cleanly without altering unrelated configs.
- **Preserve Unrelated Configuration**: When updating engine settings, keybindings, or interface options, avoid stripping or modifying unrelated user preferences unless explicitly requested.

---

## 3. Configuration & Desktop Standards Hygiene

- **FreeDesktop XDG Compliance**:
  - `desktop/OpenRCT2.desktop` must follow the FreeDesktop Desktop Entry Specification, including `StartupWMClass=openrct2`, `PrefersNonDefaultGPU=true`, and all standard MIME types / URI handlers (`x-scheme-handler/openrct2`).
  - Icons must be installed into standard hicolor theme directories and icon caches must be updated (`update-desktop-database`, `gtk-update-icon-cache`).
- **Control Scheme Intent**:
  - Maintain the curated WASD viewport navigation and Q/E rotation mappings in `shortcuts.json`.
  - Ensure all JSON files pass validation (`jq . config/shortcuts.json`).
- **Engine Setting Standards**:
  - Ensure uncapped FPS, hardware drawing optimization, and UI scaling integrity are preserved in `config.ini`.
  - Maintain clean game paths for base RCT1/RCT2 installations.

---

## 4. Cross-Platform Parity & Tooling Maintenance

- **Cross-Platform Consistency**: When adding or updating configurations, ensure compatibility across Linux, macOS, and Windows environments.
- **Maintain Bidirectional Sync Targets**:
  - Every config and desktop file tracked in the repository must be integrated into:
    1. `make install` & `make install-<target>`
    2. `make uninstall`
    3. `make sync` (to pull in-game/desktop tweaks from the system back into the repo, sanitizing personal paths)
    4. `make diff` (to inspect differences between repo and active system configs)
    5. `setup.sh` (Linux / macOS fallback script)
    6. `setup.ps1` (Windows PowerShell script)
    7. `README.md` (documentation and shortcuts table)

---

## 5. Documentation Boundaries & Mandatory Updates

- **Mandatory Documentation Synchronization**:
  - **Every agent change must update documentation**: Whenever shortcuts, engine settings, desktop entries, build targets, directory layouts, or script mechanics are modified, the relevant documentation **must** be updated within the same pull request/commit.
- **`README.md` is for Users**:
  - `README.md` must focus purely on user-facing concerns: installation commands, directory setup, shortcuts catalog, engine features, and player-facing usage.
  - Keep `README.md` clear, accessible, and free of internal agent mechanics or maintenance minutiae.
- **`AGENTS.md` is for Agents & Contributors**:
  - Repository principles, path invariant enforcement, internal build architecture, script implementation details, and agent guidelines belong in `AGENTS.md`.
  - Never leak prompt engineering directives, agent-specific meta-instructions, or internal maintenance rules into `README.md`.

---

## 6. Continuous Learning & Principle Encoding

- **Persist User Corrections**:
  - Whenever an agent is corrected, redirected, or receives feedback on repository conventions, it **must immediately encode the underlying principle into `AGENTS.md`** before concluding the task.
  - This ensures all future agent sessions and contributors automatically inherit the correction.

---

## 7. Verification Checklist for Agents

Before completing any changes:
1. Run `make check` to execute the full local validation suite (syntax, JSON validity, path invariants, idempotent install, and uninstall test).
2. Verify `git diff` contains no hardcoded usernames or personal paths.
3. Run `make -n install` or `make help` to verify `Makefile` syntax.
4. If shell scripts were modified, run `bash -n <script.sh>` to verify syntax.
5. **Update Documentation**:
   - Update `README.md` if user-facing behavior changed (shortcuts, commands, defaults, directories).
   - Update `AGENTS.md` if repository principles, build architecture, or agent rules changed.
   - **Encode Corrections**: If the user or a reviewer provided a correction, ensure the principle is codified in `AGENTS.md`.
6. Verify `README.md` tables (shortcuts cheat sheet, Makefile reference) accurately reflect the new state.
