package fsutil

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ExtractZipOptions configures jailed ZIP extraction inside an os.Root directory.
type ExtractZipOptions struct {
	StripPrefix         string
	AllowBundleSymlinks bool
	ModeForEntry        func(cleanRel string, zfMode os.FileMode) os.FileMode
	MaxFileBytes        int64
	MaxTotalBytes       int64
	MaxEntries          int
}

// ExtractZip opens zipPath and extracts matching entries inside destRoot using os.OpenRoot,
// returning the sorted list of extracted relative slash-separated paths.
func ExtractZip(zipPath, destRoot string, opts ExtractZipOptions) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	return ExtractZipFiles(zr.File, destRoot, opts)
}

// ExtractZipFiles extracts matching entries from files inside destRoot using os.OpenRoot,
// enforcing entry-count, per-file, and total-archive byte bounds alongside path traversal checks.
func ExtractZipFiles(files []*zip.File, destRoot string, opts ExtractZipOptions) ([]string, error) {
	maxEntries := opts.MaxEntries
	if maxEntries <= 0 {
		maxEntries = MaxZipEntries
	}
	if len(files) > maxEntries {
		return nil, fmt.Errorf("archive contains %d entries, exceeding maximum (%d)", len(files), maxEntries)
	}
	maxFileBytes := opts.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = MaxExtractFileBytes
	}
	maxTotalBytes := opts.MaxTotalBytes
	if maxTotalBytes <= 0 {
		maxTotalBytes = MaxExtractTotalBytes
	}

	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(destRoot)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	var (
		extracted   []string
		symlinks    []*zip.File
		symlinkRels []string
		totalBytes  int64
	)

	for _, zf := range files {
		slash := strings.ReplaceAll(zf.Name, `\`, `/`)
		if opts.StripPrefix != "" {
			after, ok := strings.CutPrefix(slash, opts.StripPrefix)
			if !ok {
				continue
			}
			slash = after
		}
		if slash == "" || strings.HasSuffix(slash, "/") || zf.FileInfo().IsDir() {
			continue
		}
		cleanRel, relErr := CleanRel(slash)
		if relErr != nil {
			return nil, fmt.Errorf("unsafe archive path %q: %w", zf.Name, relErr)
		}
		if zf.Mode()&os.ModeSymlink != 0 {
			if !opts.AllowBundleSymlinks {
				return nil, fmt.Errorf("refusing symlink entry %q in archive", zf.Name)
			}
			symlinks = append(symlinks, zf)
			symlinkRels = append(symlinkRels, cleanRel)
			continue
		}

		perm := os.FileMode(0o644)
		if opts.ModeForEntry != nil {
			perm = opts.ModeForEntry(cleanRel, zf.Mode())
		}
		remainingArchive := max(maxTotalBytes-totalBytes, 0)
		entryLimit := min(maxFileBytes, remainingArchive)

		written, writeErr := writeZipEntryToRoot(root, zf, cleanRel, perm, entryLimit)
		if writeErr != nil {
			return nil, writeErr
		}
		totalBytes += written
		extracted = append(extracted, cleanRel)
	}

	if len(symlinks) > 0 {
		if err := installValidatedBundleSymlinks(root, destRoot, symlinks, symlinkRels); err != nil {
			return nil, err
		}
		extracted = append(extracted, symlinkRels...)
	}

	if err := root.Close(); err != nil {
		return nil, err
	}
	slices.Sort(extracted)
	return extracted, nil
}

func writeZipEntryToRoot(
	root *os.Root,
	zf *zip.File,
	cleanRel string,
	perm os.FileMode,
	maxBytes int64,
) (int64, error) {
	osRel := filepath.FromSlash(cleanRel)
	parent := filepath.Dir(osRel)
	if parent != "." && parent != "" {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return 0, err
		}
	}
	rc, err := zf.Open()
	if err != nil {
		return 0, err
	}
	defer rc.Close()

	f, err := root.OpenFile(osRel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return 0, err
	}
	limited := &io.LimitedReader{R: rc, N: maxBytes + 1}
	written, copyErr := io.Copy(f, limited)
	closeErr := f.Close()
	if copyErr != nil {
		_ = root.Remove(osRel)
		return 0, copyErr
	}
	if written > maxBytes {
		_ = root.Remove(osRel)
		return 0, fmt.Errorf("archive entry %q exceeds size limit (%d bytes)", zf.Name, maxBytes)
	}
	return written, closeErr
}

func installValidatedBundleSymlinks(
	root *os.Root,
	destRoot string,
	symlinks []*zip.File,
	symlinkRels []string,
) error {
	realRoot, err := filepath.EvalSymlinks(destRoot)
	if err != nil {
		return err
	}
	for i, zf := range symlinks {
		if err := writeSafeSymlinkToRoot(root, realRoot, destRoot, zf, symlinkRels[i]); err != nil {
			return err
		}
	}
	for i, cleanRel := range symlinkRels {
		if _, statErr := root.Stat(filepath.FromSlash(cleanRel)); statErr != nil {
			return fmt.Errorf("bundle symlink %q escapes or is broken: %w", symlinks[i].Name, statErr)
		}
	}
	return nil
}

func writeSafeSymlinkToRoot(
	root *os.Root,
	realRoot, bundleRoot string,
	zf *zip.File,
	cleanRel string,
) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	linkBytes, err := io.ReadAll(io.LimitReader(rc, 4096))
	if err != nil {
		return err
	}
	target := strings.TrimSpace(string(linkBytes))
	slashTarget := strings.ReplaceAll(target, `\`, `/`)
	if target == "" || strings.ContainsRune(target, 0) || filepath.IsAbs(target) ||
		strings.HasPrefix(slashTarget, "/") || (len(slashTarget) >= 2 && slashTarget[1] == ':') {
		return fmt.Errorf("unsafe symlink target %q in %q", target, zf.Name)
	}

	osRel := filepath.FromSlash(cleanRel)
	parent := filepath.Dir(osRel)
	if parent != "." && parent != "" {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}

	realLinkDir, err := filepath.EvalSymlinks(filepath.Join(bundleRoot, parent))
	if err != nil {
		return err
	}
	relDir, err := filepath.Rel(realRoot, realLinkDir)
	if err != nil || relDir == ".." || strings.HasPrefix(filepath.Clean(relDir), "..") {
		return fmt.Errorf("symlink parent %q escapes bundle", zf.Name)
	}

	resolvedTarget := filepath.Clean(filepath.Join(realLinkDir, filepath.FromSlash(slashTarget)))
	relTarget, err := filepath.Rel(realRoot, resolvedTarget)
	if err != nil || relTarget == ".." || strings.HasPrefix(filepath.Clean(relTarget), "..") {
		return fmt.Errorf("symlink %q target %q escapes bundle", zf.Name, target)
	}

	_ = root.Remove(osRel)
	return root.Symlink(filepath.FromSlash(slashTarget), osRel)
}
