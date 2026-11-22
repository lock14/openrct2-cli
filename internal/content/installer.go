// Package content downloads, verifies, extracts, and removes OpenRCT2 asset packs
// and JavaScript plugins declared in the catalog.
package content

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

// AddonReceipt records the installed version and relative target files for a single add-on.
type AddonReceipt struct {
	ID          string   `json:"id"`
	Version     string   `json:"version,omitempty"`
	SHA256      string   `json:"sha256,omitempty"`
	Files       []string `json:"files"`
	InstalledAt string   `json:"installed_at"`
}

// LoadAddonReceipts reads the add-on receipt map from path if present.
func LoadAddonReceipts(path string) (map[string]AddonReceipt, error) {
	out := make(map[string]AddonReceipt)
	ok, err := fsutil.ReadJSONFile(path, &out)
	if err != nil {
		return nil, err
	}
	if !ok || out == nil {
		return make(map[string]AddonReceipt), nil
	}
	return out, nil
}

// SaveAddonReceipts writes the add-on receipt map atomically to path.
func SaveAddonReceipts(path string, receipts map[string]AddonReceipt) error {
	_, _, err := fsutil.WriteJSONAtomicIfChanged(path, receipts, 0o644)
	return err
}

// IsInstalled reports whether all declared Targets of addon exist under openrct2Dir.
func IsInstalled(openrct2Dir string, addon catalog.Addon) bool {
	if strings.TrimSpace(openrct2Dir) == "" || len(addon.Targets) == 0 {
		return false
	}
	for _, rel := range addon.Targets {
		full, err := fsutil.SafeJoin(openrct2Dir, rel)
		if err != nil {
			return false
		}
		fi, statErr := os.Stat(full)
		if statErr != nil || fi.IsDir() || fi.Size() == 0 {
			return false
		}
	}
	return true
}

// Installer manages downloading, verifying, extracting, and removing OpenRCT2 add-ons.
type Installer struct {
	Paths  *config.Paths
	Client *http.Client
	Out    io.Writer
	Force  bool
}

// NewInstaller creates a new content Installer for paths.
func NewInstaller(paths *config.Paths, force bool, out io.Writer) *Installer {
	if out == nil {
		out = io.Discard
	}
	return &Installer{
		Paths:  paths,
		Client: &http.Client{Timeout: 5 * time.Minute},
		Out:    out,
		Force:  force,
	}
}

// InstallAll installs all add-ons in addons sequentially, collecting any errors.
func (ins *Installer) InstallAll(ctx context.Context, addons []catalog.Addon) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var errs []error
	for _, ad := range addons {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		if err := ins.InstallAddon(ctx, ad); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", ad.ID, err))
		}
	}
	return errors.Join(errs...)
}

// InstallAddon downloads, SHA-256-verifies, and deploys a single catalog.Addon.
func (ins *Installer) InstallAddon(ctx context.Context, addon catalog.Addon) error {
	if ins.Paths == nil {
		return errors.New("nil configuration paths")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	if !ins.Force && IsInstalled(ins.Paths.OpenRCT2Dir, addon) {
		fmt.Fprintf(ins.Out, "✓ %s (%s) already installed\n", addon.EffectiveDisplayName(), addon.ID)
		return nil
	}

	receipts, err := LoadAddonReceipts(ins.Paths.AddonReceiptsFile)
	if err != nil {
		return fmt.Errorf("loading addon receipts %s: %w", ins.Paths.AddonReceiptsFile, err)
	}

	fmt.Fprintf(ins.Out, "Installing %s (%s)...\n", addon.EffectiveDisplayName(), addon.Version)

	tmpDir, err := os.MkdirTemp("", "rct-addon-dl-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tmpPayload := filepath.Join(tmpDir, "addon.bin")
	if _, err := fsutil.DownloadVerifiedContext(
		ctx,
		ins.Client,
		addon.Source.URL,
		tmpPayload,
		addon.Source.SHA256,
		0o644,
	); err != nil {
		return fmt.Errorf("downloading %s: %w", addon.ID, err)
	}

	installedFiles, err := ins.deployPayload(tmpPayload, addon)
	if err != nil {
		return err
	}
	if !IsInstalled(ins.Paths.OpenRCT2Dir, addon) {
		return fmt.Errorf("addon %s extracted successfully, but expected target files were not found", addon.ID)
	}

	if prev, hadPrev := receipts[addon.ID]; hadPrev {
		pruneSupersededAddonFiles(ins.Paths.OpenRCT2Dir, prev.Files, installedFiles)
	}

	slices.Sort(installedFiles)
	receipts[addon.ID] = AddonReceipt{
		ID:          addon.ID,
		Version:     addon.Version,
		SHA256:      addon.Source.SHA256,
		Files:       installedFiles,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := SaveAddonReceipts(ins.Paths.AddonReceiptsFile, receipts); err != nil {
		return fmt.Errorf("saving addon receipt: %w", err)
	}

	fmt.Fprintf(ins.Out, "✓ Installed %s (%d file(s))\n", addon.EffectiveDisplayName(), len(installedFiles))
	return nil
}

func (ins *Installer) deployPayload(tmpPayload string, addon catalog.Addon) ([]string, error) {
	switch addon.Source.Mode {
	case catalog.InstallModeFile:
		cleanRel, relErr := fsutil.CleanRel(addon.Source.TargetPath)
		if relErr != nil {
			return nil, fmt.Errorf("invalid target_path for %s: %w", addon.ID, relErr)
		}
		dest, joinErr := fsutil.SafeJoin(ins.Paths.OpenRCT2Dir, cleanRel)
		if joinErr != nil {
			return nil, joinErr
		}
		payloadBytes, readErr := os.ReadFile(tmpPayload)
		if readErr != nil {
			return nil, readErr
		}
		if _, _, wErr := fsutil.WriteFileAtomicIfChanged(dest, payloadBytes, 0o644); wErr != nil {
			return nil, wErr
		}
		return []string{cleanRel}, nil

	case catalog.InstallModeZip:
		extracted, exErr := extractAddonZip(tmpPayload, ins.Paths.OpenRCT2Dir, addon.Source.TargetDir)
		if exErr != nil {
			return nil, fmt.Errorf("extracting %s: %w", addon.ID, exErr)
		}
		return extracted, nil

	default:
		return nil, fmt.Errorf("unsupported install mode %q for %s", addon.Source.Mode, addon.ID)
	}
}

func extractAddonZip(zipPath, openrct2Dir, targetSubdir string) ([]string, error) {
	extractRoot := openrct2Dir
	relPrefix := ""
	trimmedSub := strings.TrimSpace(targetSubdir)
	if trimmedSub != "" && trimmedSub != "." {
		cleanSub, subErr := fsutil.CleanRel(trimmedSub)
		if subErr != nil {
			return nil, subErr
		}
		joined, joinErr := fsutil.SafeJoin(openrct2Dir, cleanSub)
		if joinErr != nil {
			return nil, joinErr
		}
		extractRoot = joined
		relPrefix = cleanSub + "/"
	}

	extracted, err := fsutil.ExtractZip(zipPath, extractRoot, fsutil.ExtractZipOptions{
		AllowBundleSymlinks: false,
	})
	if err != nil {
		return nil, err
	}
	if relPrefix == "" {
		return extracted, nil
	}
	for i, rel := range extracted {
		extracted[i] = relPrefix + rel
	}
	return extracted, nil
}

func pruneSupersededAddonFiles(openrct2Dir string, previousFiles, currentFiles []string) {
	keep := make(map[string]bool, len(currentFiles))
	for _, f := range currentFiles {
		if clean, err := fsutil.CleanRel(f); err == nil {
			keep[clean] = true
		}
	}
	for _, oldRel := range previousFiles {
		clean, err := fsutil.CleanRel(oldRel)
		if err != nil || keep[clean] {
			continue
		}
		if dest, joinErr := fsutil.SafeJoin(openrct2Dir, clean); joinErr == nil {
			_ = os.Remove(dest)
			fsutil.RemoveBackups(dest)
		}
	}
}

// UninstallAddon removes all installed files tracked for addon under paths.OpenRCT2Dir.
func UninstallAddon(paths *config.Paths, addon catalog.Addon, out io.Writer) (int, error) {
	if paths == nil {
		return 0, errors.New("nil configuration paths")
	}
	if out == nil {
		out = io.Discard
	}

	receipts, err := LoadAddonReceipts(paths.AddonReceiptsFile)
	if err != nil {
		return 0, fmt.Errorf("loading addon receipts %s: %w", paths.AddonReceiptsFile, err)
	}

	seen := make(map[string]bool)
	var relList []string
	addRel := func(r string) {
		clean, relErr := fsutil.CleanRel(r)
		if relErr == nil && !seen[clean] {
			seen[clean] = true
			relList = append(relList, clean)
		}
	}
	for _, t := range addon.Targets {
		addRel(t)
	}
	if rec, ok := receipts[addon.ID]; ok {
		for _, f := range rec.Files {
			addRel(f)
		}
	}

	removed := 0
	for _, rel := range relList {
		dest, joinErr := fsutil.SafeJoin(paths.OpenRCT2Dir, rel)
		if joinErr != nil {
			continue
		}
		if rmErr := os.Remove(dest); rmErr == nil {
			removed++
		} else if !errors.Is(rmErr, fs.ErrNotExist) {
			return removed, fmt.Errorf("removing %s: %w", dest, rmErr)
		}
		fsutil.RemoveBackups(dest)
	}

	if _, ok := receipts[addon.ID]; ok {
		delete(receipts, addon.ID)
		if err := SaveAddonReceipts(paths.AddonReceiptsFile, receipts); err != nil {
			return removed, fmt.Errorf("saving addon receipts: %w", err)
		}
	}

	if removed > 0 {
		fmt.Fprintf(out, "✓ Removed %s (%d file(s))\n", addon.EffectiveDisplayName(), removed)
	}
	return removed, nil
}
