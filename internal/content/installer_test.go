package content

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

func TestInstallAndUninstallFileAndZipAddons(t *testing.T) {
	t.Parallel()

	jsPayload := []byte("registerPlugin({ name: 'benchwarmer' });")
	jsSumArr := sha256.Sum256(jsPayload)
	jsSum := hex.EncodeToString(jsSumArr[:])

	musicZip, musicSum := makeZip(t, map[string]string{
		"assetpack/openrct2.music.alternative.parkap": "alt-pack",
		"assetpack/openrct2.music.cover.parkap":       "cover-pack",
		"object/official/music/galaxy.parkobj":        "galaxy-obj",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/benchwarmer.js":
			_, _ = w.Write(jsPayload)
		case "/openmusic.zip":
			_, _ = w.Write(musicZip)
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

	// Create an unrelated custom user object to ensure UninstallAddon never touches it
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

	zipAddon := catalog.Addon{
		ID:          "openmusic",
		DisplayName: "OpenMusic",
		AddonType:   catalog.AddonTypeAssetPack,
		Version:     "v1.6.1",
		Source: catalog.AddonSource{
			URL:       srv.URL + "/openmusic.zip",
			SHA256:    musicSum,
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

	if err := ins.InstallAll(context.Background(), []catalog.Addon{fileAddon, zipAddon}); err != nil {
		t.Fatalf("InstallAll: %v", err)
	}
	if !IsInstalled(paths.OpenRCT2Dir, fileAddon) || !IsInstalled(paths.OpenRCT2Dir, zipAddon) {
		t.Fatal("expected both addons installed")
	}

	// Uninstall openmusic and verify even the receipt-tracked galaxy.parkobj is removed while unrelatedObj stays
	removed, err := UninstallAddon(paths, zipAddon, &out)
	if err != nil || removed != 3 {
		t.Fatalf("UninstallAddon(openmusic) = (%d, %v), want (3, nil)", removed, err)
	}
	if _, err := os.Stat(unrelatedObj); err != nil {
		t.Fatalf("expected unrelated user object preserved: %v", err)
	}
}
