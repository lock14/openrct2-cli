// Package rctcfg renders, merges, diffs, syncs, and deploys OpenRCT2 configuration
// templates and FreeDesktop launcher integrations.
package rctcfg

import (
	"fmt"
	"slices"
	"strings"
)

// FormatINIString encodes s as a double-quoted OpenRCT2 INI string literal.
func FormatINIString(s string) string {
	escaped := strings.ReplaceAll(s, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

// UnquoteINIString strips surrounding quotes and unescapes OpenRCT2 INI string literals.
func UnquoteINIString(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) >= 2 && trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"' {
		inner := trimmed[1 : len(trimmed)-1]
		var b strings.Builder
		b.Grow(len(inner))
		for i := 0; i < len(inner); i++ {
			if inner[i] == '\\' && i+1 < len(inner) {
				i++
				b.WriteByte(inner[i])
				continue
			}
			b.WriteByte(inner[i])
		}
		return b.String()
	}
	return trimmed
}

// GetINIValue retrieves the raw value assigned to key inside [section] of iniText.
func GetINIValue(iniText, section, key string) (string, bool) {
	curSection := ""
	for line := range strings.SplitSeq(strings.ReplaceAll(iniText, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			curSection = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		if !strings.EqualFold(curSection, section) {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(k), key) {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
}

type iniUpdate struct {
	origKey string
	val     string
}

// SetINIValues updates or appends raw formatted key = value entries inside [section]
// deterministically while preserving comments, blank lines, and ordering of untouched keys.
func SetINIValues(iniText, section string, updates map[string]string) string {
	if len(updates) == 0 {
		return iniText
	}
	lines := strings.Split(strings.ReplaceAll(iniText, "\r\n", "\n"), "\n")
	applied := make(map[string]bool, len(updates))
	lowerUpdates := make(map[string]iniUpdate, len(updates))
	orderedKeys := make([]string, 0, len(updates))
	for k, v := range updates {
		lk := strings.ToLower(k)
		lowerUpdates[lk] = iniUpdate{origKey: k, val: v}
		orderedKeys = append(orderedKeys, lk)
	}
	slices.Sort(orderedKeys)

	curSection := ""
	targetSectionFound := false
	insertAfterIdx := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			curSection = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			if strings.EqualFold(curSection, section) {
				targetSectionFound = true
				insertAfterIdx = i
			}
			continue
		}
		if !strings.EqualFold(curSection, section) {
			continue
		}
		if trimmed == "" || strings.HasPrefix(trimmed, ";") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		k, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		insertAfterIdx = i
		lk := strings.ToLower(strings.TrimSpace(k))
		if upd, want := lowerUpdates[lk]; want {
			lines[i] = fmt.Sprintf("%s = %s", strings.TrimSpace(k), upd.val)
			applied[lk] = true
		}
	}

	var missingLines []string
	for _, lk := range orderedKeys {
		if !applied[lk] {
			upd := lowerUpdates[lk]
			missingLines = append(missingLines, fmt.Sprintf("%s = %s", upd.origKey, upd.val))
		}
	}

	if len(missingLines) > 0 {
		if targetSectionFound && insertAfterIdx >= 0 {
			lines = slices.Insert(lines, insertAfterIdx+1, missingLines...)
		} else {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			lines = append(lines, fmt.Sprintf("[%s]", section))
			lines = append(lines, missingLines...)
		}
	}

	return strings.Join(lines, "\n")
}
