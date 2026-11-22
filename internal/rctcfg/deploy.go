package rctcfg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	openrct2cli "github.com/lock14/openrct2-cli"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/display"
	"github.com/lock14/openrct2-cli/internal/fsutil"
	"github.com/lock14/openrct2-cli/internal/gamedata"
)

const desktopCacheTimeout = 5 * time.Second

// RenderOptions holds machine-specific runtime values substituted into embedded templates.
type RenderOptions struct {
	Display    display.Info
	RCT2Path   string
	RCT1Path   string
	SaveDir    string
	PlayerName string
	ExecPath   string
}

// ResolvePlayerName determines the current OS username for OpenRCT2 multiplayer defaults.
func ResolvePlayerName() string {
	for _, envKey := range []string{"USER", "USERNAME"} {
		if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
			return v
		}
	}
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		parts := strings.Split(u.Username, `\`)
		return parts[len(parts)-1]
	}
	return "Player"
}

// FormatDesktopExec formats an executable path safely for a FreeDesktop Exec= key.
func FormatDesktopExec(execPath string) string {
	trimmed := strings.TrimSpace(execPath)
	if trimmed == "" {
		return "openrct2"
	}
	if strings.ContainsAny(trimmed, " \t\"\\") {
		escaped := strings.ReplaceAll(trimmed, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return trimmed
}

// RenderConfigINI renders the embedded configs/openrct2/config.ini template using opts.
func RenderConfigINI(opts RenderOptions) (string, error) {
	raw, err := fs.ReadFile(openrct2cli.FS, "configs/openrct2/config.ini")
	if err != nil {
		return "", fmt.Errorf("reading embedded config.ini: %w", err)
	}
	disp := opts.Display
	if disp.FullscreenWidth <= 0 || disp.FullscreenHeight <= 0 {
		disp = display.BuildInfo(1920, 1080)
	}
	player := strings.TrimSpace(opts.PlayerName)
	if player == "" {
		player = "Player"
	}

	replacer := strings.NewReplacer(
		"__FULLSCREEN_WIDTH__", strconv.Itoa(disp.FullscreenWidth),
		"__FULLSCREEN_HEIGHT__", strconv.Itoa(disp.FullscreenHeight),
		"__WINDOW_WIDTH__", strconv.Itoa(disp.WindowWidth),
		"__WINDOW_HEIGHT__", strconv.Itoa(disp.WindowHeight),
		"__WINDOW_SCALE__", disp.ScaleString(),
		`"__GAME_PATH__"`, FormatINIString(opts.RCT2Path),
		`"__RCT1_PATH__"`, FormatINIString(opts.RCT1Path),
		`"__SAVE_DIR__"`, FormatINIString(opts.SaveDir),
		`"__USER__"`, FormatINIString(player),
	)
	return replacer.Replace(fsutil.NormalizeLF(string(raw))), nil
}

// RenderShortcutsJSON returns the embedded configs/openrct2/shortcuts.json content.
func RenderShortcutsJSON() ([]byte, error) {
	return fs.ReadFile(openrct2cli.FS, "configs/openrct2/shortcuts.json")
}

// RenderDesktopEntry renders configs/desktop/OpenRCT2.desktop with the target engine binary path.
func RenderDesktopEntry(execPath string) (string, error) {
	raw, err := fs.ReadFile(openrct2cli.FS, "configs/desktop/OpenRCT2.desktop")
	if err != nil {
		return "", fmt.Errorf("reading embedded OpenRCT2.desktop: %w", err)
	}
	rendered := strings.ReplaceAll(
		fsutil.NormalizeLF(string(raw)),
		"__OPENRCT2_EXEC__",
		FormatDesktopExec(execPath),
	)
	return rendered, nil
}

// DeployComponent selects which subset of configuration/desktop files to deploy.
type DeployComponent string

const (
	// ComponentAll deploys config.ini, shortcuts.json, and (on Linux) desktop & icon files.
	ComponentAll DeployComponent = "all"
	// ComponentConfigs deploys both config.ini and shortcuts.json.
	ComponentConfigs DeployComponent = "configs"
	// ComponentConfig deploys only config.ini.
	ComponentConfig DeployComponent = "config"
	// ComponentShortcuts deploys only shortcuts.json.
	ComponentShortcuts DeployComponent = "shortcuts"
	// ComponentDesktop deploys only OpenRCT2.desktop (Linux).
	ComponentDesktop DeployComponent = "desktop"
	// ComponentIcons deploys only hicolor PNG icons (Linux).
	ComponentIcons DeployComponent = "icons"
)

// ValidDeployComponents returns the canonical names accepted for --only component selection.
func ValidDeployComponents() []string {
	return []string{
		string(ComponentAll),
		string(ComponentConfigs),
		string(ComponentConfig),
		string(ComponentShortcuts),
		string(ComponentDesktop),
		string(ComponentIcons),
	}
}

// ParseDeployComponent validates and normalizes a user-supplied deployment component selector.
func ParseDeployComponent(raw string) (DeployComponent, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return ComponentAll, nil
	case "configs":
		return ComponentConfigs, nil
	case "config", "ini":
		return ComponentConfig, nil
	case "shortcuts", "keys":
		return ComponentShortcuts, nil
	case "desktop":
		return ComponentDesktop, nil
	case "icons", "icon":
		return ComponentIcons, nil
	default:
		return "", fmt.Errorf(
			"invalid --only component %q (expected all, configs, config, shortcuts, desktop, or icons)",
			raw,
		)
	}
}

// DeployOptions configures OpenRCT2 configuration and desktop deployment.
type DeployOptions struct {
	TargetOS        string
	Component       DeployComponent
	Force           bool
	SkipCacheUpdate bool
	Render          RenderOptions
}

// ReadExistingGamePaths reads game_path and rct1_path from an existing config.ini on disk.
func ReadExistingGamePaths(configINIPath string) (string, string) {
	data, err := os.ReadFile(configINIPath)
	if err != nil {
		return "", ""
	}
	text := string(data)
	var rct2, rct1 string
	if raw, ok := GetINIValue(text, "general", "game_path"); ok {
		rct2 = UnquoteINIString(raw)
	}
	if raw, ok := GetINIValue(text, "general", "rct1_path"); ok {
		rct1 = UnquoteINIString(raw)
	}
	return rct2, rct1
}

// UpdateConfigGamePaths surgically updates game_path and/or rct1_path in an existing config.ini,
// or deploys a fresh config.ini when none exists yet.
func UpdateConfigGamePaths(paths *config.Paths, rct2Path, rct1Path string, out io.Writer) error {
	if paths == nil {
		return errors.New("nil paths")
	}
	if out == nil {
		out = io.Discard
	}
	dest := filepath.Join(paths.OpenRCT2Dir, "config.ini")
	existingBytes, err := os.ReadFile(dest)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		rendered, renderErr := RenderConfigINI(RenderOptions{
			Display:    display.Detect(),
			RCT2Path:   rct2Path,
			RCT1Path:   rct1Path,
			SaveDir:    paths.SaveDir,
			PlayerName: ResolvePlayerName(),
		})
		if renderErr != nil {
			return renderErr
		}
		if _, _, wErr := fsutil.WriteTextAtomicIfChanged(dest, rendered, 0o644); wErr != nil {
			return wErr
		}
		fmt.Fprintf(out, "✓ Created %s with discovered game paths\n", dest)
		return nil
	}

	updates := make(map[string]string)
	if strings.TrimSpace(rct2Path) != "" {
		updates["game_path"] = FormatINIString(rct2Path)
	}
	if strings.TrimSpace(rct1Path) != "" {
		updates["rct1_path"] = FormatINIString(rct1Path)
	}
	if len(updates) == 0 {
		return nil
	}
	updated := SetINIValues(string(existingBytes), "general", updates)
	changed, bkp, err := fsutil.WriteTextAtomicIfChanged(dest, updated, 0o644)
	if err != nil {
		return err
	}
	if bkp != "" {
		fmt.Fprintf(out, "  Backed up config.ini -> %s\n", filepath.Base(bkp))
	}
	if changed {
		fmt.Fprintf(out, "✓ Updated game paths in %s\n", dest)
	} else {
		fmt.Fprintf(out, "✓ Game paths in %s already up to date\n", dest)
	}
	return nil
}

// Deploy installs the requested OpenRCT2 configuration and desktop components idempotently.
func Deploy(paths *config.Paths, opts DeployOptions, out io.Writer) error {
	if paths == nil {
		return errors.New("nil configuration paths")
	}
	if out == nil {
		out = io.Discard
	}
	comp := opts.Component
	if comp == "" {
		comp = ComponentAll
	}
	if opts.Render.SaveDir == "" {
		opts.Render.SaveDir = paths.SaveDir
	}
	if opts.Render.PlayerName == "" {
		opts.Render.PlayerName = ResolvePlayerName()
	}
	if opts.Render.ExecPath == "" {
		opts.Render.ExecPath = paths.ManagedEngineBinaryForOS(opts.TargetOS)
	}

	wantConfig := comp == ComponentAll || comp == ComponentConfigs || comp == ComponentConfig
	wantShortcuts := comp == ComponentAll || comp == ComponentConfigs || comp == ComponentShortcuts
	wantDesktop := opts.TargetOS == "linux" && (comp == ComponentAll || comp == ComponentDesktop)
	wantIcons := opts.TargetOS == "linux" && (comp == ComponentAll || comp == ComponentIcons)

	if wantConfig {
		if err := deployConfigINI(paths, opts, out); err != nil {
			return err
		}
	}
	if wantShortcuts {
		if err := deployShortcuts(paths, out); err != nil {
			return err
		}
	}
	if wantDesktop {
		if err := deployDesktopFile(paths, opts.Render.ExecPath, out); err != nil {
			return err
		}
	}
	if wantIcons {
		if err := deployIcons(paths, out); err != nil {
			return err
		}
	}
	if (wantDesktop || wantIcons) && !opts.SkipCacheUpdate {
		refreshDesktopCaches(paths)
	}
	return nil
}

func deployConfigINI(paths *config.Paths, opts DeployOptions, out io.Writer) error {
	dest := filepath.Join(paths.OpenRCT2Dir, "config.ini")
	existingRCT2, existingRCT1 := ReadExistingGamePaths(dest)

	// Preserve existing valid game paths when caller did not supply newly discovered ones.
	if opts.Render.RCT2Path == "" {
		if _, ok := gamedata.ValidateRCT2Dir(existingRCT2); ok {
			opts.Render.RCT2Path = existingRCT2
		}
	}
	if opts.Render.RCT1Path == "" && gamedata.ValidateRCT1Dir(existingRCT1) {
		opts.Render.RCT1Path = existingRCT1
	}

	if existingBytes, err := os.ReadFile(dest); err == nil && !opts.Force {
		// Non-destructive default: preserve existing user config.ini, only backfilling missing/invalid game paths.
		updates := make(map[string]string)
		if _, ok := gamedata.ValidateRCT2Dir(existingRCT2); !ok && opts.Render.RCT2Path != "" {
			updates["game_path"] = FormatINIString(opts.Render.RCT2Path)
		}
		if !gamedata.ValidateRCT1Dir(existingRCT1) && opts.Render.RCT1Path != "" {
			updates["rct1_path"] = FormatINIString(opts.Render.RCT1Path)
		}
		if len(updates) == 0 {
			fmt.Fprintf(out, "✓ Preserved existing config.ini -> %s (use --force to overwrite with template)\n", dest)
			return nil
		}
		merged := SetINIValues(string(existingBytes), "general", updates)
		_, bkp, wErr := fsutil.WriteTextAtomicIfChanged(dest, merged, 0o644)
		if wErr != nil {
			return wErr
		}
		if bkp != "" {
			fmt.Fprintf(out, "  Backed up existing config.ini -> %s\n", filepath.Base(bkp))
		}
		fmt.Fprintf(out, "✓ Linked discovered game paths in existing config.ini -> %s\n", dest)
		return nil
	}

	rendered, err := RenderConfigINI(opts.Render)
	if err != nil {
		return err
	}
	changed, bkp, err := fsutil.WriteTextAtomicIfChanged(dest, rendered, 0o644)
	if err != nil {
		return err
	}
	if bkp != "" {
		fmt.Fprintf(out, "  Backed up existing config.ini -> %s\n", filepath.Base(bkp))
	}
	if changed {
		fmt.Fprintf(
			out,
			"✓ Installed config.ini -> %s (%dx%d, scale %s)\n",
			dest,
			opts.Render.Display.FullscreenWidth,
			opts.Render.Display.FullscreenHeight,
			opts.Render.Display.ScaleString(),
		)
	} else {
		fmt.Fprintf(out, "✓ config.ini already up to date -> %s\n", dest)
	}
	return nil
}

func reportAssetDeploy(out io.Writer, label, backupLabel, dest, bkp string, changed bool) {
	if bkp != "" {
		fmt.Fprintf(out, "  Backed up existing %s -> %s\n", backupLabel, filepath.Base(bkp))
	}
	if changed {
		fmt.Fprintf(out, "✓ Installed %s -> %s\n", label, dest)
	} else {
		fmt.Fprintf(out, "✓ %s already up to date -> %s\n", label, dest)
	}
}

func deployShortcuts(paths *config.Paths, out io.Writer) error {
	raw, err := RenderShortcutsJSON()
	if err != nil {
		return err
	}
	dest := filepath.Join(paths.OpenRCT2Dir, "shortcuts.json")
	changed, bkp, err := fsutil.WriteTextAtomicIfChanged(dest, string(raw), 0o644)
	if err != nil {
		return err
	}
	reportAssetDeploy(out, "shortcuts.json", "shortcuts.json", dest, bkp, changed)
	return nil
}

func deployDesktopFile(paths *config.Paths, execPath string, out io.Writer) error {
	if paths.DesktopDir == "" {
		return nil
	}
	rendered, err := RenderDesktopEntry(execPath)
	if err != nil {
		return err
	}
	dest := filepath.Join(paths.DesktopDir, "OpenRCT2.desktop")
	changed, bkp, err := fsutil.WriteTextAtomicIfChanged(dest, rendered, 0o755)
	if err != nil {
		return err
	}
	reportAssetDeploy(out, "OpenRCT2.desktop", "OpenRCT2.desktop", dest, bkp, changed)
	return nil
}

func deployIcons(paths *config.Paths, out io.Writer) error {
	if paths.HicolorIconDir == "" {
		return nil
	}
	iconBytes, err := fs.ReadFile(openrct2cli.FS, "configs/desktop/openrct2.png")
	if err != nil {
		return fmt.Errorf("reading embedded openrct2.png: %w", err)
	}
	dst := filepath.Join(paths.HicolorIconDir, "openrct2.png")
	changed, bkp, wErr := fsutil.WriteFileAtomicIfChanged(dst, iconBytes, 0o644)
	if wErr != nil {
		return wErr
	}
	reportAssetDeploy(out, "OpenRCT2 icon", "icon", dst, bkp, changed)
	return nil
}

func runBoundedCommand(name string, args ...string) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), desktopCacheTimeout)
	defer cancel()
	_ = exec.CommandContext(ctx, bin, args...).Run()
}

func refreshDesktopCaches(paths *config.Paths) {
	if paths.DesktopDir != "" {
		runBoundedCommand("update-desktop-database", paths.DesktopDir)
	}
	if paths.IconDir != "" {
		hicolorRoot := filepath.Join(paths.IconDir, "hicolor")
		runBoundedCommand("gtk-update-icon-cache", "-q", "-t", hicolorRoot)
	}
}
