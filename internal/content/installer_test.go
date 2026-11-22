package content

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
	"testing"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/config"
)

func makeZip(t *testing.T, files map[string]string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	_ = zw.Close()
	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestInstallUpgradePruneAndUninstallAddons(t *testing.T) {
	t.Parallel()

	jsPayload := []byte("registerPlugin({ name: 'benchwarmer' });")
	jsSumArr := sha256.Sum256(jsPayload)
	jsSum := hex.EncodeToString(jsSumArr[:])

	musicZipV1, musicSumV1 := makeZip(t, map[string]string{
		"assetpack/openrct2.music.alternative.parkap": "alt-pack-v1",
		"assetpack/openrct2.music.cover.parkap":       "cover-pack-v1",
		"object/official/music/legacy_v1.parkobj":     "legacy-obj",
	})
	musicZipV2, musicSumV2 := makeZip(t, map[string]string{
		"assetpack/openrct2.music.alternative.parkap": "alt-pack-v2",
		"assetpack/openrct2.music.cover.parkap":       "cover-pack-v2",
		"object/official/music/galaxy_v2.parkobj":     "galaxy-obj",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/benchwarmer.js":
			_, _ = w.Write(jsPayload)
		case "/openmusic-v1.zip":
			_, _ = w.Write(musicZipV1)
		case "/openmusic-v2.zip":
			_, _ = w.Write(musicZipV2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	paths := config.ResolveFor("linux")
	paths.SetOpenRCT2Dir(filepath.Join(root, "openrct2"))
	paths.SetConfigDir(filepath.Join(root, "cfg"))
	paths.SetStateDir(filepath.Join(root, "state"))

	// Create an unrelated custom user object to ensure upgrades/uninstalls never touch it
	unrelatedObj := filepath.Join(paths.OpenRCT2Dir, "object", "custom", "myride.parkobj")
	if err := os.MkdirAll(filepath.Dir(unrelatedObj), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelatedObj, []byte("keep-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	fileAddon := catalog.Addon{
		ID:          "benchwarmer",
		DisplayName: "Benchwarmer",
		AddonType:   catalog.AddonTypePlugin,
		Version:     "v1.2.0",
		Source: catalog.AddonSource{
			URL:        srv.URL + "/benchwarmer.js",
			SHA256:     jsSum,
			Mode:       catalog.InstallModeFile,
			TargetPath: "plugin/benchwarmer.js",
		},
		Targets: []string{"plugin/benchwarmer.js"},
	}

	zipAddonV1 := catalog.Addon{
		ID:          "openmusic",
		DisplayName: "OpenMusic",
		AddonType:   catalog.AddonTypeAssetPack,
		Version:     "v1.6.0",
		Source: catalog.AddonSource{
			URL:       srv.URL + "/openmusic-v1.zip",
			SHA256:    musicSumV1,
			Mode:      catalog.InstallModeZip,
			TargetDir: ".",
		},
		Targets: []string{
			"assetpack/openrct2.music.alternative.parkap",
			"assetpack/openrct2.music.cover.parkap",
		},
	}

	var out bytes.Buffer
	ins := NewInstaller(paths, false, &out)
	ins.Client = srv.Client()

	if err := ins.InstallAll(t.Context(), []catalog.Addon{fileAddon, zipAddonV1}); err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	if !IsInstalled(paths.OpenRCT2Dir, fileAddon) || !IsInstalled(paths.OpenRCT2Dir, zipAddonV1) {
		t.Fatal("expected both addons installed")
	}

	// Upgrade openmusic to v2 with Force=true and verify legacy_v1.parkobj is pruned
	zipAddonV2 := zipAddonV1
	zipAddonV2.Version = "v1.6.1"
	zipAddonV2.Source.URL = srv.URL + "/openmusic-v2.zip"
	zipAddonV2.Source.SHA256 = musicSumV2

	insForce := NewInstaller(paths, true, &out)
	insForce.Client = srv.Client()
	if err := insForce.InstallAddon(t.Context(), zipAddonV2); err != nil {
		t.Fatalf("upgrade InstallAddon: %v", err)
	}
	legacyFile := filepath.Join(paths.OpenRCT2Dir, "object", "official", "music", "legacy_v1.parkobj")
	if _, err := os.Stat(legacyFile); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("expected superseded legacy_v1.parkobj pruned on upgrade")
	}

	// Uninstall openmusic and verify all v2 files are removed while benchwarmer + unrelatedObj stay
	removed, err := UninstallAddon(paths, zipAddonV2, &out)
	if err != nil || removed != 3 {
		t.Fatalf("UninstallAddon(openmusic) = (%d, %v), want (3, nil)", removed, err)
	}
	if _, err := os.Stat(unrelatedObj); err != nil {
		t.Fatalf("expected unrelated user object preserved: %v", err)
	}
	if !IsInstalled(paths.OpenRCT2Dir, fileAddon) {
		t.Fatal("expected benchwarmer to remain installed")
	}

	// Verify corrupt addon-receipts.json fails safely instead of clobbering existing receipts
	if err := os.WriteFile(paths.AddonReceiptsFile, []byte("{corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := insForce.InstallAddon(t.Context(), fileAddon); err == nil {
		t.Fatal("expected error when addon-receipts.json is corrupt")
	}
}
