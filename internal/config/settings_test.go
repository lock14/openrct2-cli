package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveForPlatforms(t *testing.T) {
	t.Setenv("RCT_CONFIG_DIR", "")
	t.Setenv("RCT_STATE_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	linuxPaths := ResolveFor("linux")
	if !strings.HasSuffix(filepath.ToSlash(linuxPaths.OpenRCT2Dir), "/.config/OpenRCT2") {
		t.Errorf("linux OpenRCT2Dir = %q", linuxPaths.OpenRCT2Dir)
	}
	if !strings.HasSuffix(filepath.ToSlash(linuxPaths.StateDir), "/.local/state/openrct2-cli") {
		t.Errorf("linux StateDir = %q", linuxPaths.StateDir)
	}
	if !strings.HasSuffix(filepath.ToSlash(linuxPaths.EngineDir), "/.local/share/openrct2-cli/engine") {
		t.Errorf("linux EngineDir = %q", linuxPaths.EngineDir)
	}
	if !strings.HasSuffix(filepath.ToSlash(linuxPaths.DesktopDir), "/.local/share/applications") {
		t.Errorf("linux DesktopDir = %q", linuxPaths.DesktopDir)
	}
	if !strings.HasSuffix(filepath.ToSlash(linuxPaths.ManagedEngineBinaryForOS("linux")), "/.local/bin/openrct2") {
		t.Errorf("linux ManagedEngineBinary = %q", linuxPaths.ManagedEngineBinaryForOS("linux"))
	}

	darwinPaths := ResolveFor("darwin")
	if !strings.HasSuffix(filepath.ToSlash(darwinPaths.OpenRCT2Dir), "/Library/Application Support/OpenRCT2") {
		t.Errorf("darwin OpenRCT2Dir = %q", darwinPaths.OpenRCT2Dir)
	}
	if !strings.HasSuffix(filepath.ToSlash(darwinPaths.StateDir), "/Library/Application Support/openrct2-cli/state") {
		t.Errorf("darwin StateDir = %q", darwinPaths.StateDir)
	}
	if !strings.HasSuffix(filepath.ToSlash(darwinPaths.EngineDir), "/Applications/OpenRCT2.app") {
		t.Errorf("darwin EngineDir = %q", darwinPaths.EngineDir)
	}

	winPaths := ResolveFor("windows")
	if !strings.HasSuffix(filepath.ToSlash(winPaths.OpenRCT2Dir), "/Documents/OpenRCT2") {
		t.Errorf("windows OpenRCT2Dir = %q", winPaths.OpenRCT2Dir)
	}
	if !strings.HasSuffix(filepath.ToSlash(winPaths.BinDir), "/Programs/OpenRCT2/bin") {
		t.Errorf("windows BinDir = %q", winPaths.BinDir)
	}
	winBin := filepath.ToSlash(winPaths.ManagedEngineBinaryForOS("windows"))
	if !strings.HasSuffix(winBin, "/Programs/OpenRCT2/engine/openrct2.exe") {
		t.Errorf("windows ManagedEngineBinary = %q", winPaths.ManagedEngineBinaryForOS("windows"))
	}
}

func TestSaveAndLoadConfigPreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	p := ResolveFor("linux")
	p.SetConfigDir(dir)

	initial := []byte("{\n  \"channel\": \"develop\",\n  \"future_key\": 42\n}\n")
	if _, err := LoadConfig(p); err != nil {
		t.Fatalf("empty load: %v", err)
	}
	if err := SaveConfig(p, &AppConfig{Channel: "release"}); err != nil {
		t.Fatalf("save initial: %v", err)
	}
	if err := writeRaw(p.ConfigFile, initial); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.EffectiveChannel() != ChannelDevelop {
		t.Fatalf("EffectiveChannel = %q, want %q", cfg.EffectiveChannel(), ChannelDevelop)
	}
	cfg.BinDir = "/opt/custom/bin"
	if err := SaveConfig(p, cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}

	reloaded, err := LoadConfig(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.BinDir != "/opt/custom/bin" {
		t.Fatalf("BinDir = %q", reloaded.BinDir)
	}
	if _, ok := reloaded.extra["future_key"]; !ok {
		t.Fatal("expected unknown future_key preserved across SaveConfig")
	}
}

func TestNormalizeChannelAndKeyAccessors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", ChannelRelease, false},
		{"release", ChannelRelease, false},
		{"stable", ChannelRelease, false},
		{"develop", ChannelDevelop, false},
		{"dev", ChannelDevelop, false},
		{"nightly", ChannelDevelop, false},
		{"bogus", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeChannel(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("NormalizeChannel(%q) = (%q, %v), want (%q, err=%v)", tc.in, got, err, tc.want, tc.wantErr)
		}
	}

	keys := ValidKeys()
	if len(keys) != 7 {
		t.Fatalf("ValidKeys = %v, want 7 keys", keys)
	}
	keys[0] = "mutated"
	if ValidKeys()[0] == "mutated" {
		t.Fatal("ValidKeys must return a defensive copy")
	}

	cfg := &AppConfig{}
	p := ResolveFor("linux")
	key, val, err := cfg.Set("openrct2-dir", "/tmp/custom-openrct2")
	if err != nil || key != KeyOpenRCT2Dir || val != "/tmp/custom-openrct2" {
		t.Fatalf("cfg.Set(openrct2-dir) = (%q, %q, %v)", key, val, err)
	}
	ApplyConfigPaths(p, cfg, "linux")
	eff, err := GetEffective(p, cfg, "openrct2-dir")
	if err != nil || eff != "/tmp/custom-openrct2" {
		t.Fatalf("GetEffective = (%q, %v)", eff, err)
	}
	if _, _, err := cfg.Set("channel", "invalid"); err == nil {
		t.Fatal("expected invalid channel error in cfg.Set")
	}
	if _, err := NormalizeKey("nonexistent"); err == nil {
		t.Fatal("expected unknown key error")
	}
}

func TestApplyConfigPathsAndHelpers(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	p := ResolveFor("windows")
	engDir := filepath.Join(tmp, "engine")
	cfg := &AppConfig{
		OpenRCT2Dir: filepath.Join(tmp, "user"),
		BinDir:      filepath.Join(tmp, "bin"),
		EngineDir:   engDir,
		AppsDir:     filepath.Join(tmp, "Apps"),
	}
	ApplyConfigPaths(p, cfg, "windows")
	if p.OpenRCT2Dir != cfg.OpenRCT2Dir || p.BinDir != cfg.BinDir || p.EngineDir != engDir {
		t.Fatalf("ApplyConfigPaths mismatch: %+v", p)
	}
	if ValueSource("") != "default" || ValueSource("set") != "config" {
		t.Fatal("ValueSource mismatch")
	}
	if !strings.HasSuffix(p.ManagedHeadlessBinaryForOS("windows"), "openrct2.exe") {
		t.Fatalf("expected openrct2.exe fallback, got %s", p.ManagedHeadlessBinaryForOS("windows"))
	}
	_ = writeMkdirFile(filepath.Join(engDir, "openrct2-cli.exe"), []byte("cli"))
	if !strings.HasSuffix(p.ManagedHeadlessBinaryForOS("windows"), "openrct2-cli.exe") {
		t.Fatalf("expected openrct2-cli.exe, got %s", p.ManagedHeadlessBinaryForOS("windows"))
	}
}
