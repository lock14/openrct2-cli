package fsutil

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
	"COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
	"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func isWindowsReservedSegment(part string) bool {
	stem := part
	if dot := strings.IndexByte(stem, '.'); dot >= 0 {
		stem = stem[:dot]
	}
	stem = strings.ToUpper(strings.TrimSpace(stem))
	return windowsReservedNames[stem]
}

// IsValidSHA256 reports whether s is a valid 64-character hexadecimal SHA-256 digest.
func IsValidSHA256(s string) bool {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) != 64 {
		return false
	}
	_, err := hex.DecodeString(trimmed)
	return err == nil
}

// ValidateSHA256Hex normalizes raw to lowercase when non-empty and verifies that it
// is a valid 64-character hexadecimal SHA-256 digest.
func ValidateSHA256Hex(raw string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(raw))
	if trimmed == "" {
		return "", nil
	}
	if !IsValidSHA256(trimmed) {
		return "", fmt.Errorf("invalid 64-character sha256 hex digest %q", raw)
	}
	return trimmed, nil
}

// CleanRel validates and normalizes a relative path (converting backslashes to
// forward slashes), rejecting empty strings, NUL bytes, leading slashes, Windows
// drive-letter prefixes, UNC prefixes, trailing spaces or dots on segments,
// Windows reserved DOS device names, and any "." or ".." path segments.
func CleanRel(rel string) (string, error) {
	if rel == "" {
		return "", errors.New("empty relative path")
	}
	if strings.IndexByte(rel, 0) >= 0 {
		return "", fmt.Errorf("invalid NUL byte in path %q", rel)
	}
	norm := strings.ReplaceAll(rel, "\\", "/")
	if strings.HasPrefix(norm, "/") {
		return "", fmt.Errorf("absolute path %q is not allowed", rel)
	}
	if len(norm) >= 2 && norm[1] == ':' {
		return "", fmt.Errorf("drive-letter path %q is not allowed", rel)
	}
	parts := strings.Split(norm, "/")
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid relative path segment in %q", rel)
		}
		if strings.TrimRight(part, " .") != part {
			return "", fmt.Errorf("trailing space or dot in path segment %q", rel)
		}
		if strings.ContainsRune(part, ':') {
			return "", fmt.Errorf("invalid colon in path segment %q", rel)
		}
		if isWindowsReservedSegment(part) {
			return "", fmt.Errorf("reserved device name in path segment %q", rel)
		}
		cleaned = append(cleaned, part)
	}
	return strings.Join(cleaned, "/"), nil
}

// SafeJoin joins a relative path under root after validating it with CleanRel
// and verifying that the resulting path remains strictly inside root.
func SafeJoin(root, rel string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("empty root directory")
	}
	cleanRel, err := CleanRel(rel)
	if err != nil {
		return "", err
	}
	cleanRoot := filepath.Clean(root)
	target := filepath.Clean(filepath.Join(cleanRoot, filepath.FromSlash(cleanRel)))
	prefix := cleanRoot
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	if target == cleanRoot || !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("path %q escapes root %q", rel, root)
	}
	return target, nil
}
