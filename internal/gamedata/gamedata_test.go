package gamedata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVDFPaths(t *testing.T) {
	t.Parallel()
	sample := `"libraryfolders"
{
	"0"
	{
		"path"		"/mnt/nvme/SteamLibrary"
	}
	"1"
	{
		"path"		"D:\\SteamLibrary"
	}
}`
	got := ParseVDFPaths(strings.NewReader(sample))
	if len(got) != 2 || got[0] != "/mnt/nvme/SteamLibrary" || got[1] != `D:\SteamLibrary` {
		t.Fatalf("ParseVDFPaths = %v", got)
	}
}

func TestValidateAndDiscoverSteamAndGOG(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	// Create mock Steam library with RCT2 (lowercase data/g1.dat to verify case-insensitivity) and RCT1 Deluxe
	steamRoot := filepath.Join(tmp, "Steam")
	rct2Dir := filepath.Join(steamRoot, "steamapps", "common", "Roller Coaster Tycoon 2")
	rct1Dir := filepath.Join(steamRoot, "steamapps", "common", "RollerCoaster Tycoon Deluxe")

	if err := os.MkdirAll(filepath.Join(rct2Dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rct2Dir, "ObjData"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rct2Dir, "data", "G1.DAT"), []byte("rct2-assets"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(rct1Dir, "Data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(rct1Dir, "Scenarios"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rct1Dir, "Data", "CSG1.DAT"), []byte("rct1-assets"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also create RCT Classic to verify standard RCT2 takes precedence over Classic
	classicDir := filepath.Join(steamRoot, "steamapps", "common", "RollerCoaster Tycoon Classic")
	if err := os.MkdirAll(filepath.Join(classicDir, "Assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(classicDir, "Assets", "g1.dat"), []byte("classic-assets"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := Discover([]string{steamRoot})
	if res.RCT2Path != rct2Dir {
		t.Fatalf("RCT2Path = %q, want %q", res.RCT2Path, rct2Dir)
	}
	if res.RCT2Edition != "RollerCoaster Tycoon 2" {
		t.Fatalf("RCT2Edition = %q", res.RCT2Edition)
	}
	if res.RCT1Path != rct1Dir {
		t.Fatalf("RCT1Path = %q, want %q", res.RCT1Path, rct1Dir)
	}

	// Verify Classic fallback when standard RCT2 is absent
	resClassic := Discover([]string{classicDir})
	if resClassic.RCT2Path != classicDir || resClassic.RCT2Edition != "RollerCoaster Tycoon Classic" {
		t.Fatalf("Classic fallback = %+v", resClassic)
	}

	// Verify ParseLibraryFolders on disk and SearchRootsForOS across platforms
	secondaryLib := filepath.Join(tmp, "SecondarySteamLib")
	if err := os.MkdirAll(secondaryLib, 0o755); err != nil {
		t.Fatal(err)
	}
	vdfFile := filepath.Join(steamRoot, "steamapps", "libraryfolders.vdf")
	vdfBody := "\"libraryfolders\"\n{\n  \"0\" { \"path\" \"" +
		strings.ReplaceAll(secondaryLib, `\`, `\\`) + "\" }\n}\n"
	if err := os.WriteFile(vdfFile, []byte(vdfBody), 0o644); err != nil {
		t.Fatal(err)
	}
	libs := ParseLibraryFolders(vdfFile)
	if len(libs) != 1 || libs[0] != filepath.Clean(secondaryLib) {
		t.Fatalf("ParseLibraryFolders = %v, want [%s]", libs, secondaryLib)
	}

	for _, goos := range []string{"linux", "darwin", "windows"} {
		roots := SearchRootsForOS(goos, tmp, steamRoot)
		if len(roots) < 2 {
			t.Fatalf("SearchRootsForOS(%s) = %v, expected steamRoot + secondaryLib", goos, roots)
		}
	}
}

func BenchmarkParseVDFPaths(b *testing.B) {
	sample := `"libraryfolders"
{
	"0" { "path" "/var/games/Steam" }
	"1" { "path" "/mnt/games/SteamLibrary" }
}`
	b.ReportAllocs()
	for b.Loop() {
		_ = ParseVDFPaths(strings.NewReader(sample))
	}
}
