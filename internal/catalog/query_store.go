package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"unicode"

	"github.com/lock14/openrct2-cli/internal/fsutil"
)

// NormalizeQuery strips punctuation/whitespace and lowercases s for tolerant lookup.
func NormalizeQuery(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func acronymOf(displayName string) string {
	var b strings.Builder
	for _, word := range strings.FieldsFunc(displayName, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if word != "" {
			b.WriteRune(unicode.ToLower([]rune(word)[0]))
		}
	}
	return b.String()
}

// FindChannel resolves a channel by ID or alias (such as "release", "stable", "develop", "dev").
func (c *Catalog) FindChannel(query string) *ChannelConfig {
	if c == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		q = "release"
	}
	if ch, ok := c.Channels[q]; ok {
		return &ch
	}
	for _, ch := range c.Channels {
		for _, alias := range ch.Aliases {
			if strings.EqualFold(alias, q) {
				copyCh := ch
				return &copyCh
			}
		}
	}
	return nil
}

// FindAddon resolves an Addon using tiered matching: exact ID, alias, normalized display name,
// target filename stem, and multi-word acronym.
func (c *Catalog) FindAddon(query string) *Addon {
	if c == nil {
		return nil
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil
	}
	norm := NormalizeQuery(trimmed)

	// Tier 1: Exact ID
	for i := range c.Addons {
		if strings.EqualFold(c.Addons[i].ID, trimmed) {
			return &c.Addons[i]
		}
	}

	// Tier 2: Declared aliases
	for i := range c.Addons {
		for _, alias := range c.Addons[i].Aliases {
			if strings.EqualFold(alias, trimmed) || (norm != "" && NormalizeQuery(alias) == norm) {
				return &c.Addons[i]
			}
		}
	}

	// Tier 3: Normalized ID or DisplayName
	for i := range c.Addons {
		if norm != "" && (NormalizeQuery(c.Addons[i].ID) == norm || NormalizeQuery(c.Addons[i].DisplayName) == norm) {
			return &c.Addons[i]
		}
	}

	// Tier 4: Target filename or stem
	stemQuery := NormalizeQuery(strings.TrimSuffix(strings.ToLower(trimmed), ".js"))
	for i := range c.Addons {
		for _, target := range c.Addons[i].Targets {
			base := path.Base(target)
			stem := strings.TrimSuffix(base, path.Ext(base))
			if strings.EqualFold(base, trimmed) || (stemQuery != "" && NormalizeQuery(stem) == stemQuery) {
				return &c.Addons[i]
			}
		}
	}

	// Tier 5: Acronym shorthand (e.g. "rve" -> "Ride Vehicle Editor")
	if len(norm) >= 2 {
		for i := range c.Addons {
			if acronymOf(c.Addons[i].DisplayName) == norm {
				return &c.Addons[i]
			}
		}
	}

	return nil
}

// SaveUserAddon validates and persists a custom user Addon to <catalogDir>/addon.<id>.json.
func SaveUserAddon(catalogDir string, addon Addon) (string, error) {
	if strings.TrimSpace(catalogDir) == "" {
		return "", errors.New("empty user catalog directory")
	}
	addon.SchemaVersion = SchemaVersion
	addon.Kind = KindAddon
	addon.ID = strings.ToLower(strings.TrimSpace(addon.ID))
	if !IsValidID(addon.ID) {
		return "", fmt.Errorf("invalid addon id %q (use lowercase letters, digits, and hyphens)", addon.ID)
	}
	if addon.AddonType != AddonTypePlugin && addon.AddonType != AddonTypeAssetPack {
		return "", fmt.Errorf(
			"invalid addon type %q (expected %q or %q)",
			addon.AddonType, AddonTypePlugin, AddonTypeAssetPack,
		)
	}
	if addon.Source.Mode != InstallModeFile && addon.Source.Mode != InstallModeZip {
		return "", fmt.Errorf(
			"invalid install mode %q (expected %q or %q)",
			addon.Source.Mode, InstallModeFile, InstallModeZip,
		)
	}
	if !strings.HasPrefix(addon.Source.URL, "https://") && !strings.HasPrefix(addon.Source.URL, "http://") {
		return "", fmt.Errorf("source URL %q must use https:// or http://", addon.Source.URL)
	}
	if addon.Source.Mode == InstallModeFile {
		cleanRel, err := fsutil.CleanRel(addon.Source.TargetPath)
		if err != nil {
			return "", fmt.Errorf("invalid target_path: %w", err)
		}
		addon.Source.TargetPath = cleanRel
		if len(addon.Targets) == 0 {
			addon.Targets = []string{cleanRel}
		}
	} else if addon.Source.TargetDir != "" && addon.Source.TargetDir != "." {
		cleanDir, err := fsutil.CleanRel(addon.Source.TargetDir)
		if err != nil {
			return "", fmt.Errorf("invalid target_dir: %w", err)
		}
		addon.Source.TargetDir = cleanDir
	}
	if len(addon.Targets) == 0 {
		return "", errors.New("at least one target path is required")
	}
	for i, t := range addon.Targets {
		cleanT, err := fsutil.CleanRel(t)
		if err != nil {
			return "", fmt.Errorf("invalid target %q: %w", t, err)
		}
		addon.Targets[i] = cleanT
	}

	destPath, err := fsutil.SafeJoin(catalogDir, fmt.Sprintf("addon.%s.json", addon.ID))
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(addon); err != nil {
		return "", err
	}

	if _, _, err := fsutil.WriteFileAtomicIfChanged(destPath, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	return destPath, nil
}

// RemoveUserAddon deletes <catalogDir>/addon.<id>.json and any associated backups.
func RemoveUserAddon(catalogDir, id string) (bool, error) {
	cleanID := strings.ToLower(strings.TrimSpace(id))
	if !IsValidID(cleanID) {
		return false, fmt.Errorf("invalid addon id %q", id)
	}
	target, err := fsutil.SafeJoin(catalogDir, fmt.Sprintf("addon.%s.json", cleanID))
	if err != nil {
		return false, err
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	fsutil.RemoveBackups(target)
	return true, nil
}
