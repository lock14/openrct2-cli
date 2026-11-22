package rctcfg

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

const maxDisplayedDiffLines = 20

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

type diffEdit struct {
	op   byte // '-' expected-only, '+' actual-only
	line string
}

// computeLineDiff computes a minimal LCS-based line diff when sizes are modest,
// falling back to positional comparison for huge files.
func computeLineDiff(expLines, actLines []string) []diffEdit {
	n, m := len(expLines), len(actLines)
	if int64(n)*int64(m) > 250_000 {
		var edits []diffEdit
		limit := max(n, m)
		for i := range limit {
			var e, a string
			if i < n {
				e = expLines[i]
			}
			if i < m {
				a = actLines[i]
			}
			if e != a {
				if e != "" {
					edits = append(edits, diffEdit{op: '-', line: e})
				}
				if a != "" {
					edits = append(edits, diffEdit{op: '+', line: a})
				}
			}
		}
		return edits
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if expLines[i] == actLines[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else {
				dp[i][j] = max(dp[i+1][j], dp[i][j+1])
			}
		}
	}

	var edits []diffEdit
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case expLines[i] == actLines[j]:
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			edits = append(edits, diffEdit{op: '-', line: expLines[i]})
			i++
		default:
			edits = append(edits, diffEdit{op: '+', line: actLines[j]})
			j++
		}
	}
	for ; i < n; i++ {
		edits = append(edits, diffEdit{op: '-', line: expLines[i]})
	}
	for ; j < m; j++ {
		edits = append(edits, diffEdit{op: '+', line: actLines[j]})
	}
	return edits
}

func printLineDiff(title, expected, diskPath string, out io.Writer) {
	diskData, err := os.ReadFile(diskPath)
	if err != nil {
		fmt.Fprintf(out, "File not installed on disk: %s\n", diskPath)
		return
	}

	expText := strings.TrimSpace(fsutil.NormalizeLF(expected))
	actText := strings.TrimSpace(fsutil.NormalizeLF(string(diskData)))
	if expText == actText {
		fmt.Fprintf(out, "✓ %s is in sync with system (%s)\n", title, diskPath)
		return
	}

	edits := computeLineDiff(strings.Split(expText, "\n"), strings.Split(actText, "\n"))
	for i, edit := range edits {
		if i >= maxDisplayedDiffLines {
			break
		}
		fmt.Fprintf(out, "%c %s\n", edit.op, edit.line)
	}
	if len(edits) > maxDisplayedDiffLines {
		fmt.Fprintf(out, "... and %d more line differences\n", len(edits)-maxDisplayedDiffLines)
	}
}

func syncTemplateFile(
	livePath, repoDest string,
	transform func(string) string,
	out io.Writer,
) error {
	data, err := os.ReadFile(livePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", livePath, err)
	}
	if fi, statErr := os.Stat(filepath.Dir(repoDest)); statErr != nil || !fi.IsDir() {
		return nil
	}
	content := string(data)
	if transform != nil {
		content = transform(content)
	}
	if _, _, err := fsutil.WriteTextAtomicIfChanged(repoDest, content, 0o644); err != nil {
		return err
	}
	fsutil.RemoveBackups(repoDest)
	fmt.Fprintf(out, "✓ Synced %s -> %s\n", livePath, repoDest)
	return nil
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

	openrct2TemplateDir := filepath.Join(repoDir, "configs", "openrct2")
	if fi, err := os.Stat(openrct2TemplateDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("repository template directory %s not found", openrct2TemplateDir)
	}

	if err := syncTemplateFile(
		filepath.Join(paths.OpenRCT2Dir, "config.ini"),
		filepath.Join(openrct2TemplateDir, "config.ini"),
		SanitizeConfigINI,
		out,
	); err != nil {
		return err
	}

	if err := syncTemplateFile(
		filepath.Join(paths.OpenRCT2Dir, "shortcuts.json"),
		filepath.Join(openrct2TemplateDir, "shortcuts.json"),
		nil,
		out,
	); err != nil {
		return err
	}

	if targetOS == "linux" && paths.DesktopDir != "" {
		if err := syncTemplateFile(
			filepath.Join(paths.DesktopDir, "OpenRCT2.desktop"),
			filepath.Join(repoDir, "configs", "desktop", "OpenRCT2.desktop"),
			func(raw string) string {
				return reDesktopExec.ReplaceAllString(fsutil.NormalizeLF(raw), "Exec=__OPENRCT2_EXEC__ %u")
			},
			out,
		); err != nil {
			return err
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
// while preserving user saves, scenarios, tracks, and screenshots.
func Uninstall(paths *config.Paths, targetOS string, out io.Writer) error {
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
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", t, err)
		}
		fsutil.RemoveBackups(t)
	}

	if targetOS == "linux" {
		refreshDesktopCaches(paths)
	}
	return nil
}
