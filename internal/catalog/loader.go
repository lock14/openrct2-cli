package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	openrct2cli "github.com/lock14/openrct2-cli"
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

type kindEnvelope struct {
	SchemaVersion int    `json:"schema_version"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
}

func decodeSingleJSON(data []byte, dst any) error {
	trimmed := bytes.TrimPrefix(bytes.TrimSpace(data), []byte("\xef\xbb\xbf"))
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
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &env); err != nil {
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
		if !IsValidID(ch.ID) {
			return nil, nil, fmt.Errorf("invalid channel id %q", ch.ID)
		}
		ch.SourcePath = srcPath
		ch.Layer = layer
		return &ch, nil, nil

	case KindAddon:
		var ad Addon
		if err := decodeSingleJSON(data, &ad); err != nil {
			return nil, nil, err
		}
		if !IsValidID(ad.ID) {
			return nil, nil, fmt.Errorf("invalid addon id %q", ad.ID)
		}
		if ad.AddonType != AddonTypeAssetPack && ad.AddonType != AddonTypePlugin {
			return nil, nil, fmt.Errorf("invalid addon_type %q", ad.AddonType)
		}
		if ad.Source.Mode != InstallModeFile && ad.Source.Mode != InstallModeZip {
			return nil, nil, fmt.Errorf("invalid source.mode %q", ad.Source.Mode)
		}
		if strings.TrimSpace(ad.Source.URL) == "" {
			return nil, nil, errors.New("source.url is required")
		}
		if len(ad.Targets) == 0 {
			return nil, nil, errors.New("at least one target path is required")
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

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
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
		if os.IsNotExist(err) {
			return cat, nil
		}
		return nil, fmt.Errorf("reading user catalog directory %s: %w", userCatalogDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	addonIdx := make(map[string]int, len(cat.Addons))
	for i, ad := range cat.Addons {
		addonIdx[ad.ID] = i
	}

	for _, name := range names {
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
	sort.Slice(addons, func(i, j int) bool {
		if addons[i].AddonType != addons[j].AddonType {
			return addons[i].AddonType < addons[j].AddonType
		}
		return addons[i].ID < addons[j].ID
	})
}
