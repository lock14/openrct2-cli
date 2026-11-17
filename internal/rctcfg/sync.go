package rctcfg

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

var reDesktopExec = regexp.MustCompile(`(?m)^Exec=.*$`)

// DiffConfigs compares live installed files against rendered repository templates.
func DiffConfigs(paths *config.Paths, opts RenderOptions, targetOS string, out io.Writer) error {
	if paths == nil {
		return errors.New("nil configuration paths")
	}
	if out == nil {
		out = os.Stdout
	}
	if opts.SaveDir == "" {
		opts.SaveDir = paths.SaveDir
	}
	if opts.PlayerName == "" {
		opts.PlayerName = ResolvePlayerName()
	}
	if opts.ExecPath == "" {
		opts.ExecPath = paths.ManagedEngineBinaryForOS(targetOS)
	}

	cfgDest := filepath.Join(paths.OpenRCT2Dir, "config.ini")
	if opts.RCT2Path == "" || opts.RCT1Path == "" {
		exRCT2, exRCT1 := ReadExistingGamePaths(cfgDest)
		if opts.RCT2Path == "" {
			opts.RCT2Path = exRCT2
		}
		if opts.RCT1Path == "" {
			opts.RCT1Path = exRCT1
		}
	}

	renderedINI, err := RenderConfigINI(opts)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "=== OpenRCT2 config.ini Diff ===")
	printLineDiff("config.ini", renderedINI, cfgDest, out)

	shortcutsBytes, err := RenderShortcutsJSON()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\n=== OpenRCT2 shortcuts.json Diff ===")
	printLineDiff("shortcuts.json", string(shortcutsBytes), filepath.Join(paths.OpenRCT2Dir, "shortcuts.json"), out)

	if targetOS == "linux" && paths.DesktopDir != "" {
		renderedDesktop, dErr := RenderDesktopEntry(opts.ExecPath)
		if dErr != nil {
			return dErr
		}
		fmt.Fprintln(out, "\n=== OpenRCT2.desktop Diff ===")
		printLineDiff("OpenRCT2.desktop", renderedDesktop, filepath.Join(paths.DesktopDir, "OpenRCT2.desktop"), out)
	}

	return nil
}

func printLineDiff(title, expected, diskPath string, out io.Writer) {
	diskData, err := os.ReadFile(diskPath)
	if err != nil {
		fmt.Fprintf(out, "File not installed on disk: %s\n", diskPath)
		return
	}

	expLines := strings.Split(strings.TrimSpace(fsutil.NormalizeLF(expected)), "\n")
	actLines := strings.Split(strings.TrimSpace(fsutil.NormalizeLF(string(diskData))), "\n")

	diffCount := 0
	maxLines := len(expLines)
	if len(actLines) > maxLines {
		maxLines = len(actLines)
	}

	for i := 0; i < maxLines; i++ {
		var expLine, actLine string
		if i < len(expLines) {
			expLine = expLines[i]
		}
		if i < len(actLines) {
			actLine = actLines[i]
		}
		if expLine != actLine {
			diffCount++
			if diffCount <= 20 {
				if expLine != "" {
					fmt.Fprintf(out, "- %s\n", expLine)
				}
				if actLine != "" {
					fmt.Fprintf(out, "+ %s\n", actLine)
				}
			}
		}
	}

	if diffCount == 0 {
		fmt.Fprintf(out, "✓ %s is in sync with system (%s)\n", title, diskPath)
	} else if diffCount > 20 {
		fmt.Fprintf(out, "... and %d more line differences\n", diffCount-20)
	}
}

// SyncConfigs pulls live in-game tweaks from the system back into repository templates,
// sanitizing personal paths, usernames, resolutions, and runtime timestamps to placeholders.
func SyncConfigs(paths *config.Paths, repoDir, targetOS string, out io.Writer) error {
	if paths == nil {
		return errors.New("nil configuration paths")
	}
	if out == nil {
		out = os.Stdout
	}
	if strings.TrimSpace(repoDir) == "" {
		repoDir = "."
	}

	// 1. Sync config.ini
	liveConfig := filepath.Join(paths.OpenRCT2Dir, "config.ini")
	repoConfig := filepath.Join(repoDir, "configs", "openrct2", "config.ini")
	if data, err := os.ReadFile(liveConfig); err == nil {
		if _, statErr := os.Stat(filepath.Dir(repoConfig)); statErr == nil {
			sanitized := SanitizeConfigINI(string(data))
			if _, _, wErr := fsutil.WriteTextAtomicIfChanged(repoConfig, sanitized, 0o644); wErr != nil {
				return wErr
			}
			fsutil.RemoveBackups(repoConfig)
			fmt.Fprintf(out, "✓ Synced %s -> %s\n", liveConfig, repoConfig)
		}
	}

	// 2. Sync shortcuts.json
	liveShortcuts := filepath.Join(paths.OpenRCT2Dir, "shortcuts.json")
	repoShortcuts := filepath.Join(repoDir, "configs", "openrct2", "shortcuts.json")
	if data, err := os.ReadFile(liveShortcuts); err == nil {
		if _, statErr := os.Stat(filepath.Dir(repoShortcuts)); statErr == nil {
			if _, _, wErr := fsutil.WriteTextAtomicIfChanged(repoShortcuts, string(data), 0o644); wErr != nil {
				return wErr
			}
			fsutil.RemoveBackups(repoShortcuts)
			fmt.Fprintf(out, "✓ Synced %s -> %s\n", liveShortcuts, repoShortcuts)
		}
	}

	// 3. Sync OpenRCT2.desktop on Linux
	if targetOS == "linux" && paths.DesktopDir != "" {
		liveDesktop := filepath.Join(paths.DesktopDir, "OpenRCT2.desktop")
		repoDesktop := filepath.Join(repoDir, "configs", "desktop", "OpenRCT2.desktop")
		if data, err := os.ReadFile(liveDesktop); err == nil {
			if _, statErr := os.Stat(filepath.Dir(repoDesktop)); statErr == nil {
				sanitized := reDesktopExec.ReplaceAllString(
					fsutil.NormalizeLF(string(data)),
					"Exec=__OPENRCT2_EXEC__ %u",
				)
				if _, _, wErr := fsutil.WriteTextAtomicIfChanged(repoDesktop, sanitized, 0o644); wErr != nil {
					return wErr
				}
				fsutil.RemoveBackups(repoDesktop)
				fmt.Fprintf(out, "✓ Synced %s -> %s\n", liveDesktop, repoDesktop)
			}
		}
	}

	return nil
}

// SanitizeConfigINI normalizes machine-specific keys in a live config.ini back into placeholders.
func SanitizeConfigINI(raw string) string {
	text := fsutil.NormalizeLF(raw)
	text = SetINIValues(text, "general", map[string]string{
		"fullscreen_width":        "__FULLSCREEN_WIDTH__",
		"fullscreen_height":       "__FULLSCREEN_HEIGHT__",
		"window_width":            "__WINDOW_WIDTH__",
		"window_height":           "__WINDOW_HEIGHT__",
		"window_scale":            "__WINDOW_SCALE__",
		"game_path":               `"__GAME_PATH__"`,
		"rct1_path":               `"__RCT1_PATH__"`,
		"last_game_directory":     `"__SAVE_DIR__"`,
		"last_run_version":        `""`,
		"last_version_check_time": "0",
	})
	text = SetINIValues(text, "network", map[string]string{
		"player_name": `"__USER__"`,
	})
	return text
}

// Uninstall removes deployed OpenRCT2 configuration files, shortcuts, and Linux desktop/icon files
// while preserving user saves, scenarios, tracks, and screenshots. When removeEngine is true,
// managed engine binaries and receipts are also removed.
func Uninstall(paths *config.Paths, removeEngine bool, targetOS string, out io.Writer) error {
	if paths == nil {
		return errors.New("nil configuration paths")
	}
	if out == nil {
		out = io.Discard
	}

	targets := []string{
		filepath.Join(paths.OpenRCT2Dir, "config.ini"),
		filepath.Join(paths.OpenRCT2Dir, "shortcuts.json"),
	}
	if targetOS == "linux" {
		if paths.DesktopDir != "" {
			targets = append(targets, filepath.Join(paths.DesktopDir, "OpenRCT2.desktop"))
		}
		if paths.IconDir != "" {
			targets = append(targets,
				filepath.Join(paths.IconDir, "openrct2.png"),
				filepath.Join(paths.IconDir, "OpenRCT2.png"),
			)
		}
		if paths.HicolorIconDir != "" {
			targets = append(targets, filepath.Join(paths.HicolorIconDir, "openrct2.png"))
		}
	}

	for _, t := range targets {
		if err := os.Remove(t); err == nil {
			fmt.Fprintf(out, "✓ Removed %s\n", t)
		}
		fsutil.RemoveBackups(t)
	}

	if removeEngine {
		binPath := paths.ManagedEngineBinaryForOS(targetOS)
		if binPath != "" {
			if err := os.Remove(binPath); err == nil {
				fmt.Fprintf(out, "✓ Removed engine binary %s\n", binPath)
			}
			fsutil.RemoveBackups(binPath)
		}
		if targetOS == "darwin" && strings.HasSuffix(paths.EngineDir, "OpenRCT2.app") {
			if _, err := os.Stat(paths.EngineDir); err == nil {
				_ = os.RemoveAll(paths.EngineDir)
				fmt.Fprintf(out, "✓ Removed app bundle %s\n", paths.EngineDir)
			}
		} else if targetOS == "windows" && paths.EngineDir != "" {
			if _, err := os.Stat(paths.EngineDir); err == nil {
				_ = os.RemoveAll(paths.EngineDir)
				fmt.Fprintf(out, "✓ Removed engine bundle %s\n", paths.EngineDir)
			}
			shim := filepath.Join(paths.BinDir, "openrct2.cmd")
			_ = os.Remove(shim)
		} else if paths.EngineDir != "" && filepath.Clean(paths.EngineDir) != filepath.Clean(paths.BinDir) {
			if _, err := os.Stat(paths.EngineDir); err == nil {
				_ = os.RemoveAll(paths.EngineDir)
				fmt.Fprintf(out, "✓ Removed engine directory %s\n", paths.EngineDir)
			}
		}
		if paths.EngineReceiptFile != "" {
			_ = os.Remove(paths.EngineReceiptFile)
		}
	}

	if targetOS == "linux" {
		refreshDesktopCaches(paths)
	}
	return nil
}
