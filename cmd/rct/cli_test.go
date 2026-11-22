package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func execCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&outBuf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

func setupIsolatedEnv(t *testing.T) (openrct2Dir, binDir string) {
	t.Helper()
	root := t.TempDir()
	cfgDir := filepath.Join(root, "rct-cli")
	xdgCfg := filepath.Join(root, "xdg-config")
	xdgData := filepath.Join(root, "xdg-data")
	openrct2Dir = filepath.Join(root, "openrct2-user")
	binDir = filepath.Join(root, "bin")

	t.Setenv("RCT_CONFIG_DIR", cfgDir)
	t.Setenv("XDG_CONFIG_HOME", xdgCfg)
	t.Setenv("XDG_DATA_HOME", xdgData)

	if _, _, err := execCLI(t, "config", "set", "openrct2_dir", openrct2Dir); err != nil {
		t.Fatalf("config set openrct2_dir: %v", err)
	}
	if _, _, err := execCLI(t, "config", "set", "bin_dir", binDir); err != nil {
		t.Fatalf("config set bin_dir: %v", err)
	}
	return openrct2Dir, binDir
}

func TestCLIVersionHelpAndConfigWorkflow(t *testing.T) {
	openrct2Dir, binDir := setupIsolatedEnv(t)

	out, _, err := execCLI(t, "--version")
	if err != nil || !strings.Contains(out, "rct version") {
		t.Fatalf("--version = (%q, %v)", out, err)
	}

	// Switch channel and verify get
	if _, _, err := execCLI(t, "engine", "channel", "develop"); err != nil {
		t.Fatalf("engine channel develop: %v", err)
	}
	out, _, err = execCLI(t, "config", "get", "channel")
	if err != nil || strings.TrimSpace(out) != "develop" {
		t.Fatalf("config get channel = (%q, %v)", out, err)
	}

	// Install configs + shortcuts, verify diff and uninstall
	if _, _, err := execCLI(t, "config", "install", "--force", "--only", "configs"); err != nil {
		t.Fatalf("config install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(openrct2Dir, "config.ini")); err != nil {
		t.Fatalf("expected config.ini installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(openrct2Dir, "shortcuts.json")); err != nil {
		t.Fatalf("expected shortcuts.json installed: %v", err)
	}

	// Create mock engine binary so root --dry-run succeeds across all OSes
	exeName := "openrct2"
	if runtime.GOOS == "windows" {
		exeName = "openrct2.exe"
		engineDir := filepath.Join(filepath.Dir(binDir), "engine")
		_, _, _ = execCLI(t, "config", "set", "engine_dir", engineDir)
		_ = os.MkdirAll(engineDir, 0o755)
		_ = os.WriteFile(filepath.Join(engineDir, exeName), []byte("mock"), 0o755)
	} else {
		_ = os.MkdirAll(binDir, 0o755)
		_ = os.WriteFile(filepath.Join(binDir, exeName), []byte("#!/bin/sh\n"), 0o755)
	}

	out, _, err = execCLI(t, "--dry-run", "--headless", "Forest Frontiers.park", "--port", "11753")
	if err != nil {
		t.Fatalf("root --dry-run: %v", err)
	}
	if !strings.Contains(out, "Forest Frontiers.park") || !strings.Contains(out, "11753") {
		t.Fatalf("unexpected dry-run output: %s", out)
	}

	// Verify addons list, show, type filter validation, custom add, and remove
	out, _, err = execCLI(t, "addons", "list")
	if err != nil || !strings.Contains(out, "ride-vehicle-editor") || !strings.Contains(out, "openmusic") {
		t.Fatalf("addons list = (%q, %v)", out, err)
	}
	if _, _, err := execCLI(t, "addons", "install", "--type", "invalid"); err == nil {
		t.Fatal("expected error for invalid --type in addons install")
	}
	out, _, err = execCLI(t, "plugins", "show", "rve")
	if err != nil || !strings.Contains(out, "Ride Vehicle Editor") {
		t.Fatalf("plugins show rve = (%q, %v)", out, err)
	}
	if _, _, err := execCLI(t, "addons", "add", "test-mod", "--url", "https://example.com/mod.js?dl=1"); err != nil {
		t.Fatalf("addons add: %v", err)
	}
	if _, _, err := execCLI(t, "addons", "remove", "test-mod"); err != nil {
		t.Fatalf("addons remove: %v", err)
	}

	// Verify dynamic shell completion handlers
	compOut, _, err := execCLI(t, "__complete", "config", "set", "")
	if err != nil || !strings.Contains(compOut, "openrct2_dir") {
		t.Fatalf("__complete config set = (%q, %v)", compOut, err)
	}
	compOut, _, err = execCLI(t, "__complete", "config", "set", "channel", "")
	if err != nil || !strings.Contains(compOut, "develop") {
		t.Fatalf("__complete config set channel = (%q, %v)", compOut, err)
	}
	compOut, _, err = execCLI(t, "__complete", "config", "install", "--only", "")
	if err != nil || !strings.Contains(compOut, "shortcuts") {
		t.Fatalf("__complete config install --only = (%q, %v)", compOut, err)
	}
	compOut, _, err = execCLI(t, "__complete", "addons", "install", "")
	if err != nil || !strings.Contains(compOut, "openmusic") || !strings.Contains(compOut, "all") {
		t.Fatalf("__complete addons install = (%q, %v)", compOut, err)
	}

	// Verify addons build synchronizes README.md in a temporary directory
	buildDir := t.TempDir()
	seedReadme := "# Title\n\n## Curated asset packs and plugins\n\noutdated\n\n## Footer\n"
	_ = os.WriteFile(filepath.Join(buildDir, "README.md"), []byte(seedReadme), 0o644)
	if out, _, err := execCLI(t, "addons", "build", buildDir); err != nil || !strings.Contains(out, "Synchronized") {
		t.Fatalf("addons build = (%q, %v)", out, err)
	}

	// Verify games set & status with mock RCT2 directory
	rct2Mock := filepath.Join(t.TempDir(), "RCT2")
	_ = os.MkdirAll(filepath.Join(rct2Mock, "Data"), 0o755)
	_ = os.MkdirAll(filepath.Join(rct2Mock, "ObjData"), 0o755)
	_ = os.WriteFile(filepath.Join(rct2Mock, "Data", "g1.dat"), []byte("g1"), 0o644)
	if _, _, err := execCLI(t, "games", "set", "--rct2", rct2Mock); err != nil {
		t.Fatalf("games set: %v", err)
	}
	out, _, err = execCLI(t, "games", "status")
	if err != nil || !strings.Contains(out, "[✓ Valid]") {
		t.Fatalf("games status = (%q, %v)", out, err)
	}

	// Clean uninstall
	if _, _, err := execCLI(t, "config", "uninstall"); err != nil {
		t.Fatalf("config uninstall: %v", err)
	}
	if _, err := os.Stat(filepath.Join(openrct2Dir, "config.ini")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("expected config.ini removed by uninstall")
	}
}

func TestCLISubcommandsEndToEnd(t *testing.T) {
	openrct2Dir, binDir := setupIsolatedEnv(t)

	// Help, engine status, engine channel get
	if out, _, err := execCLI(t, "--help"); err != nil || !strings.Contains(out, "rct [file|uri]") {
		t.Fatalf("--help failed: %v", err)
	}
	if out, _, err := execCLI(t, "engine", "status"); err != nil || !strings.Contains(out, "Active Channel:") {
		t.Fatalf("engine status = (%q, %v)", out, err)
	}
	if out, _, err := execCLI(t, "engine", "channel"); err != nil || strings.TrimSpace(out) != "release" {
		t.Fatalf("engine channel = (%q, %v)", out, err)
	}

	// Config show, get/set all keys, and error paths
	if out, _, err := execCLI(t, "config", "show"); err != nil || !strings.Contains(out, "CLI Config File:") {
		t.Fatalf("config show = (%q, %v)", out, err)
	}
	for _, kv := range [][2]string{
		{"apps_dir", filepath.Join(t.TempDir(), "Apps")},
		{"rct1_path", filepath.Join(t.TempDir(), "RCT1")},
	} {
		if _, _, err := execCLI(t, "config", "set", kv[0], kv[1]); err != nil {
			t.Fatalf("config set %s: %v", kv[0], err)
		}
		if out, _, err := execCLI(t, "config", "get", kv[0]); err != nil || strings.TrimSpace(out) != kv[1] {
			t.Fatalf("config get %s = (%q, %v)", kv[0], out, err)
		}
	}
	for _, k := range []string{"openrct2_dir", "bin_dir", "engine_dir", "rct2_path"} {
		if _, _, err := execCLI(t, "config", "get", k); err != nil {
			t.Fatalf("config get %s: %v", k, err)
		}
	}
	if _, _, err := execCLI(t, "config", "get", "invalid_key"); err == nil {
		t.Fatal("expected error on unknown config get key")
	}
	if _, _, err := execCLI(t, "config", "set", "invalid_key", "x"); err == nil {
		t.Fatal("expected error on unknown config set key")
	}

	// Games scan with mock Steam library
	steamMock := filepath.Join(t.TempDir(), "Steam")
	rct2Dir := filepath.Join(steamMock, "steamapps", "common", "Roller Coaster Tycoon 2")
	rct1Dir := filepath.Join(steamMock, "steamapps", "common", "RollerCoaster Tycoon Deluxe")
	_ = os.MkdirAll(filepath.Join(rct2Dir, "Data"), 0o755)
	_ = os.MkdirAll(filepath.Join(rct2Dir, "ObjData"), 0o755)
	_ = os.WriteFile(filepath.Join(rct2Dir, "Data", "g1.dat"), []byte("g1"), 0o644)
	_ = os.MkdirAll(filepath.Join(rct1Dir, "Data"), 0o755)
	_ = os.MkdirAll(filepath.Join(rct1Dir, "Scenarios"), 0o755)
	_ = os.WriteFile(filepath.Join(rct1Dir, "Data", "csg1.dat"), []byte("csg1"), 0o644)

	if out, _, err := execCLI(t, "games", "scan", "--root", steamMock, "--dry-run"); err != nil ||
		!strings.Contains(out, "Roller Coaster Tycoon 2") ||
		!strings.Contains(out, "RollerCoaster Tycoon Deluxe") {
		t.Fatalf("games scan --dry-run = (%q, %v)", out, err)
	}
	if _, _, err := execCLI(t, "games", "scan", "--root", steamMock); err != nil {
		t.Fatalf("games scan apply: %v", err)
	}

	// Config diff & sync into temp repo
	if _, _, err := execCLI(t, "config", "install", "--force", "--only", "shortcuts"); err != nil {
		t.Fatalf("config install shortcuts: %v", err)
	}
	if out, _, err := execCLI(t, "config", "diff"); err != nil || !strings.Contains(out, "shortcuts.json Diff") {
		t.Fatalf("config diff = (%q, %v)", out, err)
	}
	tmpRepo := t.TempDir()
	_ = os.MkdirAll(filepath.Join(tmpRepo, "configs", "openrct2"), 0o755)
	_ = os.MkdirAll(filepath.Join(tmpRepo, "configs", "desktop"), 0o755)
	if _, _, err := execCLI(t, "config", "sync", "--repo", tmpRepo); err != nil {
		t.Fatalf("config sync: %v", err)
	}

	// Setup with pre-populated mock engine binary and --skip-addons (zero live network calls)
	if runtime.GOOS == "windows" {
		engDir := filepath.Join(filepath.Dir(binDir), "engine")
		_, _, _ = execCLI(t, "config", "set", "engine_dir", engDir)
		_ = os.MkdirAll(engDir, 0o755)
		_ = os.WriteFile(filepath.Join(engDir, "openrct2.exe"), []byte("mock-exe"), 0o755)
	} else {
		_ = os.MkdirAll(binDir, 0o755)
		_ = os.WriteFile(filepath.Join(binDir, "openrct2"), []byte("#!/bin/sh\n"), 0o755)
	}
	if out, _, err := execCLI(t, "setup", "--skip-addons"); err != nil ||
		!strings.Contains(out, "OpenRCT2 setup complete") {
		t.Fatalf("setup --skip-addons = (%q, %v)", out, err)
	}

	// Addons lint against repository root
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	if out, _, err := execCLI(t, "addons", "lint", repoRoot); err != nil ||
		!strings.Contains(out, "passed validation") {
		t.Fatalf("addons lint = (%q, %v)", out, err)
	}

	// Clean uninstall --all
	if _, _, err := execCLI(t, "config", "uninstall", "--all"); err != nil {
		t.Fatalf("config uninstall --all: %v", err)
	}
	if _, err := os.Stat(filepath.Join(openrct2Dir, "shortcuts.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("expected shortcuts.json removed")
	}
}
