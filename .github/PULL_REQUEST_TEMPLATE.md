## Summary

Explain the problem or limitation in the prior code and why this design was chosen.

## Design and architecture checklist

- [ ] **Declarative catalog**: Curated channels, asset packs, and plugins are defined in `catalog/*.json`, validated via `rct addons lint`, and synchronized into `README.md` via `rct addons build`.
- [ ] **Portability and path invariants**: Zero hardcoded usernames or personal paths committed; placeholder tokens (`__FULLSCREEN_WIDTH__`, `__WINDOW_SCALE__`, `__GAME_PATH__`, `__RCT1_PATH__`, `__SAVE_DIR__`, `__USER__`, `__OPENRCT2_EXEC__`) used in configuration templates.
- [ ] **Destructive safety and idempotence**: Configuration and entity writes use atomic rename and change-detected timestamped backups (`.bak.<timestamp>`).
- [ ] **Strict line length**: Code complies with the 120-character maximum line length limit.

## Verification

- [ ] Ran `make format-check` (or `gofmt -s -w .`).
- [ ] Ran `make tidy-check` (`go.mod` and `go.sum` verified).
- [ ] Ran `make lint` (`go vet`, `revive`, and `desktop-file-validate` pass with zero warnings).
- [ ] Ran `make check` (all unit tests pass with `-race` and `-shuffle=on`, catalog lint, `README.md` parity, and path invariant audit).
- [ ] Updated and synchronized documentation (`README.md`, `CONTRIBUTING.md`, `AGENTS.md`, `llms.txt`).
