package catalog

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	openrct2cli "github.com/lock14/openrct2-cli"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

var (
	idRegex           = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	personalPathRegex = regexp.MustCompile(
		`(?i)(/home/[a-z0-9_-]+|/Users/[a-z0-9_-]+|C:(?:\\+|/)Users(?:\\+|/)[a-z0-9_-]+)`,
	)
)

// IsValidID reports whether id conforms to lowercase kebab-case identifier syntax.
func IsValidID(id string) bool {
	return idRegex.MatchString(strings.TrimSpace(id))
}

// ScanPersonalPath returns the first matched personal home path in text, or "".
func ScanPersonalPath(text string) string {
	return personalPathRegex.FindString(text)
}

func decodeSingleJSON(data []byte, dst any) error {
	trimmed := fsutil.StripUTF8BOM(bytes.TrimSpace(data))
	if len(trimmed) == 0 {
		return errors.New("empty JSON document")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return errors.New("unexpected trailing data after JSON object")
	}
	return nil
}

// ValidateChannel verifies and normalizes structural invariants on ch.
func ValidateChannel(ch *ChannelConfig) error {
	if ch == nil {
		return errors.New("nil channel entity")
	}
	if !IsValidID(ch.ID) {
		return fmt.Errorf("invalid channel id %q", ch.ID)
	}
	return nil
}

// ValidateAddon verifies and normalizes structural, checksum, and relative-path invariants on ad.
func ValidateAddon(ad *Addon) error {
	if ad == nil {
		return errors.New("nil addon entity")
	}
	if !IsValidID(ad.ID) {
		return fmt.Errorf("invalid addon id %q (use lowercase letters, digits, and hyphens)", ad.ID)
	}
	if ad.AddonType != AddonTypeAssetPack && ad.AddonType != AddonTypePlugin {
		return fmt.Errorf(
			"invalid addon_type %q (expected %q or %q)",
			ad.AddonType, AddonTypePlugin, AddonTypeAssetPack,
		)
	}
	if ad.Source.Mode != InstallModeFile && ad.Source.Mode != InstallModeZip {
		return fmt.Errorf(
			"invalid source.mode %q (expected %q or %q)",
			ad.Source.Mode, InstallModeFile, InstallModeZip,
		)
	}
	url := strings.TrimSpace(ad.Source.URL)
	if url == "" {
		return errors.New("source.url is required")
	}
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
		return fmt.Errorf("source URL %q must use https:// or http://", ad.Source.URL)
	}
	ad.Source.URL = url

	normSum, err := fsutil.ValidateSHA256Hex(ad.Source.SHA256)
	if err != nil {
		return fmt.Errorf("invalid source.sha256: %w", err)
	}
	ad.Source.SHA256 = normSum

	if ad.Source.Mode == InstallModeFile {
		cleanRel, relErr := fsutil.CleanRel(ad.Source.TargetPath)
		if relErr != nil {
			return fmt.Errorf("invalid target_path: %w", relErr)
		}
		ad.Source.TargetPath = cleanRel
		if len(ad.Targets) == 0 {
			ad.Targets = []string{cleanRel}
		}
	} else if trimmedDir := strings.TrimSpace(ad.Source.TargetDir); trimmedDir != "" && trimmedDir != "." {
		cleanDir, dirErr := fsutil.CleanRel(trimmedDir)
		if dirErr != nil {
			return fmt.Errorf("invalid target_dir: %w", dirErr)
		}
		ad.Source.TargetDir = cleanDir
	}

	if len(ad.Targets) == 0 {
		return errors.New("at least one target path is required")
	}
	for i, t := range ad.Targets {
		cleanT, relErr := fsutil.CleanRel(t)
		if relErr != nil {
			return fmt.Errorf("invalid target %q: %w", t, relErr)
		}
		ad.Targets[i] = cleanT
	}
	return nil
}

// DecodeEntityBytes decodes a single ChannelConfig or Addon JSON document strictly.
func DecodeEntityBytes(data []byte, srcPath string, layer Layer) (*ChannelConfig, *Addon, error) {
	if layer == LayerCurated {
		if match := ScanPersonalPath(string(data)); match != "" {
			return nil, nil, fmt.Errorf("curated entity contains personal path %q", match)
		}
	}

	var env struct {
		SchemaVersion int    `json:"schema_version"`
		Kind          string `json:"kind"`
	}
	if err := json.Unmarshal(fsutil.StripUTF8BOM(data), &env); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if env.SchemaVersion != SchemaVersion {
		return nil, nil, fmt.Errorf("unsupported schema_version %d (expected %d)", env.SchemaVersion, SchemaVersion)
	}

	switch env.Kind {
	case KindChannel:
		var ch ChannelConfig
		if err := decodeSingleJSON(data, &ch); err != nil {
			return nil, nil, err
		}
		if err := ValidateChannel(&ch); err != nil {
			return nil, nil, err
		}
		ch.SourcePath = srcPath
		ch.Layer = layer
		return &ch, nil, nil

	case KindAddon:
		var ad Addon
		if err := decodeSingleJSON(data, &ad); err != nil {
			return nil, nil, err
		}
		if len(ad.Targets) == 0 {
			return nil, nil, errors.New("at least one target path is required")
		}
		if err := ValidateAddon(&ad); err != nil {
			return nil, nil, err
		}
		ad.SourcePath = srcPath
		ad.Layer = layer
		return nil, &ad, nil

	default:
		return nil, nil, fmt.Errorf("unknown entity kind %q", env.Kind)
	}
}

// LoadCurated loads the embedded curated catalog entities from openrct2cli.FS.
func LoadCurated() (*Catalog, error) {
	return LoadFromFS(openrct2cli.FS, "catalog", LayerCurated)
}

func sortedJSONEntryNames(entries []fs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			names = append(names, e.Name())
		}
	}
	slices.Sort(names)
	return names
}

// LoadFromFS loads all *.json catalog entities from dir inside fsys.
func LoadFromFS(fsys fs.FS, dir string, layer Layer) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading catalog directory %s: %w", dir, err)
	}

	cat := &Catalog{
		Channels: make(map[string]ChannelConfig),
	}
	addonIndex := make(map[string]int)

	for _, name := range sortedJSONEntryNames(entries) {
		relPath := path.Join(dir, name)
		data, readErr := fs.ReadFile(fsys, relPath)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", relPath, readErr)
		}
		ch, ad, decErr := DecodeEntityBytes(data, relPath, layer)
		if decErr != nil {
			return nil, fmt.Errorf("%s: %w", relPath, decErr)
		}
		if ch != nil {
			expected := fmt.Sprintf("channel.%s.json", ch.ID)
			if name != expected {
				return nil, fmt.Errorf("%s: filename must be %s", relPath, expected)
			}
			if _, dup := cat.Channels[ch.ID]; dup {
				return nil, fmt.Errorf("duplicate channel id %q in %s", ch.ID, relPath)
			}
			cat.Channels[ch.ID] = *ch
		}
		if ad != nil {
			expected := fmt.Sprintf("addon.%s.json", ad.ID)
			if name != expected {
				return nil, fmt.Errorf("%s: filename must be %s", relPath, expected)
			}
			if _, dup := addonIndex[ad.ID]; dup {
				return nil, fmt.Errorf("duplicate addon id %q in %s", ad.ID, relPath)
			}
			addonIndex[ad.ID] = len(cat.Addons)
			cat.Addons = append(cat.Addons, *ad)
		}
	}

	sortAddons(cat.Addons)
	return cat, nil
}

// LoadLayered loads curated entities and overlays user entities from userCatalogDir.
// When strict is false, malformed user JSON files are quarantined into cat.Diagnostics.
func LoadLayered(userCatalogDir string, strict bool) (*Catalog, error) {
	cat, err := LoadCurated()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(userCatalogDir) == "" {
		return cat, nil
	}

	entries, err := os.ReadDir(userCatalogDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cat, nil
		}
		return nil, fmt.Errorf("reading user catalog directory %s: %w", userCatalogDir, err)
	}

	addonIdx := make(map[string]int, len(cat.Addons))
	for i, ad := range cat.Addons {
		addonIdx[ad.ID] = i
	}

	for _, name := range sortedJSONEntryNames(entries) {
		fullPath := filepath.Join(userCatalogDir, name)
		data, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			if strict {
				return nil, readErr
			}
			cat.Diagnostics = append(cat.Diagnostics, Diagnostic{Path: fullPath, Message: readErr.Error()})
			continue
		}
		ch, ad, decErr := DecodeEntityBytes(data, fullPath, LayerUser)
		if decErr != nil {
			if strict {
				return nil, fmt.Errorf("%s: %w", fullPath, decErr)
			}
			cat.Diagnostics = append(cat.Diagnostics, Diagnostic{
				Path:    fullPath,
				Message: fmt.Sprintf("quarantined invalid user entity (%v)", decErr),
			})
			continue
		}
		if ch != nil {
			cat.Channels[ch.ID] = *ch
		}
		if ad != nil {
			if idx, exists := addonIdx[ad.ID]; exists {
				cat.Addons[idx] = *ad
			} else {
				addonIdx[ad.ID] = len(cat.Addons)
				cat.Addons = append(cat.Addons, *ad)
			}
		}
	}

	sortAddons(cat.Addons)
	return cat, nil
}

func sortAddons(addons []Addon) {
	slices.SortFunc(addons, func(a, b Addon) int {
		return cmp.Or(
			cmp.Compare(a.AddonType, b.AddonType),
			cmp.Compare(a.ID, b.ID),
		)
	})
}
