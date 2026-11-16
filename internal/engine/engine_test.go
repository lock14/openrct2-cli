package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
)

func buildZipBytes(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstallerLinuxMacWindowsAndUpdateSkip(t *testing.T) {
	t.Parallel()

	appImageBytes := []byte("#!/bin/sh\necho OpenRCT2-AppImage\n")
	appImageSum := sha256.Sum256(appImageBytes)
	appImageHex := hex.EncodeToString(appImageSum[:])

	macZip := buildZipBytes(t, map[string]string{
		"OpenRCT2.app/Contents/Info.plist":     "<plist><key>CFBundleExecutable</key><string>OpenRCT2</string></plist>",
		"OpenRCT2.app/Contents/MacOS/OpenRCT2": "mac-binary",
	})
	macSum := sha256.Sum256(macZip)
	macHex := hex.EncodeToString(macSum[:])

	winZip := buildZipBytes(t, map[string]string{
		"openrct2.exe":     "win-exe",
		"openrct2-cli.exe": "win-cli-exe",
		"openrct2.dll":     "win-dll",
		"data/g2.dat":      "g2-data",
	})
	winSum := sha256.Sum256(winZip)
	winHex := hex.EncodeToString(winSum[:])

	downloads := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/OpenRCT2/OpenRCT2/releases/latest":
			rel := GitHubRelease{
				TagName: "v0.5.7",
				Assets: []GitHubAsset{
					{
						Name:               "OpenRCT2-v0.5.7-linux-x86_64.AppImage",
						BrowserDownloadURL: srv.URL + "/dl/OpenRCT2.AppImage",
						Digest:             "sha256:" + appImageHex,
					},
					{
						Name:               "OpenRCT2-v0.5.7-macos-universal.zip",
						BrowserDownloadURL: srv.URL + "/dl/macos.zip",
						Digest:             "sha256:" + macHex,
					},
					{
						Name:               "OpenRCT2-v0.5.7-windows-portable-x64.zip",
						BrowserDownloadURL: srv.URL + "/dl/win.zip",
						Digest:             "sha256:" + winHex,
					},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case "/dl/OpenRCT2.AppImage":
			downloads++
			_, _ = w.Write(appImageBytes)
		case "/dl/macos.zip":
			downloads++
			_, _ = w.Write(macZip)
		case "/dl/win.zip":
			downloads++
			_, _ = w.Write(winZip)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cat, err := catalog.LoadCurated()
	if err != nil {
		t.Fatal(err)
	}
	ch := cat.Channels["release"]

	// 1. Linux AppImage + idempotent update check
	linuxRoot := t.TempDir()
	linuxPaths := config.ResolveFor("linux")
	linuxPaths.SetConfigDir(filepath.Join(linuxRoot, "cfg"))
	linuxPaths.SetBinDirForOS("linux", filepath.Join(linuxRoot, "bin"))

	var out bytes.Buffer
	ins := NewInstaller(linuxPaths, &out)
	ins.APIBase = srv.URL
	ins.Client = srv.Client()
	ins.GOOS = "linux"
	ins.GOARCH = "amd64"

	rec, err := ins.Install(context.Background(), ch, false, false)
	if err != nil {
		t.Fatalf("Linux Install: %v", err)
	}
	if rec.Tag != "v0.5.7" {
		t.Fatalf("rec.Tag = %q", rec.Tag)
	}
	prevDownloads := downloads
	if _, err := ins.Install(context.Background(), ch, false, true); err != nil {
		t.Fatalf("Linux checkUpdate: %v", err)
	}
	if downloads != prevDownloads {
		t.Fatalf("expected checkUpdate to skip re-download when already on v0.5.7")
	}

	// Verify PrepareLaunch dry-run preview
	plan, err := PrepareLaunch(linuxPaths, LaunchOptions{
		TargetOS:  "linux",
		Headless:  true,
		DryRun:    true,
		Target:    "Six Flags.park",
		ExtraArgs: []string{"--port", "11753"},
	})
	if err != nil {
		t.Fatalf("PrepareLaunch: %v", err)
	}
	preview := FormatCommandPreview(plan.Binary, plan.Args)
	if !strings.Contains(preview, "--headless") || !strings.Contains(preview, `"Six Flags.park"`) {
		t.Fatalf("unexpected preview: %s", preview)
	}

	// 2. macOS .app bundle + POSIX wrapper
	macRoot := t.TempDir()
	macPaths := config.ResolveFor("darwin")
	macPaths.SetConfigDir(filepath.Join(macRoot, "cfg"))
	macPaths.BinDir = filepath.Join(macRoot, "bin")
	macPaths.SetAppsDir("darwin", filepath.Join(macRoot, "Apps"))

	insMac := NewInstaller(macPaths, &out)
	insMac.APIBase = srv.URL
	insMac.Client = srv.Client()
	insMac.GOOS = "darwin"
	insMac.GOARCH = "arm64"
	if _, err := insMac.Install(context.Background(), ch, false, false); err != nil {
		t.Fatalf("macOS Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(macRoot, "Apps", "OpenRCT2.app", "Contents", "MacOS", "OpenRCT2")); err != nil {
		t.Fatalf("expected OpenRCT2.app extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(macRoot, "bin", "openrct2")); err != nil {
		t.Fatalf("expected macOS wrapper script: %v", err)
	}

	// 3. Windows portable bundle + cmd shim + headless binary resolution
	winRoot := t.TempDir()
	winPaths := config.ResolveFor("windows")
	winPaths.SetConfigDir(filepath.Join(winRoot, "cfg"))
	winPaths.BinDir = filepath.Join(winRoot, "bin")
	winPaths.EngineDir = filepath.Join(winRoot, "engine")

	insWin := NewInstaller(winPaths, &out)
	insWin.APIBase = srv.URL
	insWin.Client = srv.Client()
	insWin.GOOS = "windows"
	insWin.GOARCH = "amd64"
	if _, err := insWin.Install(context.Background(), ch, false, false); err != nil {
		t.Fatalf("Windows Install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(winRoot, "engine", "data", "g2.dat")); err != nil {
		t.Fatalf("expected Windows data/g2.dat extracted: %v", err)
	}
	winPlan, err := PrepareLaunch(winPaths, LaunchOptions{TargetOS: "windows", Headless: true})
	if err != nil {
		t.Fatalf("Windows headless PrepareLaunch: %v", err)
	}
	if !strings.HasSuffix(winPlan.Binary, "openrct2-cli.exe") {
		t.Fatalf("expected openrct2-cli.exe for Windows headless mode, got %s", winPlan.Binary)
	}

	// 4. Execute DryRun preview and test offline pinned fallback + Zip Slip rejection
	var dryOut bytes.Buffer
	if err := Execute(context.Background(), plan, &dryOut, &dryOut); err != nil {
		t.Fatalf("Execute DryRun: %v", err)
	}
	if !strings.Contains(dryOut.String(), "Six Flags.park") {
		t.Fatalf("DryRun output missing target: %s", dryOut.String())
	}

	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer failSrv.Close()

	pinnedCh := ch
	pinnedRecipe := pinnedCh.Platforms["linux/amd64"]
	pinnedRecipe.Pinned = &catalog.PinnedAsset{
		Version: "v0.5.7-pinned",
		URL:     srv.URL + "/dl/OpenRCT2.AppImage",
		SHA256:  appImageHex,
	}
	pinnedCh.Platforms = map[string]catalog.PlatformRecipe{"linux/amd64": pinnedRecipe}

	insFallback := NewInstaller(linuxPaths, &out)
	insFallback.APIBase = failSrv.URL
	insFallback.Client = failSrv.Client()
	insFallback.GOOS = "linux"
	insFallback.GOARCH = "amd64"
	tag, dlURL, sum, err := insFallback.ResolveAsset(context.Background(), pinnedCh)
	if err != nil || tag != "v0.5.7-pinned" || sum != appImageHex || dlURL == "" {
		t.Fatalf("expected pinned fallback, got (%q, %q, %q, %v)", tag, dlURL, sum, err)
	}

	// Zip-Slip path traversal must be rejected inside extractZipTreeInRoot
	evilZip := buildZipBytes(t, map[string]string{"../escape.exe": "bad"})
	evilPath := filepath.Join(t.TempDir(), "evil.zip")
	_ = os.WriteFile(evilPath, evilZip, 0o644)
	zr, err := zip.OpenReader(evilPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	if err := extractZipTreeInRoot(zr.File, "", t.TempDir(), false); err == nil {
		t.Fatal("expected Zip-Slip archive entry to fail validation")
	}

	// Verify SelectAsset skips .zsync / .sig signature files and matches portable binary
	dl, dig, ok := SelectAsset(catalog.PlatformRecipe{Assets: []string{`^OpenRCT2-.*\.AppImage`}}, []GitHubAsset{
		{Name: "OpenRCT2-v0.5.7.AppImage.zsync", BrowserDownloadURL: "https://x/zsync"},
		{Name: "OpenRCT2-v0.5.7.AppImage", BrowserDownloadURL: "https://x/bin", Digest: "sha256:abcd"},
	})
	if !ok || dl != "https://x/bin" || dig != "abcd" {
		t.Fatalf("SelectAsset = (%q, %q, %v)", dl, dig, ok)
	}

	// Verify intra-bundle symlink extraction inside macOS .app archive
	var symBuf bytes.Buffer
	zw := zip.NewWriter(&symBuf)
	wReg, _ := zw.Create("OpenRCT2.app/Contents/Frameworks/libA.1.dylib")
	_, _ = wReg.Write([]byte("dylib"))
	symHdr := &zip.FileHeader{Name: "OpenRCT2.app/Contents/Frameworks/libA.dylib"}
	symHdr.SetMode(os.ModeSymlink | 0o777)
	wSym, _ := zw.CreateHeader(symHdr)
	_, _ = wSym.Write([]byte("libA.1.dylib"))
	_ = zw.Close()

	symZipPath := filepath.Join(t.TempDir(), "sym.zip")
	_ = os.WriteFile(symZipPath, symBuf.Bytes(), 0o644)
	szr, err := zip.OpenReader(symZipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer szr.Close()
	if err := extractZipTreeInRoot(szr.File, "OpenRCT2.app/", t.TempDir(), true); err != nil {
		t.Fatalf("expected valid intra-bundle symlink to extract: %v", err)
	}
}
