package rctcfg

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/display"
)

func makeTestPaths(t *testing.T, targetOS string) *config.Paths {
	t.Helper()
	root := t.TempDir()
	p := config.ResolveFor(targetOS)
	p.SetOpenRCT2Dir(filepath.Join(root, "openrct2"))
	p.SetConfigDir(filepath.Join(root, "cli-config"))
	p.BinDir = filepath.Join(root, "bin")
	p.EngineDir = filepath.Join(root, "engine")
	if targetOS == "linux" {
		p.DesktopDir = filepath.Join(root, "applications")
		p.IconDir = filepath.Join(root, "icons")
		p.HicolorIconDir = filepath.Join(p.IconDir, "hicolor", "256x256", "apps")
	}
	return p
}

func TestDeployIdempotenceSyncDiffAndUninstall(t *testing.T) {
	t.Parallel()
	paths := makeTestPaths(t, "linux")

	// Create a valid mock RCT2 directory and user save file (which Uninstall must preserve)
	rct2Dir := filepath.Join(t.TempDir(), "RCT2")
	if err := os.MkdirAll(filepath.Join(rct2Dir, "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rct2Dir, "ObjData"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rct2Dir, "Data", "g1.dat"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(paths.SaveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	saveFile := filepath.Join(paths.SaveDir, "park.park")
	if err := os.WriteFile(saveFile, []byte("precious-save"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := DeployOptions{
		TargetOS:        "linux",
		Component:       ComponentAll,
		Force:           true,
		SkipCacheUpdate: true,
		Render: RenderOptions{
			Display:    display.BuildInfo(2560, 1440),
			RCT2Path:   rct2Dir,
			SaveDir:    paths.SaveDir,
			PlayerName: "TestBuilder",
			ExecPath:   filepath.Join(paths.BinDir, "openrct2"),
		},
	}

	var out bytes.Buffer
	if err := Deploy(paths, opts, &out); err != nil {
		t.Fatalf("first Deploy: %v", err)
	}

	// Verify no unresolved placeholders remain in deployed files
	cfgBytes, err := os.ReadFile(filepath.Join(paths.OpenRCT2Dir, "config.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cfgBytes), "__") {
		t.Fatalf("unresolved placeholder in config.ini:\n%s", string(cfgBytes))
	}

	// Second identical deploy must NOT create any .bak.* files
	if err := Deploy(paths, opts, &out); err != nil {
		t.Fatalf("second Deploy: %v", err)
	}
	entries, _ := os.ReadDir(paths.OpenRCT2Dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak.") {
			t.Fatalf("idempotence violated: unexpected backup %s", e.Name())
		}
	}

	// Diff should report in sync
	var diffOut bytes.Buffer
	if err := DiffConfigs(paths, opts.Render, "linux", &diffOut); err != nil {
		t.Fatalf("DiffConfigs: %v", err)
	}
	if !strings.Contains(diffOut.String(), "in sync with system") {
		t.Fatalf("expected in sync diff output, got:\n%s", diffOut.String())
	}

	// Sync back into a mock repo directory and verify sanitization removes personal/temp paths
	repoDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoDir, "configs", "openrct2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "configs", "desktop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SyncConfigs(paths, repoDir, "linux", &out); err != nil {
		t.Fatalf("SyncConfigs: %v", err)
	}
	syncedINI, err := os.ReadFile(filepath.Join(repoDir, "configs", "openrct2", "config.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(syncedINI), "TestBuilder") || strings.Contains(string(syncedINI), rct2Dir) {
		t.Fatalf("SyncConfigs leaked personal state:\n%s", string(syncedINI))
	}
	if !strings.Contains(string(syncedINI), "__FULLSCREEN_WIDTH__") ||
		!strings.Contains(string(syncedINI), `"__USER__"`) {
		t.Fatalf("SyncConfigs missing placeholders:\n%s", string(syncedINI))
	}

	// Uninstall must remove config.ini, shortcuts.json, and desktop files while preserving save/park.park
	if err := Uninstall(paths, false, "linux", &out); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(paths.OpenRCT2Dir, "config.ini")); !os.IsNotExist(err) {
		t.Fatal("expected config.ini removed")
	}
	if _, err := os.Stat(saveFile); err != nil {
		t.Fatalf("expected save file preserved, got: %v", err)
	}
}

func TestNonForceDeployBackfillsMissingGamePathWithoutClobbering(t *testing.T) {
	t.Parallel()
	paths := makeTestPaths(t, "darwin")
	if err := os.MkdirAll(paths.OpenRCT2Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgFile := filepath.Join(paths.OpenRCT2Dir, "config.ini")
	userCustom := "[general]\nuncap_fps = false\ngame_path = \"\"\n[interface]\ncurrent_theme = \"*CUSTOM\"\n"
	if err := os.WriteFile(cfgFile, []byte(userCustom), 0o644); err != nil {
		t.Fatal(err)
	}

	rct2Dir := filepath.Join(t.TempDir(), "RCT2")
	_ = os.MkdirAll(filepath.Join(rct2Dir, "Data"), 0o755)
	_ = os.MkdirAll(filepath.Join(rct2Dir, "ObjData"), 0o755)
	_ = os.WriteFile(filepath.Join(rct2Dir, "Data", "g1.dat"), []byte("ok"), 0o644)

	var out bytes.Buffer
	err := Deploy(paths, DeployOptions{
		TargetOS:  "darwin",
		Component: ComponentConfig,
		Force:     false,
		Render:    RenderOptions{RCT2Path: rct2Dir},
	}, &out)
	if err != nil {
		t.Fatalf("Deploy non-force: %v", err)
	}

	gotBytes, _ := os.ReadFile(cfgFile)
	got := string(gotBytes)
	if !strings.Contains(got, `current_theme = "*CUSTOM"`) || !strings.Contains(got, "uncap_fps = false") {
		t.Fatalf("expected user settings preserved, got:\n%s", got)
	}
	gotGamePath, _ := ReadExistingGamePaths(cfgFile)
	if gotGamePath != rct2Dir {
		t.Fatalf("game_path = %q, want %q", gotGamePath, rct2Dir)
	}

	// Verify UpdateConfigGamePaths on fresh and existing config.ini + FormatDesktopExec quoting
	freshPaths := makeTestPaths(t, "windows")
	if err := UpdateConfigGamePaths(freshPaths, rct2Dir, "", &out); err != nil {
		t.Fatalf("UpdateConfigGamePaths fresh: %v", err)
	}
	if err := UpdateConfigGamePaths(freshPaths, rct2Dir, "", &out); err != nil {
		t.Fatalf("UpdateConfigGamePaths existing: %v", err)
	}
	if got := FormatDesktopExec("/opt/My Games/openrct2"); got != `"/opt/My Games/openrct2"` {
		t.Fatalf("FormatDesktopExec with spaces = %q", got)
	}
}
