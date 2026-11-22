package engine

import (
	"archive/zip"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/lock14/openrct2-cli/internal/fsutil"
)

var rePlistBundleExec = regexp.MustCompile(
	`(?s)<key>\s*CFBundleExecutable\s*</key>\s*<string>\s*([^<\r\n]+?)\s*</string>`,
)

// IsBinaryReady reports whether path exists on disk as a non-empty regular file
// (following symlinks when present).
func IsBinaryReady(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir() && fi.Size() > 0
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
		var rollbackErr error
		if hadExisting {
			rollbackErr = os.Rename(backupDir, destDir)
		}
		return errors.Join(fmt.Errorf("installing bundle %s: %w", destDir, err), rollbackErr)
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

func bundleEntryMode(cleanRel string, zfMode os.FileMode) os.FileMode {
	lower := strings.ToLower(cleanRel)
	if zfMode&0o111 != 0 ||
		strings.HasPrefix(cleanRel, "Contents/MacOS/") ||
		strings.HasSuffix(lower, ".dylib") ||
		strings.HasSuffix(lower, ".exe") ||
		strings.HasSuffix(lower, ".dll") {
		return 0o755
	}
	return 0o644
}

func extractZipTreeInRoot(
	files []*zip.File,
	stripPrefix, destRoot string,
	allowBundleSymlinks bool,
) error {
	_, err := fsutil.ExtractZipFiles(files, destRoot, fsutil.ExtractZipOptions{
		StripPrefix:         stripPrefix,
		AllowBundleSymlinks: allowBundleSymlinks,
		ModeForEntry:        bundleEntryMode,
	})
	return err
}

func (ins *Installer) installLinuxAppImage(tmpPayload, payloadPath, linkDest string) error {
	if strings.TrimSpace(payloadPath) == "" || filepath.Clean(payloadPath) == filepath.Clean(linkDest) {
		if _, err := fsutil.CopyFileAtomic(tmpPayload, linkDest, 0o755); err != nil {
			return err
		}
		return os.Chmod(linkDest, 0o755)
	}
	if _, err := fsutil.CopyFileAtomic(tmpPayload, payloadPath, 0o755); err != nil {
		return err
	}
	if err := os.Chmod(payloadPath, 0o755); err != nil {
		return err
	}
	if err := fsutil.WriteSymlinkAtomic(linkDest, payloadPath); err != nil {
		// Fallback to POSIX wrapper if filesystem forbids symlinks.
		return writePosixWrapper(linkDest, payloadPath)
	}
	return nil
}

func findMacAppBundleInZip(files []*zip.File) (string, string, error) {
	for _, zf := range files {
		slash := strings.ReplaceAll(zf.Name, `\`, `/`)
		if strings.Contains(slash, "..") {
			continue
		}
		parts := strings.Split(slash, "/")
		for i, part := range parts {
			if strings.HasSuffix(strings.ToLower(part), ".app") && part != ".app" {
				return strings.Join(parts[:i+1], "/") + "/", part, nil
			}
		}
	}
	return "", "", errors.New("no .app bundle found in macOS archive")
}

func (ins *Installer) installMacAppZip(zipPath, wrapperDest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	appPrefix, appBundleName, err := findMacAppBundleInZip(zr.File)
	if err != nil {
		return err
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
	if !IsBinaryReady(filepath.Join(stagedDir, "openrct2.exe")) {
		return errors.New("openrct2.exe missing from Windows portable archive")
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
