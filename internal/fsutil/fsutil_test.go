package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
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

func TestSymlinkReplacementSkipsTargetBackup(t *testing.T) {
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
	if err := os.Symlink(extTarget, linkPath); err != nil {
		t.Fatal(err)
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

func TestCleanRelAndSafeJoin(t *testing.T) {
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

	if _, err := DownloadVerified(srv.Client(), srv.URL, dest, "0000", 0o644); err == nil {
		t.Fatal("expected sha256 mismatch error")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("corrupt download should not create destination file")
	}

	n, err := DownloadVerified(srv.Client(), srv.URL, dest, hexSum, 0o644)
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
	if _, err := os.Stat(limitedDest); !os.IsNotExist(err) {
		t.Fatal("oversized write should not create target")
	}
}
