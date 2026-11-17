// Package catalog manages declarative OpenRCT2 engine channels, asset packs, and JavaScript plugins.
package catalog

import "fmt"

// Layer identifies whether a catalog entity comes from embedded defaults or user catalog.d.
type Layer string

const (
	// LayerCurated marks embedded read-only entities shipped in catalog/*.json.
	LayerCurated Layer = "curated"
	// LayerUser marks user-defined entities loaded from <ConfigDir>/catalog.d/*.json.
	LayerUser Layer = "user"
)

const (
	// SchemaVersion is the supported catalog JSON schema version.
	SchemaVersion = 1
	// KindChannel identifies an OpenRCT2 engine release channel entity.
	KindChannel = "channel"
	// KindAddon identifies an OpenRCT2 asset pack or JavaScript plugin entity.
	KindAddon = "addon"
	// AddonTypeAssetPack identifies official or custom audio/music/sequence asset packs.
	AddonTypeAssetPack = "assetpack"
	// AddonTypePlugin identifies OpenRCT2 JavaScript (.js) plugins.
	AddonTypePlugin = "plugin"
	// InstallModeFile writes a single downloaded file directly to Source.TargetPath.
	InstallModeFile = "file"
	// InstallModeZip extracts a downloaded ZIP archive safely under Source.TargetDir.
	InstallModeZip = "zip"
)

// PinnedAsset defines a deterministic fallback release URL and SHA-256 checksum.
type PinnedAsset struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256,omitempty"`
}

// PlatformRecipe defines GitHub Releases asset regexes and pinned fallback per OS/arch.
type PlatformRecipe struct {
	Assets      []string     `json:"assets,omitempty"`
	Pinned      *PinnedAsset `json:"pinned,omitempty"`
	Unsupported bool         `json:"unsupported,omitempty"`
}

// ChannelConfig represents an OpenRCT2 release channel ("release" or "develop").
type ChannelConfig struct {
	SchemaVersion int                       `json:"schema_version"`
	Kind          string                    `json:"kind"`
	ID            string                    `json:"id"`
	DisplayName   string                    `json:"display_name"`
	Description   string                    `json:"description,omitempty"`
	Aliases       []string                  `json:"aliases,omitempty"`
	Repo          string                    `json:"repo"`
	Platforms     map[string]PlatformRecipe `json:"platforms"`

	SourcePath string `json:"-"`
	Layer      Layer  `json:"-"`
}

// EffectiveDisplayName returns DisplayName when non-empty, falling back to ID.
func (c ChannelConfig) EffectiveDisplayName() string {
	if c.DisplayName != "" {
		return c.DisplayName
	}
	return c.ID
}

// AddonSource defines how an add-on payload is downloaded and unpacked.
type AddonSource struct {
	Repo       string `json:"repo,omitempty"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256,omitempty"`
	Mode       string `json:"mode"`
	TargetDir  string `json:"target_dir,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
}

// Addon represents a curated or custom OpenRCT2 asset pack or JavaScript plugin.
type Addon struct {
	SchemaVersion int         `json:"schema_version"`
	Kind          string      `json:"kind"`
	ID            string      `json:"id"`
	DisplayName   string      `json:"display_name"`
	AddonType     string      `json:"addon_type"`
	Category      string      `json:"category,omitempty"`
	Author        string      `json:"author,omitempty"`
	Version       string      `json:"version,omitempty"`
	Description   string      `json:"description,omitempty"`
	Aliases       []string    `json:"aliases,omitempty"`
	Homepage      string      `json:"homepage,omitempty"`
	Source        AddonSource `json:"source"`
	Targets       []string    `json:"targets"`

	SourcePath string `json:"-"`
	Layer      Layer  `json:"-"`
}

// EffectiveDisplayName returns DisplayName when non-empty, falling back to ID.
func (a Addon) EffectiveDisplayName() string {
	if a.DisplayName != "" {
		return a.DisplayName
	}
	return a.ID
}

// Diagnostic records a non-fatal warning when quarantining malformed user catalog files.
type Diagnostic struct {
	Path    string
	Message string
}

func (d Diagnostic) String() string {
	return fmt.Sprintf("%s: %s", d.Path, d.Message)
}

// Catalog holds the merged channel and add-on definitions across curated and user layers.
type Catalog struct {
	Channels    map[string]ChannelConfig
	Addons      []Addon
	Diagnostics []Diagnostic
}
