package engine

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lock14/openrct2-cli/internal/fsutil"
)

var rePlistBundleExec = regexp.MustCompile(
	`(?s)<key>\s*CFBundleExecutable\s*</key>\s*<string>\s*([^<\r\n]+?)\s*</string>`,
)

func zipSlashName(zf *zip.File) string {
	return strings.ReplaceAll(zf.Name, `\`, `/`)
}

func isZipSymlink(zf *zip.File) bool {
	return zf.Mode()&os.ModeSymlink != 0
}

func swapDirectoryAtomic(stagedDir, destDir string) error {
	backupDir := destDir + ".rct-bak"
	_ = os.RemoveAll(backupDir)

	hadExisting := false
	if _, err := os.Lstat(destDir); err == nil {
		if err := os.Rename(destDir, backupDir); err != nil {
			return fmt.Errorf("backing up existing bundle %s: %w", destDir, err)
		}
		hadExisting = true
	}

	if err := os.Rename(stagedDir, destDir); err != nil {
		if hadExisting {
			_ = os.Rename(backupDir, destDir)
		}
		return fmt.Errorf("installing bundle %s: %w", destDir, err)
	}

	if hadExisting {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func writePosixWrapper(dest, appBin string) error {
	wrapper := fmt.Sprintf("#!/bin/sh\nexec %s \"$@\"\n", shellSingleQuote(appBin))
	_, _, err := fsutil.WriteFileAtomicIfChanged(dest, []byte(wrapper), 0o755)
	return err
}

func writeWindowsCmdShim(shimPath, exePath string) error {
	shim := fmt.Sprintf("@echo off\r\n\"%s\" %%*\r\n", exePath)
	_, _, err := fsutil.WriteFileAtomicIfChanged(shimPath, []byte(shim), 0o644)
	return err
}

func (ins *Installer) installMacAppZip(zipPath, wrapperDest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	appPrefix := ""
	appBundleName := ""
	for _, zf := range zr.File {
		slash := zipSlashName(zf)
		if strings.Contains(slash, "..") {
			continue
		}
		parts := strings.Split(slash, "/")
		for i, part := range parts {
			if strings.HasSuffix(strings.ToLower(part), ".app") && part != ".app" {
				appPrefix = strings.Join(parts[:i+1], "/") + "/"
				appBundleName = part
				break
			}
		}
		if appPrefix != "" {
			break
		}
	}
	if appPrefix == "" {
		return fmt.Errorf("no .app bundle found in macOS archive")
	}

	appsDir := ins.Paths.AppsDir
	if strings.TrimSpace(appsDir) == "" {
		home, _ := os.UserHomeDir()
		appsDir = filepath.Join(home, "Applications")
	}
	if err := os.MkdirAll(appsDir, 0o755); err != nil {
		return err
	}
	appDest, err := fsutil.SafeJoin(appsDir, appBundleName)
	if err != nil {
		return err
	}

	stageParent, err := os.MkdirTemp(appsDir, ".rct-app-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageParent)

	stagedApp, err := fsutil.SafeJoin(stageParent, appBundleName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stagedApp, 0o755); err != nil {
		return err
	}

	if err := extractZipTreeInRoot(zr.File, appPrefix, stagedApp, true); err != nil {
		return err
	}
	if _, err := resolveMacExecutable(stagedApp); err != nil {
		return err
	}
	if err := swapDirectoryAtomic(stagedApp, appDest); err != nil {
		return err
	}
	appBin, err := resolveMacExecutable(appDest)
	if err != nil {
		return err
	}
	return writePosixWrapper(wrapperDest, appBin)
}

func resolveMacExecutable(appDir string) (string, error) {
	macOSDir := filepath.Join(appDir, "Contents", "MacOS")
	entries, err := os.ReadDir(macOSDir)
	if err != nil || len(entries) == 0 {
		return "", fmt.Errorf("executable not found in %s", macOSDir)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), "OpenRCT2") {
			return filepath.Join(macOSDir, e.Name()), nil
		}
	}
	plistPath := filepath.Join(appDir, "Contents", "Info.plist")
	if plistData, readErr := os.ReadFile(plistPath); readErr == nil {
		if m := rePlistBundleExec.FindSubmatch(plistData); len(m) >= 2 {
			execName := filepath.Base(strings.TrimSpace(string(m[1])))
			cand := filepath.Join(macOSDir, execName)
			if fi, statErr := os.Stat(cand); statErr == nil && !fi.IsDir() {
				return cand, nil
			}
		}
	}
	for _, e := range entries {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			return filepath.Join(macOSDir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no executable found in %s", macOSDir)
}

func (ins *Installer) installWindowsPortableZip(zipPath, destExe string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	engineDir := ins.Paths.EngineDir
	if strings.TrimSpace(engineDir) == "" {
		engineDir = filepath.Dir(destExe)
	}
	parentDir := filepath.Dir(engineDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return err
	}

	stageParent, err := os.MkdirTemp(parentDir, ".rct-win-stage-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stageParent)

	stagedDir := filepath.Join(stageParent, "engine")
	if err := os.MkdirAll(stagedDir, 0o755); err != nil {
		return err
	}

	if err := extractZipTreeInRoot(zr.File, "", stagedDir, false); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(stagedDir, "openrct2.exe")); err != nil {
		return fmt.Errorf("openrct2.exe missing from Windows portable archive: %w", err)
	}
	if err := swapDirectoryAtomic(stagedDir, engineDir); err != nil {
		return err
	}

	if ins.Paths.BinDir != "" {
		shimPath := filepath.Join(ins.Paths.BinDir, "openrct2.cmd")
		if err := writeWindowsCmdShim(shimPath, destExe); err != nil {
			return err
		}
	}
	return nil
}

func extractZipTreeInRoot(
	files []*zip.File,
	stripPrefix, destRoot string,
	allowBundleSymlinks bool,
) error {
	root, err := os.OpenRoot(destRoot)
	if err != nil {
		return err
	}
	defer root.Close()

	var symlinks []*zip.File
	var symlinkRels []string

	for _, zf := range files {
		slash := zipSlashName(zf)
		if stripPrefix != "" {
			if !strings.HasPrefix(slash, stripPrefix) {
				continue
			}
			slash = strings.TrimPrefix(slash, stripPrefix)
		}
		if slash == "" || strings.HasSuffix(slash, "/") || zf.FileInfo().IsDir() {
			continue
		}
		cleanRel, relErr := fsutil.CleanRel(slash)
		if relErr != nil {
			return fmt.Errorf("unsafe archive path %q: %w", zf.Name, relErr)
		}
		if isZipSymlink(zf) {
			if !allowBundleSymlinks {
				return fmt.Errorf("unexpected symlink %q in archive", zf.Name)
			}
			symlinks = append(symlinks, zf)
			symlinkRels = append(symlinkRels, cleanRel)
			continue
		}
		perm := os.FileMode(0o644)
		lower := strings.ToLower(cleanRel)
		if zf.Mode()&0o111 != 0 ||
			strings.HasPrefix(cleanRel, "Contents/MacOS/") ||
			strings.HasSuffix(lower, ".dylib") ||
			strings.HasSuffix(lower, ".exe") ||
			strings.HasSuffix(lower, ".dll") {
			perm = 0o755
		}
		if err := writeZipEntryToRoot(root, zf, cleanRel, perm); err != nil {
			return err
		}
	}

	if len(symlinks) > 0 {
		realRoot, evalErr := filepath.EvalSymlinks(destRoot)
		if evalErr != nil {
			return evalErr
		}
		for i, zf := range symlinks {
			if err := writeSafeSymlinkToRoot(root, realRoot, destRoot, zf, symlinkRels[i]); err != nil {
				return err
			}
		}
		for i, cleanRel := range symlinkRels {
			if _, statErr := root.Stat(cleanRel); statErr != nil {
				return fmt.Errorf("bundle symlink %q escapes or is broken: %w", symlinks[i].Name, statErr)
			}
		}
	}

	return root.Close()
}

func writeZipEntryToRoot(root *os.Root, zf *zip.File, cleanRel string, perm os.FileMode) error {
	parent := filepath.Dir(filepath.FromSlash(cleanRel))
	if parent != "." && parent != "" {
		if err := root.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	f, err := root.OpenFile(filepath.FromSlash(cleanRel), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	limited := &io.LimitedReader{R: rc, N: fsutil.MaxExtractFileBytes + 1}
	written, copyErr := io.Copy(f, limited)
	closeErr := f.Close()
	if copyErr != nil {
		_ = root.Remove(filepath.FromSlash(cleanRel))
		return copyErr
	}
	if written > fsutil.MaxExtractFileBytes {
		_ = root.Remove(filepath.FromSlash(cleanRel))
		return fmt.Errorf("archive entry %q exceeds maximum size (%d bytes)", zf.Name, fsutil.MaxExtractFileBytes)
	}
	return closeErr
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
