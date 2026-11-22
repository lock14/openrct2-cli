package engine

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lock14/openrct2-cli/internal/config"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

// Uninstall removes managed user-space OpenRCT2 engine binaries, wrappers/shims,
// extracted application bundles, and the installation receipt for targetOS.
func Uninstall(paths *config.Paths, targetOS string, out io.Writer) error {
	if paths == nil {
		return errors.New("nil configuration paths")
	}
	if out == nil {
		out = io.Discard
	}
	if targetOS == "" {
		targetOS = runtime.GOOS
	}

	binPath := paths.ManagedEngineBinaryForOS(targetOS)
	if binPath != "" {
		if err := os.Remove(binPath); err == nil {
			fmt.Fprintf(out, "✓ Removed engine binary %s\n", binPath)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing %s: %w", binPath, err)
		}
		fsutil.RemoveBackups(binPath)
	}

	switch {
	case targetOS == "darwin" && strings.HasSuffix(paths.EngineDir, "OpenRCT2.app"):
		if _, err := os.Lstat(paths.EngineDir); err == nil {
			if rmErr := os.RemoveAll(paths.EngineDir); rmErr != nil {
				return fmt.Errorf("removing app bundle %s: %w", paths.EngineDir, rmErr)
			}
			fmt.Fprintf(out, "✓ Removed app bundle %s\n", paths.EngineDir)
		}
	case targetOS == "windows" && paths.EngineDir != "":
		if _, err := os.Lstat(paths.EngineDir); err == nil {
			if rmErr := os.RemoveAll(paths.EngineDir); rmErr != nil {
				return fmt.Errorf("removing engine bundle %s: %w", paths.EngineDir, rmErr)
			}
			fmt.Fprintf(out, "✓ Removed engine bundle %s\n", paths.EngineDir)
		}
		if paths.BinDir != "" {
			shim := filepath.Join(paths.BinDir, "openrct2.cmd")
			_ = os.Remove(shim)
			fsutil.RemoveBackups(shim)
		}
	case paths.EngineDir != "" && filepath.Clean(paths.EngineDir) != filepath.Clean(paths.BinDir):
		if _, err := os.Lstat(paths.EngineDir); err == nil {
			if rmErr := os.RemoveAll(paths.EngineDir); rmErr != nil {
				return fmt.Errorf("removing engine directory %s: %w", paths.EngineDir, rmErr)
			}
			fmt.Fprintf(out, "✓ Removed engine directory %s\n", paths.EngineDir)
		}
	}

	if paths.EngineReceiptFile != "" {
		if err := os.Remove(paths.EngineReceiptFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("removing engine receipt %s: %w", paths.EngineReceiptFile, err)
		}
		fsutil.RemoveBackups(paths.EngineReceiptFile)
	}
	return nil
}
