// Package config provides platform-idiomatic path resolution and persistent CLI settings.
package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Paths holds resolved platform-idiomatic filesystem paths for OpenRCT2 and rct.
type Paths struct {
	OpenRCT2Dir       string
	SaveDir           string
	PluginDir         string
	AssetPackDir      string
	SequenceDir       string
	ConfigDir         string
	ConfigFile        string
	CatalogDir        string
	StateDir          string
	EngineReceiptFile string
	AddonReceiptsFile string
	BinDir            string
	EngineDir         string
	AppsDir           string
	DesktopDir        string
	IconDir           string
	HicolorIconDir    string
}

// GetPaths returns the resolved platform-idiomatic paths for the current OS.
func GetPaths() *Paths {
	return ResolveFor(runtime.GOOS)
}

// ResolveFor returns platform-idiomatic paths for targetOS ("linux", "darwin", or "windows").
func ResolveFor(targetOS string) *Paths {
	home, _ := os.UserHomeDir()
	if home == "" {
		home = os.Getenv("HOME")
	}

	p := &Paths{}

	switch targetOS {
	case "darwin":
		appSupport := filepath.Join(home, "Library", "Application Support")
		cliRoot := filepath.Join(appSupport, "openrct2-cli")
		p.SetOpenRCT2Dir(filepath.Join(appSupport, "OpenRCT2"))
		p.SetConfigDir(cliRoot)
		p.SetStateDir(filepath.Join(cliRoot, "state"))
		p.BinDir = filepath.Join(home, ".local", "bin")
		p.AppsDir = filepath.Join(home, "Applications")
		p.EngineDir = filepath.Join(p.AppsDir, "OpenRCT2.app")

	case "windows":
		localAppData := os.Getenv("LOCALAPPDATA")
		if localAppData == "" {
			localAppData = filepath.Join(home, "AppData", "Local")
		}
		userProfile := os.Getenv("USERPROFILE")
		if userProfile == "" {
			userProfile = home
		}
		cliRoot := filepath.Join(localAppData, "openrct2-cli")
		p.SetOpenRCT2Dir(filepath.Join(userProfile, "Documents", "OpenRCT2"))
		p.SetConfigDir(cliRoot)
		p.SetStateDir(filepath.Join(cliRoot, "state"))

		// Microsoft FOLDERID_UserProgramFiles standard (%LOCALAPPDATA%\Programs\OpenRCT2)
		programsRoot := filepath.Join(localAppData, "Programs", "OpenRCT2")
		p.BinDir = filepath.Join(programsRoot, "bin")
		p.EngineDir = filepath.Join(programsRoot, "engine")

	default: // "linux" and other XDG Unix platforms
		xdgConfig := os.Getenv("XDG_CONFIG_HOME")
		if xdgConfig == "" {
			xdgConfig = filepath.Join(home, ".config")
		}
		xdgData := os.Getenv("XDG_DATA_HOME")
		if xdgData == "" {
			xdgData = filepath.Join(home, ".local", "share")
		}
		xdgState := os.Getenv("XDG_STATE_HOME")
		if xdgState == "" {
			xdgState = filepath.Join(home, ".local", "state")
		}
		p.SetOpenRCT2Dir(filepath.Join(xdgConfig, "OpenRCT2"))
		p.SetConfigDir(filepath.Join(xdgConfig, "openrct2-cli"))
		p.SetStateDir(filepath.Join(xdgState, "openrct2-cli"))
		p.BinDir = filepath.Join(home, ".local", "bin")
		p.EngineDir = filepath.Join(xdgData, "openrct2-cli", "engine")
		p.DesktopDir = filepath.Join(xdgData, "applications")
		p.IconDir = filepath.Join(xdgData, "icons")
		p.HicolorIconDir = filepath.Join(p.IconDir, "hicolor", "256x256", "apps")
	}

	if envConfig := strings.TrimSpace(os.Getenv("RCT_CONFIG_DIR")); envConfig != "" {
		p.SetConfigDir(envConfig)
		p.SetStateDir(filepath.Join(envConfig, "state"))
	}
	if envState := strings.TrimSpace(os.Getenv("RCT_STATE_DIR")); envState != "" {
		p.SetStateDir(envState)
	}

	return p
}

// SetOpenRCT2Dir updates OpenRCT2Dir and synchronizes SaveDir, PluginDir, AssetPackDir, and SequenceDir.
func (p *Paths) SetOpenRCT2Dir(dir string) {
	p.OpenRCT2Dir = dir
	p.SaveDir = filepath.Join(dir, "save")
	p.PluginDir = filepath.Join(dir, "plugin")
	p.AssetPackDir = filepath.Join(dir, "assetpack")
	p.SequenceDir = filepath.Join(dir, "sequence")
}

// SetConfigDir updates ConfigDir and synchronizes ConfigFile and CatalogDir.
func (p *Paths) SetConfigDir(dir string) {
	p.ConfigDir = dir
	p.ConfigFile = filepath.Join(dir, "config.json")
	p.CatalogDir = filepath.Join(dir, "catalog.d")
}

// SetStateDir updates StateDir and synchronizes EngineReceiptFile and AddonReceiptsFile.
func (p *Paths) SetStateDir(dir string) {
	p.StateDir = dir
	p.EngineReceiptFile = filepath.Join(dir, "engine-receipt.json")
	p.AddonReceiptsFile = filepath.Join(dir, "addon-receipts.json")
}

// SetBinDir updates BinDir.
func (p *Paths) SetBinDir(dir string) {
	p.SetBinDirForOS(runtime.GOOS, dir)
}

// SetBinDirForOS updates BinDir without coupling EngineDir to BinDir.
func (p *Paths) SetBinDirForOS(_, dir string) {
	p.BinDir = dir
}

// SetAppsDir updates AppsDir and synchronizes EngineDir on macOS.
func (p *Paths) SetAppsDir(targetOS, dir string) {
	p.AppsDir = dir
	if targetOS == "darwin" {
		p.EngineDir = filepath.Join(dir, "OpenRCT2.app")
	}
}

// EnginePayloadForOS returns the managed engine payload path inside EngineDir for targetOS.
func (p *Paths) EnginePayloadForOS(targetOS string) string {
	if p == nil {
		return ""
	}
	switch targetOS {
	case "windows":
		return filepath.Join(p.EngineDir, "openrct2.exe")
	case "darwin":
		return filepath.Join(p.EngineDir, "Contents", "MacOS", "OpenRCT2")
	default:
		return filepath.Join(p.EngineDir, "OpenRCT2.AppImage")
	}
}

// ManagedEngineBinary returns the primary OpenRCT2 command path for the current OS.
func (p *Paths) ManagedEngineBinary() string {
	return p.ManagedEngineBinaryForOS(runtime.GOOS)
}

// ManagedEngineBinaryForOS returns the primary OpenRCT2 entrypoint path for targetOS.
func (p *Paths) ManagedEngineBinaryForOS(targetOS string) string {
	if p == nil {
		return ""
	}
	if targetOS == "windows" {
		return filepath.Join(p.EngineDir, "openrct2.exe")
	}
	return filepath.Join(p.BinDir, "openrct2")
}

// ManagedHeadlessBinaryForOS returns the headless CLI executable path on Windows or the main binary on Unix.
func (p *Paths) ManagedHeadlessBinaryForOS(targetOS string) string {
	if p == nil {
		return ""
	}
	if targetOS == "windows" {
		cand := filepath.Join(p.EngineDir, "openrct2-cli.exe")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		return filepath.Join(p.EngineDir, "openrct2.exe")
	}
	return filepath.Join(p.BinDir, "openrct2")
}
