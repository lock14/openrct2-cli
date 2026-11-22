package catalog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCuratedQueriesAndDefensiveCopies(t *testing.T) {
	t.Parallel()
	cat, err := LoadCurated()
	if err != nil {
		t.Fatalf("LoadCurated: %v", err)
	}
	if len(cat.Channels) != 2 {
		t.Fatalf("channels = %d, want 2", len(cat.Channels))
	}
	if len(cat.Addons) < 10 {
		t.Fatalf("addons = %d, want >= 10", len(cat.Addons))
	}

	ch := cat.FindChannel("stable")
	if ch == nil || ch.ID != "release" {
		t.Fatalf("FindChannel(stable) = %v, want release", ch)
	}
	// Mutating returned ChannelConfig must not mutate Catalog internals
	ch.Aliases[0] = "mutated-alias"
	if cat.FindChannel("stable") == nil {
		t.Fatal("expected FindChannel to return a defensive copy")
	}

	if dev := cat.FindChannel("nightly"); dev == nil || dev.ID != "develop" {
		t.Fatalf("FindChannel(nightly) = %v, want develop", dev)
	}

	cases := []struct {
		query  string
		wantID string
	}{
		{"openmusic", "openmusic"},
		{"music", "openmusic"},
		{"Ride Vehicle Editor", "ride-vehicle-editor"},
		{"rve", "ride-vehicle-editor"},
		{"RideVehicleEditor.js", "ride-vehicle-editor"},
		{"benchwarmer", "benchwarmer"},
	}
	for _, tc := range cases {
		got := cat.FindAddon(tc.query)
		if got == nil || got.ID != tc.wantID {
			t.Errorf("FindAddon(%q) = %v, want %q", tc.query, got, tc.wantID)
		}
	}

	// Mutating returned Addon slice must not mutate Catalog internals
	rve := cat.FindAddon("rve")
	rve.Targets[0] = "mutated.js"
	if cat.FindAddon("rve").Targets[0] == "mutated.js" {
		t.Fatal("expected FindAddon to return a defensive copy")
	}
}

func TestUserCatalogLayerAndQuarantine(t *testing.T) {
	t.Parallel()
	userDir := t.TempDir()

	// Write one malformed JSON file to verify quarantine
	badPath := filepath.Join(userDir, "addon.broken.json")
	if err := os.WriteFile(badPath, []byte(`{"schema_version": 999}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Reject custom addon with invalid SHA-256 digest
	if _, err := SaveUserAddon(userDir, Addon{
		ID:        "bad-hash",
		AddonType: AddonTypePlugin,
		Source: AddonSource{
			URL:        "https://example.com/mod.js",
			SHA256:     "short-hash",
			Mode:       InstallModeFile,
			TargetPath: "plugin/mod.js",
		},
	}); err == nil {
		t.Fatal("expected SaveUserAddon to reject malformed sha256")
	}

	// Add one valid custom plugin
	savedPath, err := SaveUserAddon(userDir, Addon{
		ID:          "custom-helper",
		DisplayName: "Custom Helper",
		AddonType:   AddonTypePlugin,
		Source: AddonSource{
			URL:        "https://example.com/custom-helper.js",
			Mode:       InstallModeFile,
			TargetPath: "plugin/custom-helper.js",
		},
	})
	if err != nil {
		t.Fatalf("SaveUserAddon: %v", err)
	}
	if _, err := os.Stat(savedPath); err != nil {
		t.Fatalf("expected saved file: %v", err)
	}

	cat, err := LoadLayered(userDir, false)
	if err != nil {
		t.Fatalf("LoadLayered non-strict: %v", err)
	}
	if len(cat.Diagnostics) != 1 {
		t.Fatalf("Diagnostics = %d, want 1", len(cat.Diagnostics))
	}
	if got := cat.FindAddon("custom-helper"); got == nil || got.Layer != LayerUser {
		t.Fatalf("expected custom-helper in LayerUser, got %v", got)
	}

	if _, err := LoadLayered(userDir, true); err == nil {
		t.Fatal("expected strict LoadLayered to fail on broken user file")
	}

	removed, err := RemoveUserAddon(userDir, "custom-helper")
	if err != nil || !removed {
		t.Fatalf("RemoveUserAddon = (%v, %v), want (true, nil)", removed, err)
	}

	// Verify DecodeEntityBytes strict validation, path traversal rejection, and personal-path rejection
	badDocs := [][]byte{
		[]byte(``),
		[]byte(`{"schema_version":1,"kind":"addon"}{"extra":1}`),
		[]byte(`{"schema_version":1,"kind":"unknown","id":"x"}`),
		[]byte(`{"schema_version":1,"kind":"addon","id":"BAD_ID","addon_type":"plugin",` +
			`"source":{"url":"https://x","mode":"file","target_path":"plugin/a.js"},"targets":["plugin/a.js"]}`),
		[]byte(`{"schema_version":1,"kind":"addon","id":"x","addon_type":"plugin",` +
			`"source":{"url":"https://x","mode":"file","target_path":"../evil.js"},"targets":["plugin/a.js"]}`),
		[]byte(`{"schema_version":1,"kind":"addon","id":"x","addon_type":"plugin",` +
			`"source":{"url":"https://x","sha256":"bad","mode":"file",` +
			`"target_path":"plugin/a.js"},"targets":["plugin/a.js"]}`),
		[]byte(`{"schema_version":1,"kind":"addon","id":"x","addon_type":"plugin",` +
			`"source":{"url":"https://example.com` + `/Users/` + `alice/x","mode":"file",` +
			`"target_path":"plugin/a.js"},"targets":["plugin/a.js"]}`),
	}
	for i, doc := range badDocs {
		if _, _, err := DecodeEntityBytes(doc, "test.json", LayerCurated); err == nil {
			t.Fatalf("case %d: expected DecodeEntityBytes error", i)
		}
	}
}

func BenchmarkFindAddon(b *testing.B) {
	cat, err := LoadCurated()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_ = cat.FindAddon("Ride Vehicle Editor")
		_ = cat.FindAddon("rve")
		_ = cat.FindAddon("openmusic")
	}
}
