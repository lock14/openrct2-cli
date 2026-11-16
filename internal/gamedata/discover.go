// Package gamedata discovers and validates RollerCoaster Tycoon 1 and 2 installations
// across Steam libraries, GOG directories, Wine prefixes, and local paths.
package gamedata

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

var vdfPathRegex = regexp.MustCompile(`"path"\s*"([^"]+)"`)

// ParseVDFPaths extracts folder paths from a Steam libraryfolders.vdf stream.
func ParseVDFPaths(r io.Reader) []string {
	var paths []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		matches := vdfPathRegex.FindStringSubmatch(scanner.Text())
		if len(matches) > 1 {
			p := strings.TrimSpace(matches[1])
			p = strings.ReplaceAll(p, `\\`, `\`)
			if p != "" {
				paths = append(paths, p)
			}
		}
	}
	return paths
}

// ParseLibraryFolders reads a Steam libraryfolders.vdf file and returns existing library roots.
func ParseLibraryFolders(vdfPath string) []string {
	f, err := os.Open(vdfPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var paths []string
	for _, p := range ParseVDFPaths(f) {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			paths = append(paths, p)
		}
	}
	return paths
}

// DefaultSearchRoots returns candidate Steam, GOG, Wine, and local game installation roots.
func DefaultSearchRoots(extraHints ...string) []string {
	home, _ := os.UserHomeDir()
	return SearchRootsForOS(runtime.GOOS, home, extraHints...)
}

// SearchRootsForOS builds deduplicated existing search roots for goos and home.
func SearchRootsForOS(goos, home string, extraHints ...string) []string {
	var roots []string
	for _, h := range extraHints {
		if strings.TrimSpace(h) != "" {
			roots = append(roots, strings.TrimSpace(h))
		}
	}

	switch goos {
	case "darwin":
		if home != "" {
			roots = append(roots,
				filepath.Join(home, "Library", "Application Support", "Steam"),
				filepath.Join(home, "Library", "Application Support", "GOG.com"),
				filepath.Join(home, "GOG Games"),
				filepath.Join(home, "Games"),
			)
		}
		if matches, err := filepath.Glob("/Applications/*Roller*"); err == nil {
			roots = append(roots, matches...)
		}

	case "windows":
		prog86 := os.Getenv("ProgramFiles(x86)")
		if prog86 == "" {
			prog86 = `C:\Program Files (x86)`
		}
		prog := os.Getenv("ProgramFiles")
		if prog == "" {
			prog = `C:\Program Files`
		}
		roots = append(roots,
			filepath.Join(prog86, "Steam"),
			filepath.Join(prog, "Steam"),
			filepath.Join(prog86, "GOG Galaxy", "Games"),
			`C:\GOG Games`,
		)
		for _, drive := range []string{"C", "D", "E", "F", "G"} {
			roots = append(roots,
				drive+`:\Steam`,
				drive+`:\SteamLibrary`,
				drive+`:\GOG Games`,
				drive+`:\Games`,
			)
		}

	default: // linux and BSD
		if home != "" {
			roots = append(roots,
				filepath.Join(home, ".local", "share", "Steam"),
				filepath.Join(home, ".steam", "steam"),
				filepath.Join(home, ".steam", "root"),
				filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
				filepath.Join(home, "GOG Games"),
				filepath.Join(home, "Games"),
				filepath.Join(home, ".wine", "drive_c", "GOG Games"),
				filepath.Join(home, ".wine", "drive_c", "Program Files (x86)", "Steam"),
			)
		}
		roots = append(roots,
			"/opt/OpenRCT2/RCT",
			"/opt/OpenRCT2/RCT2",
			"/opt/OpenRCT2/RCT1",
			"/opt/OpenRCT2",
		)
	}

	var expanded []string
	for _, r := range roots {
		expanded = append(expanded, r)
		vdf := filepath.Join(r, "steamapps", "libraryfolders.vdf")
		expanded = append(expanded, ParseLibraryFolders(vdf)...)
	}

	return deduplicateExistingDirs(expanded)
}

func deduplicateExistingDirs(candidates []string) []string {
	var existing []string
	seen := make(map[string]bool)
	for _, r := range candidates {
		if strings.TrimSpace(r) == "" {
			continue
		}
		clean := filepath.Clean(r)
		if resolved, err := filepath.EvalSymlinks(clean); err == nil {
			clean = filepath.Clean(resolved)
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		if fi, err := os.Stat(clean); err == nil && fi.IsDir() {
			existing = append(existing, clean)
		}
	}
	return existing
}

// DiscoveryResult holds discovered RCT2 and RCT1 base game asset paths.
type DiscoveryResult struct {
	RCT2Path    string
	RCT2Edition string
	RCT1Path    string
}

func findCaseInsensitiveChild(parent, childName string) (string, os.FileInfo, bool) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return "", nil, false
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name(), childName) {
			full := filepath.Join(parent, e.Name())
			info, statErr := os.Stat(full)
			if statErr == nil {
				return full, info, true
			}
		}
	}
	return "", nil, false
}

func hasCaseInsensitiveFile(root string, segments ...string) bool {
	cur := root
	for i, seg := range segments {
		next, info, ok := findCaseInsensitiveChild(cur, seg)
		if !ok {
			return false
		}
		if i == len(segments)-1 {
			return !info.IsDir() && info.Size() > 0
		}
		if !info.IsDir() {
			return false
		}
		cur = next
	}
	return false
}

func hasCaseInsensitiveDir(root string, segments ...string) bool {
	cur := root
	for _, seg := range segments {
		next, info, ok := findCaseInsensitiveChild(cur, seg)
		if !ok || !info.IsDir() {
			return false
		}
		cur = next
	}
	return true
}

func looksLikeRCTFolder(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "roller") ||
		strings.Contains(lower, "rct") ||
		strings.Contains(lower, "tycoon")
}

// ValidateRCT2Dir checks whether dir is a valid RollerCoaster Tycoon 2 or RCT Classic root.
// It returns ("RollerCoaster Tycoon 2", true) for retail/Steam/GOG RCT2 installs,
// ("RollerCoaster Tycoon Classic", true) for RCT Classic installs, or ("", false).
func ValidateRCT2Dir(dir string) (string, bool) {
	if strings.TrimSpace(dir) == "" {
		return "", false
	}
	if hasCaseInsensitiveFile(dir, "Data", "g1.dat") && hasCaseInsensitiveDir(dir, "ObjData") {
		return "RollerCoaster Tycoon 2", true
	}
	if hasCaseInsensitiveFile(dir, "Assets", "g1.dat") {
		return "RollerCoaster Tycoon Classic", true
	}
	return "", false
}

// ValidateRCT1Dir checks whether dir is a valid RollerCoaster Tycoon 1 installation.
func ValidateRCT1Dir(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	if !hasCaseInsensitiveFile(dir, "Data", "csg1.dat") {
		return false
	}
	return hasCaseInsensitiveFile(dir, "Data", "csg1i.dat") || hasCaseInsensitiveDir(dir, "Scenarios")
}

// Discover scans candidate roots (and Steam/GOG child folders beneath them) for RCT2 and RCT1.
// Standard RCT2 (Triple Thrill Pack) takes precedence over RCT Classic when both exist.
func Discover(roots []string) DiscoveryResult {
	var candidates []string
	seen := make(map[string]bool)
	addCandidate := func(dir string) {
		clean := filepath.Clean(dir)
		if seen[clean] {
			return
		}
		if fi, err := os.Stat(clean); err == nil && fi.IsDir() {
			seen[clean] = true
			candidates = append(candidates, clean)
		}
	}

	knownSubfolders := []string{
		"Roller Coaster Tycoon 2",
		"RollerCoaster Tycoon 2",
		"RollerCoaster Tycoon 2 Triple Thrill Pack",
		"RollerCoaster Tycoon Classic",
		"RollerCoaster Tycoon Deluxe",
		"Roller Coaster Tycoon Deluxe",
		"RollerCoaster Tycoon",
		"RCT2",
		"RCT1",
		"RCT",
	}

	for _, root := range roots {
		addCandidate(root)
		commonDir := filepath.Join(root, "steamapps", "common")
		for _, sub := range knownSubfolders {
			addCandidate(filepath.Join(commonDir, sub))
			addCandidate(filepath.Join(root, sub))
		}
		if entries, err := os.ReadDir(commonDir); err == nil {
			names := make([]string, 0, len(entries))
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
			sort.Strings(names)
			for _, name := range names {
				if looksLikeRCTFolder(name) {
					addCandidate(filepath.Join(commonDir, name))
				}
			}
		}
		if entries, err := os.ReadDir(root); err == nil {
			for _, e := range entries {
				if e.IsDir() && looksLikeRCTFolder(e.Name()) {
					addCandidate(filepath.Join(root, e.Name()))
				}
			}
		}
	}

	var res DiscoveryResult
	var classicFallback string

	for _, cand := range candidates {
		if res.RCT2Path == "" {
			if edition, ok := ValidateRCT2Dir(cand); ok {
				if edition == "RollerCoaster Tycoon 2" {
					res.RCT2Path = cand
					res.RCT2Edition = edition
				} else if classicFallback == "" {
					classicFallback = cand
				}
			}
		}
		if res.RCT1Path == "" && ValidateRCT1Dir(cand) {
			res.RCT1Path = cand
		}
	}

	if res.RCT2Path == "" && classicFallback != "" {
		res.RCT2Path = classicFallback
		res.RCT2Edition = "RollerCoaster Tycoon Classic"
	}

	return res
}
