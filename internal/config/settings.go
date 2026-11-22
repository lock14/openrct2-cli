package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/lock14/openrct2-cli/internal/fsutil"
)

// ChannelRelease is the stable OpenRCT2 release channel identifier.
const ChannelRelease = "release"

// ChannelDevelop is the continuous development OpenRCT2 channel identifier.
const ChannelDevelop = "develop"

// Canonical persistent configuration key names in <ConfigDir>/config.json.
const (
	KeyChannel     = "channel"
	KeyOpenRCT2Dir = "openrct2_dir"
	KeyBinDir      = "bin_dir"
	KeyEngineDir   = "engine_dir"
	KeyAppsDir     = "apps_dir"
	KeyRCT2Path    = "rct2_path"
	KeyRCT1Path    = "rct1_path"
)

var orderedConfigKeys = []string{
	KeyChannel,
	KeyOpenRCT2Dir,
	KeyBinDir,
	KeyEngineDir,
	KeyAppsDir,
	KeyRCT2Path,
	KeyRCT1Path,
}

var knownConfigKeys = map[string]bool{
	KeyChannel:     true,
	KeyOpenRCT2Dir: true,
	KeyBinDir:      true,
	KeyEngineDir:   true,
	KeyAppsDir:     true,
	KeyRCT2Path:    true,
	KeyRCT1Path:    true,
}

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

// ValidKeys returns a defensive copy of all supported CLI configuration keys.
func ValidKeys() []string {
	return slices.Clone(orderedConfigKeys)
}

// NormalizeKey normalizes a user-supplied key name (accepting hyphenated or underscored forms)
// and validates that it is a supported CLI configuration key.
func NormalizeKey(raw string) (string, error) {
	norm := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(raw)), "-", "_")
	if knownConfigKeys[norm] {
		return norm, nil
	}
	return "", fmt.Errorf(
		"unknown configuration key %q (valid: %s)",
		raw,
		strings.Join(orderedConfigKeys, ", "),
	)
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

// Set validates and updates a configuration field on c, returning the canonical key and stored value.
func (c *AppConfig) Set(rawKey, rawVal string) (string, string, error) {
	if c == nil {
		return "", "", errors.New("nil application config")
	}
	key, err := NormalizeKey(rawKey)
	if err != nil {
		return "", "", err
	}
	val := strings.TrimSpace(rawVal)

	switch key {
	case KeyChannel:
		norm, chErr := NormalizeChannel(val)
		if chErr != nil {
			return "", "", chErr
		}
		c.Channel = norm
		return key, norm, nil
	case KeyOpenRCT2Dir:
		c.OpenRCT2Dir = val
	case KeyBinDir:
		c.BinDir = val
	case KeyEngineDir:
		c.EngineDir = val
	case KeyAppsDir:
		c.AppsDir = val
	case KeyRCT2Path:
		c.RCT2Path = val
	case KeyRCT1Path:
		c.RCT1Path = val
	}
	return key, val, nil
}

// GetEffective retrieves the effective resolved value for rawKey across paths and cfg.
func GetEffective(paths *Paths, cfg *AppConfig, rawKey string) (string, error) {
	key, err := NormalizeKey(rawKey)
	if err != nil {
		return "", err
	}
	switch key {
	case KeyChannel:
		return cfg.EffectiveChannel(), nil
	case KeyOpenRCT2Dir:
		if paths != nil {
			return paths.OpenRCT2Dir, nil
		}
		return "", nil
	case KeyBinDir:
		if paths != nil {
			return paths.BinDir, nil
		}
		return "", nil
	case KeyEngineDir:
		if paths != nil {
			return paths.EngineDir, nil
		}
		return "", nil
	case KeyAppsDir:
		if paths != nil {
			return paths.AppsDir, nil
		}
		return "", nil
	case KeyRCT2Path:
		if cfg != nil {
			return cfg.RCT2Path, nil
		}
		return "", nil
	case KeyRCT1Path:
		if cfg != nil {
			return cfg.RCT1Path, nil
		}
		return "", nil
	default:
		return "", fmt.Errorf("unhandled configuration key %q", key)
	}
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
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, err
	}

	data = fsutil.StripUTF8BOM(data)
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
	setNonEmpty := func(key, val string) {
		if s := strings.TrimSpace(val); s != "" {
			merged[key] = s
		}
	}
	setNonEmpty(KeyChannel, cfg.Channel)
	setNonEmpty(KeyOpenRCT2Dir, cfg.OpenRCT2Dir)
	setNonEmpty(KeyBinDir, cfg.BinDir)
	setNonEmpty(KeyEngineDir, cfg.EngineDir)
	setNonEmpty(KeyAppsDir, cfg.AppsDir)
	setNonEmpty(KeyRCT2Path, cfg.RCT2Path)
	setNonEmpty(KeyRCT1Path, cfg.RCT1Path)

	_, _, err := fsutil.WriteJSONAtomicIfChanged(paths.ConfigFile, merged, 0o644)
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
		p.SetBinDir(s)
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
