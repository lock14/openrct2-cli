// Package fsutil provides hardened filesystem helpers for atomic writes,
// idempotent non-destructive timestamped backups, JSON persistence, verified
// HTTP downloads, jailed ZIP extraction, and traversal-safe path joins.
package fsutil

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// MaxBackupsPerFile is the maximum number of timestamped backup files retained per target file.
const MaxBackupsPerFile = 3

// MaxDownloadBytes is the maximum payload size (2 GiB) accepted by DownloadVerifiedContext.
const MaxDownloadBytes int64 = 2 << 30

// MaxExtractFileBytes is the maximum decompressed size (2 GiB) accepted per archive entry.
const MaxExtractFileBytes int64 = 2 << 30

// MaxExtractTotalBytes is the maximum total decompressed bytes (8 GiB) accepted per archive.
const MaxExtractTotalBytes int64 = 8 << 30

// MaxZipEntries is the maximum number of entries accepted when extracting a ZIP archive.
const MaxZipEntries = 16384

// UserAgent is the HTTP User-Agent header sent on all outbound requests.
const UserAgent = "openrct2-cli/1.0"

func createBackupUnpruned(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil
	}
	if info.IsDir() {
		return "", fmt.Errorf("cannot back up directory %q", path)
	}

	in, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("reading %s for backup: %w", path, err)
	}
	defer in.Close()

	ts := time.Now().Format("20060102150405")
	backupPath := nextBackupPath(path, ts)

	if _, _, err := writeReaderTempAndRename(backupPath, in, info.Mode().Perm(), false, nil); err != nil {
		return "", fmt.Errorf("writing backup %s: %w", backupPath, err)
	}
	return backupPath, nil
}

func nextBackupPath(path, ts string) string {
	dir := filepath.Dir(path)
	baseName := filepath.Base(path) + ".bak." + ts
	entries, err := os.ReadDir(dir)
	if err != nil {
		return filepath.Join(dir, baseName)
	}
	maxSeq := -1
	for _, entry := range entries {
		name := entry.Name()
		if name == baseName {
			if maxSeq < 0 {
				maxSeq = 0
			}
			continue
		}
		if suffix, ok := strings.CutPrefix(name, baseName+"-"); ok {
			if seq, convErr := strconv.Atoi(suffix); convErr == nil && seq > maxSeq {
				maxSeq = seq
			}
		}
	}
	if maxSeq < 0 {
		return filepath.Join(dir, baseName)
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d", baseName, maxSeq+1))
}

type backupEntry struct {
	name string
	ts   string
	seq  int
}

func pruneBackups(path string, keep int) {
	if keep <= 0 || strings.TrimSpace(path) == "" {
		return
	}
	dir := filepath.Dir(path)
	prefix := filepath.Base(path) + ".bak."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var backups []backupEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		suffix, ok := strings.CutPrefix(entry.Name(), prefix)
		if !ok {
			continue
		}
		tsPart := suffix
		seq := 0
		if before, after, hasDash := strings.Cut(suffix, "-"); hasDash {
			if parsed, convErr := strconv.Atoi(after); convErr == nil {
				tsPart = before
				seq = parsed
			}
		}
		backups = append(backups, backupEntry{
			name: entry.Name(),
			ts:   tsPart,
			seq:  seq,
		})
	}
	if len(backups) <= keep {
		return
	}
	slices.SortFunc(backups, func(a, b backupEntry) int {
		return cmp.Or(
			cmp.Compare(a.ts, b.ts),
			cmp.Compare(a.seq, b.seq),
			cmp.Compare(a.name, b.name),
		)
	})
	for _, old := range backups[:len(backups)-keep] {
		_ = os.Remove(filepath.Join(dir, old.name))
	}
}

// RemoveBackups deletes all timestamped backup files (<path>.bak.*) for path.
func RemoveBackups(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	dir := filepath.Dir(path)
	prefix := filepath.Base(path) + ".bak."
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// WriteFileAtomic writes data to path atomically via a temporary file in the
// same directory and os.Rename. If path already exists, a timestamped backup is
// created first via BackupFile and its path is returned.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (string, error) {
	_, backupPath, err := WriteReaderAtomic(path, bytes.NewReader(data), perm)
	return backupPath, err
}

// WriteFileAtomicIfChanged writes data to path atomically only when path does not
// exist as a regular file or its exact bytes differ from data. When unchanged,
// it returns (false, "", nil) without creating a backup.
func WriteFileAtomicIfChanged(path string, data []byte, perm os.FileMode) (bool, string, error) {
	if path == "" {
		return false, "", errors.New("empty file path")
	}
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		if existing, readErr := os.ReadFile(path); readErr == nil && bytes.Equal(existing, data) {
			ensureExecutableBits(path, info.Mode().Perm(), perm)
			return false, "", nil
		}
	}
	bkp, err := WriteFileAtomic(path, data, perm)
	if err != nil {
		return false, "", err
	}
	return true, bkp, nil
}

// NormalizeLF converts CRLF line endings to LF.
func NormalizeLF(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// WriteTextAtomicIfChanged writes UTF-8 text to path atomically only when path
// does not exist as a regular file or its line-ending-normalized text differs.
func WriteTextAtomicIfChanged(path, content string, perm os.FileMode) (bool, string, error) {
	if path == "" {
		return false, "", errors.New("empty file path")
	}
	normalizedNew := strings.TrimRight(NormalizeLF(content), "\n") + "\n"
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		if existing, readErr := os.ReadFile(path); readErr == nil {
			normalizedExisting := strings.TrimRight(NormalizeLF(string(existing)), "\n") + "\n"
			if normalizedExisting == normalizedNew {
				ensureExecutableBits(path, info.Mode().Perm(), perm)
				return false, "", nil
			}
		}
	}
	bkp, err := WriteFileAtomic(path, []byte(normalizedNew), perm)
	if err != nil {
		return false, "", err
	}
	return true, bkp, nil
}

func ensureExecutableBits(path string, currentPerm, desiredPerm os.FileMode) {
	if desiredPerm&0o111 != 0 && currentPerm&0o111 == 0 {
		_ = os.Chmod(path, currentPerm|(desiredPerm&0o111))
	}
}

// WriteSymlinkAtomic creates or replaces linkDest atomically as a symbolic link
// pointing to targetPath using a collision-free temporary sibling symlink.
func WriteSymlinkAtomic(linkDest, targetPath string) error {
	if strings.TrimSpace(linkDest) == "" {
		return errors.New("empty symlink destination")
	}
	parent := filepath.Dir(linkDest)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", parent, err)
	}
	var rnd [8]byte
	_, _ = rand.Read(rnd[:])
	tmpLink := filepath.Join(parent, ".rct-link-"+hex.EncodeToString(rnd[:]))
	_ = os.Remove(tmpLink)
	if err := os.Symlink(targetPath, tmpLink); err != nil {
		return err
	}
	if err := os.Rename(tmpLink, linkDest); err != nil {
		_ = os.Remove(tmpLink)
		return err
	}
	return nil
}

// WriteReaderAtomic streams r to path atomically via a temporary file in the
// same directory and os.Rename. If path already exists, a timestamped backup is
// created first via BackupFile and its path is returned alongside the byte count.
func WriteReaderAtomic(path string, r io.Reader, perm os.FileMode) (int64, string, error) {
	if path == "" {
		return 0, "", errors.New("empty file path")
	}
	return writeReaderTempAndRename(path, r, perm, true, nil)
}

// WriteLimitedReaderAtomic streams r to path atomically while rejecting payloads
// exceeding maxBytes before any existing target file is backed up or overwritten.
func WriteLimitedReaderAtomic(
	path string,
	r io.Reader,
	maxBytes int64,
	perm os.FileMode,
) (int64, string, error) {
	if path == "" {
		return 0, "", errors.New("empty file path")
	}
	if maxBytes <= 0 {
		maxBytes = MaxExtractFileBytes
	}
	lr := &io.LimitedReader{R: r, N: maxBytes + 1}
	return writeReaderTempAndRename(path, lr, perm, true, func() error {
		if lr.N <= 0 {
			return fmt.Errorf("payload for %s exceeds maximum size (%d bytes)", path, maxBytes)
		}
		return nil
	})
}

// CopyFileAtomic copies src to dst atomically via WriteReaderAtomic, creating
// a non-clobbering timestamped backup if dst already exists.
func CopyFileAtomic(src, dst string, perm os.FileMode) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()

	_, backupPath, err := WriteReaderAtomic(dst, in, perm)
	return backupPath, err
}

func writeReaderTempAndRename(
	path string,
	r io.Reader,
	perm os.FileMode,
	createBackup bool,
	validate func() error,
) (int64, string, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, "", fmt.Errorf("creating directory %s: %w", dir, err)
	}

	effectivePerm := perm
	if info, statErr := os.Lstat(path); statErr == nil && info.Mode().IsRegular() {
		effectivePerm = info.Mode().Perm() | (perm & 0o111)
	}

	tmp, err := os.CreateTemp(dir, ".rct-tmp-*")
	if err != nil {
		return 0, "", err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := io.Copy(tmp, r)
	if err != nil {
		_ = tmp.Close()
		return written, "", err
	}
	if err := tmp.Chmod(effectivePerm); err != nil {
		_ = tmp.Close()
		return written, "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return written, "", err
	}
	if err := tmp.Close(); err != nil {
		return written, "", err
	}

	if validate != nil {
		if err := validate(); err != nil {
			return written, "", err
		}
	}

	var backupPath string
	if createBackup {
		backupPath, err = createBackupUnpruned(path)
		if err != nil {
			return written, "", err
		}
	}
	if err := os.Rename(tmpPath, path); err != nil {
		if backupPath != "" {
			_ = os.Remove(backupPath)
		}
		return written, "", err
	}
	cleanup = false
	if backupPath != "" {
		pruneBackups(path, MaxBackupsPerFile)
	}
	return written, backupPath, nil
}
