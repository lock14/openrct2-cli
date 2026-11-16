package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lock14/openrct2-cli/internal/fsutil"
)

// ChannelRelease is the stable OpenRCT2 release channel identifier.
const ChannelRelease = "release"

// ChannelDevelop is the continuous development OpenRCT2 channel identifier.
const ChannelDevelop = "develop"

// AppConfig represents persistent user settings for openrct2-cli in <ConfigDir>/config.json.
type AppConfig struct {
	Channel     string `json:"channel,omitempty"`
	OpenRCT2Dir string `json:"openrct2_dir,omitempty"`
	BinDir      string `json:"bin_dir,omitempty"`
	EngineDir   string `json:"engine_dir,omitempty"`
	AppsDir     string `json:"apps_dir,omitempty"`
	RCT2Path    string `json:"rct2_path,omitempty"`
	RCT1Path    string `json:"rct1_path,omitempty"`

	extra map[string]json.RawMessage
}

var knownConfigKeys = map[string]bool{
	"channel":      true,
	"openrct2_dir": true,
	"bin_dir":      true,
	"engine_dir":   true,
	"apps_dir":     true,
	"rct2_path":    true,
	"rct1_path":    true,
}

// NormalizeChannel validates and normalizes a release channel identifier ("release" or "develop").
func NormalizeChannel(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "release", "stable":
		return ChannelRelease, nil
	case "develop", "dev", "nightly":
		return ChannelDevelop, nil
	default:
		return "", fmt.Errorf("invalid channel %q (expected %q or %q)", raw, ChannelRelease, ChannelDevelop)
	}
}

// EffectiveChannel returns the normalized active engine release channel, defaulting to "release".
func (c *AppConfig) EffectiveChannel() string {
	if c == nil {
		return ChannelRelease
	}
	ch, err := NormalizeChannel(c.Channel)
	if err != nil {
		return ChannelRelease
	}
	return ch
}

// LoadConfig reads <ConfigDir>/config.json if present, returning defaults when absent.
// Unknown top-level JSON keys are preserved in memory across SaveConfig round-trips.
func LoadConfig(paths *Paths) (*AppConfig, error) {
	cfg := &AppConfig{}
	if paths == nil || paths.ConfigFile == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(paths.ConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, err
	}
	for k, v := range rawMap {
		if !knownConfigKeys[strings.ToLower(k)] {
			if cfg.extra == nil {
				cfg.extra = make(map[string]json.RawMessage)
			}
			cfg.extra[k] = v
		}
	}

	return cfg, nil
}

// SaveConfig writes <ConfigDir>/config.json atomically if changed, preserving unknown keys.
func SaveConfig(paths *Paths, cfg *AppConfig) error {
	if paths == nil || paths.ConfigFile == "" {
		return nil
	}
	if cfg == nil {
		cfg = &AppConfig{}
	}

	merged := make(map[string]any, len(cfg.extra)+len(knownConfigKeys))
	for k, v := range cfg.extra {
		if !knownConfigKeys[strings.ToLower(k)] {
			merged[k] = v
		}
	}
	if s := strings.TrimSpace(cfg.Channel); s != "" {
		merged["channel"] = s
	}
	if s := strings.TrimSpace(cfg.OpenRCT2Dir); s != "" {
		merged["openrct2_dir"] = s
	}
	if s := strings.TrimSpace(cfg.BinDir); s != "" {
		merged["bin_dir"] = s
	}
	if s := strings.TrimSpace(cfg.EngineDir); s != "" {
		merged["engine_dir"] = s
	}
	if s := strings.TrimSpace(cfg.AppsDir); s != "" {
		merged["apps_dir"] = s
	}
	if s := strings.TrimSpace(cfg.RCT2Path); s != "" {
		merged["rct2_path"] = s
	}
	if s := strings.TrimSpace(cfg.RCT1Path); s != "" {
		merged["rct1_path"] = s
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(merged); err != nil {
		return err
	}

	_, _, err := fsutil.WriteFileAtomicIfChanged(paths.ConfigFile, buf.Bytes(), 0o644)
	return err
}

// ApplyConfigPaths applies non-empty directory overrides from cfg onto p for targetOS.
func ApplyConfigPaths(p *Paths, cfg *AppConfig, targetOS string) {
	if p == nil || cfg == nil {
		return
	}
	if s := strings.TrimSpace(cfg.OpenRCT2Dir); s != "" {
		p.SetOpenRCT2Dir(s)
	}
	if s := strings.TrimSpace(cfg.BinDir); s != "" {
		p.SetBinDirForOS(targetOS, s)
	}
	if s := strings.TrimSpace(cfg.AppsDir); s != "" {
		p.SetAppsDir(targetOS, s)
	}
	if s := strings.TrimSpace(cfg.EngineDir); s != "" {
		p.EngineDir = s
	}
}

// ValueSource returns "config" when cfgVal is non-empty, or "default" otherwise.
func ValueSource(cfgVal string) string {
	if strings.TrimSpace(cfgVal) != "" {
		return "config"
	}
	return "default"
}
