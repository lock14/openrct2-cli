package fsutil

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteFileAtomicAndPruneBackups(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "config.ini")

	bkp, err := WriteFileAtomic(target, []byte("v1\n"), 0o644)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	if bkp != "" {
		t.Fatalf("expected no backup on initial write, got %q", bkp)
	}

	for _, ver := range []string{"v2\n", "v3\n", "v4\n", "v5\n"} {
		if _, err := WriteFileAtomic(target, []byte(ver), 0o644); err != nil {
			t.Fatalf("write %s: %v", ver, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	backups := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.ini.bak.") {
			backups++
		}
	}
	if backups != MaxBackupsPerFile {
		t.Fatalf("backups = %d, want %d", backups, MaxBackupsPerFile)
	}

	RemoveBackups(target)
	entries, _ = os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "config.ini.bak.") {
			t.Fatalf("expected backups removed, found %s", e.Name())
		}
	}
}

func TestWriteFileAtomicIfChangedIdempotency(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "shortcuts.json")

	changed, bkp, err := WriteFileAtomicIfChanged(target, []byte(`{"a":1}`), 0o644)
	if err != nil || !changed || bkp != "" {
		t.Fatalf("initial write = (%v, %q, %v), want (true, \"\", nil)", changed, bkp, err)
	}

	changed, bkp, err = WriteFileAtomicIfChanged(target, []byte(`{"a":1}`), 0o644)
	if err != nil || changed || bkp != "" {
		t.Fatalf("identical write = (%v, %q, %v), want (false, \"\", nil)", changed, bkp, err)
	}

	changed, bkp, err = WriteFileAtomicIfChanged(target, []byte(`{"a":2}`), 0o644)
	if err != nil || !changed || bkp == "" {
		t.Fatalf("changed write = (%v, %q, %v), want (true, non-empty, nil)", changed, bkp, err)
	}
}

func TestJSONHelpersAndUTF8BOM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	var dst map[string]string
	ok, err := ReadJSONFile(path, &dst)
	if err != nil || ok {
		t.Fatalf("missing ReadJSONFile = (%v, %v), want (false, nil)", ok, err)
	}

	changed, _, err := WriteJSONAtomicIfChanged(path, map[string]string{"url": "https://a&b"}, 0o644)
	if err != nil || !changed {
		t.Fatalf("WriteJSONAtomicIfChanged = (%v, %v)", changed, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `\u0026`) {
		t.Fatalf("expected unescaped '&' in JSON, got %s", string(raw))
	}

	// Prepend UTF-8 BOM and verify ReadJSONFile strips it cleanly
	bomData := append([]byte("\xef\xbb\xbf"), raw...)
	if err := os.WriteFile(path, bomData, 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = ReadJSONFile(path, &dst)
	if err != nil || !ok || dst["url"] != "https://a&b" {
		t.Fatalf("BOM ReadJSONFile = (%v, %v, %v)", ok, err, dst)
	}
}

func TestWriteTextAtomicIfChangedCRLF(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "config.ini")

	if err := os.WriteFile(target, []byte("[general]\r\nuncap_fps = true\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	changed, bkp, err := WriteTextAtomicIfChanged(target, "[general]\nuncap_fps = true\n", 0o644)
	if err != nil || changed || bkp != "" {
		t.Fatalf("CRLF-equivalent write = (%v, %q, %v), want (false, \"\", nil)", changed, bkp, err)
	}
}

func TestSymlinkAtomicAndReplacementSkipsTargetBackup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test on unix")
	}
	t.Parallel()
	dir := t.TempDir()
	extTarget := filepath.Join(dir, "external.AppImage")
	if err := os.WriteFile(extTarget, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "openrct2")
	if err := WriteSymlinkAtomic(linkPath, extTarget); err != nil {
		t.Fatalf("WriteSymlinkAtomic initial: %v", err)
	}
	if err := WriteSymlinkAtomic(linkPath, extTarget); err != nil {
		t.Fatalf("WriteSymlinkAtomic replace: %v", err)
	}

	changed, bkp, err := WriteFileAtomicIfChanged(linkPath, []byte("new-binary"), 0o755)
	if err != nil {
		t.Fatalf("replace symlink: %v", err)
	}
	if !changed || bkp != "" {
		t.Fatalf("got changed=%v bkp=%q, want true and empty backup", changed, bkp)
	}
	info, err := os.Lstat(linkPath)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("expected regular file after symlink replacement, got %v %v", info, err)
	}
}

func TestCleanRelSafeJoinAndSHA256Validation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	valid := []struct {
		name string
		rel  string
		want string
	}{
		{name: "simple file", rel: "plugin/benchwarmer.js", want: "plugin/benchwarmer.js"},
		{name: "backslash normalized", rel: `assetpack\openrct2.sound.parkap`, want: "assetpack/openrct2.sound.parkap"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := CleanRel(tc.rel)
			if err != nil || got != tc.want {
				t.Fatalf("CleanRel(%q) = (%q, %v), want (%q, nil)", tc.rel, got, err, tc.want)
			}
			if _, err := SafeJoin(root, tc.rel); err != nil {
				t.Fatalf("SafeJoin(%q, %q) unexpected error: %v", root, tc.rel, err)
			}
		})
	}

	invalid := []string{
		"",
		"../escape.js",
		"plugin/../../escape.js",
		"/etc/passwd",
		`C:\Windows\System32`,
		"CON.js",
		"plugin/trailing.",
		"plugin/trailing ",
		"plugin/bad:colon.js",
		"plugin/\x00null.js",
	}
	for _, rel := range invalid {
		if _, err := SafeJoin(root, rel); err == nil {
			t.Errorf("SafeJoin(%q) expected error, got nil", rel)
		}
	}

	upper := strings.Repeat("AB", 32)
	norm, err := ValidateSHA256Hex(upper)
	if err != nil || norm != strings.ToLower(upper) {
		t.Fatalf("ValidateSHA256Hex(valid) = (%q, %v)", norm, err)
	}
	if _, err := ValidateSHA256Hex("not-hex"); err == nil {
		t.Fatal("expected ValidateSHA256Hex error on malformed hash")
	}
}

func TestExtractZipBoundsAndJailing(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte("0123456789"))
	}
	_ = zw.Close()

	zipPath := filepath.Join(t.TempDir(), "sample.zip")
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reject multi-entry archive exceeding MaxTotalBytes
	if _, err := ExtractZip(zipPath, t.TempDir(), ExtractZipOptions{MaxTotalBytes: 20}); err == nil {
		t.Fatal("expected total byte limit error across multiple archive entries")
	}
	// Reject archive exceeding MaxEntries
	if _, err := ExtractZip(zipPath, t.TempDir(), ExtractZipOptions{MaxEntries: 2}); err == nil {
		t.Fatal("expected max entries limit error")
	}
	// Succeed within bounds
	files, err := ExtractZip(zipPath, t.TempDir(), ExtractZipOptions{MaxTotalBytes: 100, MaxEntries: 10})
	if err != nil || len(files) != 3 {
		t.Fatalf("ExtractZip valid = (%v, %v)", files, err)
	}

	// Verify CopyFileAtomic creates target and backs up on update
	srcFile := filepath.Join(t.TempDir(), "src.bin")
	dstFile := filepath.Join(t.TempDir(), "dst.bin")
	_ = os.WriteFile(srcFile, []byte("payload-1"), 0o644)
	if bkp, err := CopyFileAtomic(srcFile, dstFile, 0o644); err != nil || bkp != "" {
		t.Fatalf("CopyFileAtomic initial = (%q, %v)", bkp, err)
	}
	_ = os.WriteFile(srcFile, []byte("payload-2"), 0o644)
	if bkp, err := CopyFileAtomic(srcFile, dstFile, 0o644); err != nil || bkp == "" {
		t.Fatalf("CopyFileAtomic update = (%q, %v)", bkp, err)
	}

	// Verify symlink entries are rejected by default and escape symlinks are rejected even when enabled
	if runtime.GOOS != "windows" {
		var symBuf bytes.Buffer
		szw := zip.NewWriter(&symBuf)
		hdr := &zip.FileHeader{Name: "escape-link"}
		hdr.SetMode(os.ModeSymlink | 0o777)
		sw, _ := szw.CreateHeader(hdr)
		_, _ = sw.Write([]byte("../../outside"))
		_ = szw.Close()

		symZip := filepath.Join(t.TempDir(), "sym.zip")
		_ = os.WriteFile(symZip, symBuf.Bytes(), 0o644)
		if _, err := ExtractZip(symZip, t.TempDir(), ExtractZipOptions{AllowBundleSymlinks: false}); err == nil {
			t.Fatal("expected symlink rejected when AllowBundleSymlinks is false")
		}
		if _, err := ExtractZip(symZip, t.TempDir(), ExtractZipOptions{AllowBundleSymlinks: true}); err == nil {
			t.Fatal("expected escaping bundle symlink rejected")
		}
	}
}

func TestDownloadVerifiedContext(t *testing.T) {
	t.Parallel()
	payload := []byte("verified-plugin-payload")
	sum := sha256.Sum256(payload)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != UserAgent {
			http.Error(w, "bad ua", http.StatusBadRequest)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "plugin.js")

	if _, err := DownloadVerifiedContext(t.Context(), srv.Client(), srv.URL, dest, "0000", 0o644); err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if _, err := os.Stat(dest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("corrupt download should not create destination file")
	}

	n, err := DownloadVerifiedContext(t.Context(), srv.Client(), srv.URL, dest, hexSum, 0o644)
	if err != nil {
		t.Fatalf("DownloadVerified valid: %v", err)
	}
	if n != int64(len(payload)) {
		t.Fatalf("written = %d, want %d", n, len(payload))
	}

	// Verify WriteLimitedReaderAtomic rejects oversized streams without creating destination
	limitedDest := filepath.Join(dir, "limited.bin")
	if _, _, err := WriteLimitedReaderAtomic(limitedDest, strings.NewReader("0123456789"), 4, 0o644); err == nil {
		t.Fatal("expected size limit error")
	}
	if _, err := os.Stat(limitedDest); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("oversized write should not create target")
	}
}
