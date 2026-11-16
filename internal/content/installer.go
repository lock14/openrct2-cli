// Package content downloads, verifies, extracts, and removes OpenRCT2 asset packs
// and JavaScript plugins declared in the catalog.
package content

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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
	if strings.TrimSpace(path) == "" {
		return out, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SaveAddonReceipts writes the add-on receipt map atomically to path.
func SaveAddonReceipts(path string, receipts map[string]AddonReceipt) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(receipts); err != nil {
		return err
	}
	_, _, err := fsutil.WriteFileAtomicIfChanged(path, buf.Bytes(), 0o644)
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

// InstallAll installs all add-ons in list (or all add-ons in cat), collecting any errors.
func (ins *Installer) InstallAll(ctx context.Context, addons []catalog.Addon) error {
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

	var installedFiles []string
	switch addon.Source.Mode {
	case catalog.InstallModeFile:
		cleanRel, relErr := fsutil.CleanRel(addon.Source.TargetPath)
		if relErr != nil {
			return fmt.Errorf("invalid target_path for %s: %w", addon.ID, relErr)
		}
		dest, joinErr := fsutil.SafeJoin(ins.Paths.OpenRCT2Dir, cleanRel)
		if joinErr != nil {
			return joinErr
		}
		payloadBytes, readErr := os.ReadFile(tmpPayload)
		if readErr != nil {
			return readErr
		}
		if _, _, wErr := fsutil.WriteFileAtomicIfChanged(dest, payloadBytes, 0o644); wErr != nil {
			return wErr
		}
		installedFiles = []string{cleanRel}

	case catalog.InstallModeZip:
		extracted, exErr := extractAddonZip(tmpPayload, ins.Paths.OpenRCT2Dir, addon.Source.TargetDir)
		if exErr != nil {
			return fmt.Errorf("extracting %s: %w", addon.ID, exErr)
		}
		installedFiles = extracted

	default:
		return fmt.Errorf("unsupported install mode %q for %s", addon.Source.Mode, addon.ID)
	}

	if !IsInstalled(ins.Paths.OpenRCT2Dir, addon) {
		return fmt.Errorf("addon %s extracted successfully, but expected target files were not found", addon.ID)
	}

	receipts, _ := LoadAddonReceipts(ins.Paths.AddonReceiptsFile)
	if receipts == nil {
		receipts = make(map[string]AddonReceipt)
	}
	sort.Strings(installedFiles)
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

func extractAddonZip(zipPath, openrct2Dir, targetSubdir string) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

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

	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(extractRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var extracted []string
	for _, zf := range zr.File {
		slash := strings.ReplaceAll(zf.Name, `\`, `/`)
		if slash == "" || strings.HasSuffix(slash, "/") || zf.FileInfo().IsDir() {
			continue
		}
		if zf.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("refusing symlink entry %q in addon archive", zf.Name)
		}
		cleanRel, relErr := fsutil.CleanRel(slash)
		if relErr != nil {
			return nil, fmt.Errorf("unsafe archive entry %q: %w", zf.Name, relErr)
		}
		osRel := filepath.FromSlash(cleanRel)
		parent := filepath.Dir(osRel)
		if parent != "." && parent != "" {
			if err := root.MkdirAll(parent, 0o755); err != nil {
				return nil, err
			}
		}
		rc, openErr := zf.Open()
		if openErr != nil {
			return nil, openErr
		}
		outFile, createErr := root.OpenFile(osRel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
		if createErr != nil {
			_ = rc.Close()
			return nil, createErr
		}
		limited := &io.LimitedReader{R: rc, N: fsutil.MaxExtractFileBytes + 1}
		written, copyErr := io.Copy(outFile, limited)
		closeErr := outFile.Close()
		_ = rc.Close()
		if copyErr != nil {
			_ = root.Remove(osRel)
			return nil, copyErr
		}
		if written > fsutil.MaxExtractFileBytes {
			_ = root.Remove(osRel)
			return nil, fmt.Errorf("archive entry %q exceeds size limit", zf.Name)
		}
		if closeErr != nil {
			return nil, closeErr
		}
		extracted = append(extracted, relPrefix+cleanRel)
	}

	if err := root.Close(); err != nil {
		return nil, err
	}
	return extracted, nil
}

// UninstallAddon removes all installed files tracked for addon under paths.OpenRCT2Dir.
func UninstallAddon(paths *config.Paths, addon catalog.Addon, out io.Writer) (int, error) {
	if paths == nil {
		return 0, errors.New("nil configuration paths")
	}
	if out == nil {
		out = io.Discard
	}

	receipts, _ := LoadAddonReceipts(paths.AddonReceiptsFile)
	seen := make(map[string]bool)
	var relList []string
	addRel := func(r string) {
		clean, err := fsutil.CleanRel(r)
		if err == nil && !seen[clean] {
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
		dest, err := fsutil.SafeJoin(paths.OpenRCT2Dir, rel)
		if err != nil {
			continue
		}
		if err := os.Remove(dest); err == nil {
			removed++
		}
		fsutil.RemoveBackups(dest)
	}

	if receipts != nil {
		if _, ok := receipts[addon.ID]; ok {
			delete(receipts, addon.ID)
			_ = SaveAddonReceipts(paths.AddonReceiptsFile, receipts)
		}
	}

	if removed > 0 {
		fmt.Fprintf(out, "✓ Removed %s (%d file(s))\n", addon.EffectiveDisplayName(), removed)
	}
	return removed, nil
}
