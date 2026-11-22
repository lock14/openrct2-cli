// Package repolint validates curated catalog entities, configuration templates,
// documentation typography/link hygiene, and README.md catalog table parity.
package repolint

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

var (
	decorativeEmojiRegex = regexp.MustCompile(
		`([\x{1F300}-\x{1FAFF}\x{26A1}\x{2705}\x{2728}\x{274C}]|:(?:white_check_mark|x):)`,
	)
	italicAsideRegex   = regexp.MustCompile(`(?i)(\*|_)\((?:note|tip|alternatively)\b`)
	boldTableCellRegex = regexp.MustCompile(`^\|\s*\*\*[^*]+\*\*\s*\|`)
	htmlCommentRegex   = regexp.MustCompile(`<!--.*?-->`)
	asciiArrowRegex    = regexp.MustCompile(`(<->|->|<-|=>)`)
	headingLinkRegex   = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
)

type issueReporter func(where, format string, a ...any)

// Issue represents a repository lint violation.
type Issue struct {
	Location string
	Message  string
}

func (i Issue) String() string {
	return fmt.Sprintf("%s: %s", i.Location, i.Message)
}

// LintRepo runs catalog, template, documentation, and README table parity checks on rootDir.
func LintRepo(rootDir string) []Issue {
	var issues []Issue
	addErr := func(where, format string, a ...any) {
		issues = append(issues, Issue{
			Location: where,
			Message:  fmt.Sprintf(format, a...),
		})
	}

	catDir := filepath.Join(rootDir, "catalog")
	cat, err := catalog.LoadFromFS(os.DirFS(rootDir), "catalog", catalog.LayerCurated)
	if err != nil {
		addErr(filepath.ToSlash(catDir), "%v", err)
	} else {
		lintCatalogEntities(cat, addErr)
	}

	lintTemplates(rootDir, addErr)
	lintDocumentation(rootDir, addErr)

	if cat != nil {
		readmePath := filepath.Join(rootDir, "README.md")
		if raw, readErr := os.ReadFile(readmePath); readErr == nil {
			expectedBlock := GenerateAddonsSection(cat)
			if !strings.Contains(fsutil.NormalizeLF(string(raw)), strings.TrimSpace(expectedBlock)) {
				addErr(
					filepath.ToSlash(readmePath),
					"Curated add-ons table is out of sync with catalog/*.json (run 'rct addons build')",
				)
			}
		}
	}

	return issues
}

func lintCatalogEntities(cat *catalog.Catalog, addErr issueReporter) {
	for _, chID := range slices.Sorted(maps.Keys(cat.Channels)) {
		ch := cat.Channels[chID]
		where := ch.SourcePath
		if strings.TrimSpace(ch.Repo) == "" {
			addErr(where, "channel %s missing repo", ch.ID)
		}
		if len(ch.Platforms) == 0 {
			addErr(where, "channel %s defines no platforms", ch.ID)
		}
		for _, plat := range slices.Sorted(maps.Keys(ch.Platforms)) {
			recipe := ch.Platforms[plat]
			if recipe.Unsupported {
				continue
			}
			for _, pat := range recipe.Assets {
				if _, err := regexp.Compile(pat); err != nil {
					addErr(where, "platform %s invalid regex %q: %v", plat, pat, err)
				}
			}
			if recipe.Pinned == nil {
				addErr(where, "platform %s missing pinned fallback", plat)
				continue
			}
			if !strings.HasPrefix(recipe.Pinned.URL, "https://") {
				addErr(where, "platform %s pinned URL must use https://", plat)
			}
			if !fsutil.IsValidSHA256(recipe.Pinned.SHA256) {
				addErr(where, "platform %s invalid 64-char sha256 %q", plat, recipe.Pinned.SHA256)
			}
		}
	}

	for _, ad := range cat.Addons {
		lintAddonEntity(ad, addErr)
	}
}

func lintAddonEntity(ad catalog.Addon, addErr issueReporter) {
	where := ad.SourcePath
	if strings.TrimSpace(ad.DisplayName) == "" {
		addErr(where, "addon %s missing display_name", ad.ID)
	}
	if strings.TrimSpace(ad.Category) == "" {
		addErr(where, "addon %s missing category", ad.ID)
	}
	if strings.TrimSpace(ad.Author) == "" {
		addErr(where, "addon %s missing author", ad.ID)
	}
	if strings.TrimSpace(ad.Version) == "" {
		addErr(where, "addon %s missing version", ad.ID)
	}
	if strings.TrimSpace(ad.Description) == "" {
		addErr(where, "addon %s missing description", ad.ID)
	}
	if !strings.HasPrefix(ad.Homepage, "https://") {
		addErr(where, "addon %s homepage must use https://", ad.ID)
	}
	if !strings.HasPrefix(ad.Source.URL, "https://") {
		addErr(where, "addon %s source.url must use https://", ad.ID)
	}
	if !fsutil.IsValidSHA256(ad.Source.SHA256) {
		addErr(where, "addon %s invalid 64-char sha256 %q", ad.ID, ad.Source.SHA256)
	}
	if ad.Source.Mode == catalog.InstallModeFile {
		if _, err := fsutil.CleanRel(ad.Source.TargetPath); err != nil {
			addErr(where, "addon %s invalid source.target_path: %v", ad.ID, err)
		}
	} else if trimmedDir := strings.TrimSpace(ad.Source.TargetDir); trimmedDir != "" && trimmedDir != "." {
		if _, err := fsutil.CleanRel(trimmedDir); err != nil {
			addErr(where, "addon %s invalid source.target_dir: %v", ad.ID, err)
		}
	}
	for _, t := range ad.Targets {
		if _, err := fsutil.CleanRel(t); err != nil {
			addErr(where, "addon %s invalid target %q: %v", ad.ID, t, err)
		}
		if ad.AddonType == catalog.AddonTypePlugin && !strings.HasSuffix(strings.ToLower(t), ".js") {
			addErr(where, "plugin addon %s target %q must end with .js", ad.ID, t)
		}
	}
}

var requiredConfigINITokens = []string{
	"__FULLSCREEN_WIDTH__",
	"__FULLSCREEN_HEIGHT__",
	"__WINDOW_WIDTH__",
	"__WINDOW_HEIGHT__",
	"__WINDOW_SCALE__",
	`"__GAME_PATH__"`,
	`"__RCT1_PATH__"`,
	`"__SAVE_DIR__"`,
	`"__USER__"`,
}

func lintTemplates(rootDir string, addErr issueReporter) {
	iniPath := filepath.Join(rootDir, "configs", "openrct2", "config.ini")
	if raw, err := os.ReadFile(iniPath); err != nil {
		addErr(filepath.ToSlash(iniPath), "missing config.ini template: %v", err)
	} else {
		text := string(raw)
		where := filepath.ToSlash(iniPath)
		if m := catalog.ScanPersonalPath(text); m != "" {
			addErr(where, "contains personal path %q", m)
		}
		for _, tok := range requiredConfigINITokens {
			if !strings.Contains(text, tok) {
				addErr(where, "missing required placeholder token %s", tok)
			}
		}
	}

	shortcutsPath := filepath.Join(rootDir, "configs", "openrct2", "shortcuts.json")
	if raw, err := os.ReadFile(shortcutsPath); err != nil {
		addErr(filepath.ToSlash(shortcutsPath), "missing shortcuts.json template: %v", err)
	} else if !json.Valid(raw) {
		addErr(filepath.ToSlash(shortcutsPath), "invalid JSON syntax")
	}

	desktopPath := filepath.Join(rootDir, "configs", "desktop", "OpenRCT2.desktop")
	if raw, err := os.ReadFile(desktopPath); err != nil {
		addErr(filepath.ToSlash(desktopPath), "missing OpenRCT2.desktop template: %v", err)
	} else {
		text := string(raw)
		where := filepath.ToSlash(desktopPath)
		if m := catalog.ScanPersonalPath(text); m != "" {
			addErr(where, "contains personal path %q", m)
		}
		if !strings.Contains(text, "Exec=__OPENRCT2_EXEC__ %u") {
			addErr(where, "missing required placeholder 'Exec=__OPENRCT2_EXEC__ %%u'")
		}
	}
}

// GitHubHeadingSlug computes the GitHub-flavored Markdown anchor slug for a heading.
func GitHubHeadingSlug(heading string) string {
	stripped := headingLinkRegex.ReplaceAllString(heading, "$1")
	s := strings.ToLower(strings.TrimSpace(stripped))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' || r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}

func lintDocumentation(rootDir string, addErr issueReporter) {
	yamlFiles := []string{
		filepath.Join(".github", "workflows", "ci.yml"),
		filepath.Join(".github", "workflows", "security.yml"),
		filepath.Join(".github", "workflows", "benchmark.yml"),
		filepath.Join(".github", "dependabot.yml"),
	}
	for _, relYAML := range yamlFiles {
		lintYAMLFile(filepath.Join(rootDir, relYAML), addErr)
	}

	docFiles := []string{
		"README.md",
		"CONTRIBUTING.md",
		"AGENTS.md",
		"SECURITY.md",
		"llms.txt",
		filepath.Join(".github", "PULL_REQUEST_TEMPLATE.md"),
	}
	for _, relDoc := range docFiles {
		lintMarkdownDoc(rootDir, relDoc, addErr)
	}
}

func lintYAMLFile(path string, addErr issueReporter) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := string(raw)
	where := filepath.ToSlash(path)
	if m := catalog.ScanPersonalPath(text); m != "" {
		addErr(where, "contains personal path %q", m)
	}
	if m := decorativeEmojiRegex.FindString(text); m != "" {
		addErr(where, "contains decorative emoji %q", m)
	}
}

type lineLinkRef struct {
	lineNum int
	target  string
}

func lintMarkdownDoc(rootDir, relDoc string, addErr issueReporter) {
	docPath := filepath.Join(rootDir, relDoc)
	raw, err := os.ReadFile(docPath)
	if err != nil {
		return
	}
	where := filepath.ToSlash(docPath)
	text := fsutil.NormalizeLF(string(raw))

	if m := catalog.ScanPersonalPath(text); m != "" {
		addErr(where, "contains personal path %q", m)
	}
	if m := decorativeEmojiRegex.FindString(text); m != "" {
		addErr(where, "contains decorative emoji %q", m)
	}

	isUserFacingDoc := relDoc == "README.md" || relDoc == "CONTRIBUTING.md"
	lines := strings.Split(text, "\n")
	inFence := false
	fenceLang := ""
	fenceStart := 0
	var fenceBuf strings.Builder
	headingSlugs := make(map[string]bool)
	slugCounts := make(map[string]int)
	var links []lineLinkRef

	for idx, line := range lines {
		lineNum := idx + 1
		trimmed := strings.TrimSpace(line)
		if after, isFence := strings.CutPrefix(trimmed, "```"); isFence {
			if !inFence {
				inFence = true
				fenceLang = strings.ToLower(strings.TrimSpace(after))
				fenceStart = lineNum
				fenceBuf.Reset()
			} else {
				if fenceLang == "json" && isUserFacingDoc && !json.Valid([]byte(fenceBuf.String())) {
					addErr(where, "invalid JSON code block starting at line %d", fenceStart)
				}
				inFence = false
				fenceLang = ""
			}
			continue
		}
		if inFence {
			fenceBuf.WriteString(line)
			fenceBuf.WriteByte('\n')
			continue
		}

		lintProseLine(where, line, trimmed, lineNum, isUserFacingDoc, addErr)
		recordHeadingSlug(trimmed, headingSlugs, slugCounts)

		if isUserFacingDoc {
			for _, target := range extractMarkdownLinkTargets(line) {
				links = append(links, lineLinkRef{lineNum: lineNum, target: target})
			}
		}
	}

	if inFence {
		addErr(where, "unclosed code fence starting at line %d", fenceStart)
	}

	if isUserFacingDoc {
		docDir := filepath.Dir(docPath)
		for _, ref := range links {
			validateDocLink(where, docDir, ref, headingSlugs, addErr)
		}
	}
}

func lintProseLine(where, line, trimmed string, lineNum int, checkTableBold bool, addErr issueReporter) {
	if trimmed == "---" {
		addErr(where, "contains horizontal rule divider '---' at line %d", lineNum)
	}
	if checkTableBold {
		if m := boldTableCellRegex.FindString(trimmed); m != "" {
			addErr(where, "contains bolded first-column table cell %q at line %d", strings.TrimSpace(m), lineNum)
		}
	}

	cleaned := htmlCommentRegex.ReplaceAllString(stripInlineCode(line), "")
	if m := italicAsideRegex.FindString(cleaned); m != "" {
		addErr(where, "contains italic parenthetical aside %q", m)
	}
	if m := asciiArrowRegex.FindString(cleaned); m != "" {
		addErr(where, "contains ASCII arrow %q at line %d", m, lineNum)
	}
}

func recordHeadingSlug(trimmed string, headingSlugs map[string]bool, slugCounts map[string]int) {
	if !strings.HasPrefix(trimmed, "#") {
		return
	}
	title := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
	baseSlug := GitHubHeadingSlug(title)
	if baseSlug == "" {
		return
	}
	cnt := slugCounts[baseSlug]
	slugCounts[baseSlug] = cnt + 1
	slug := baseSlug
	if cnt > 0 {
		slug = fmt.Sprintf("%s-%d", baseSlug, cnt)
	}
	headingSlugs[slug] = true
}

func validateDocLink(
	where, docDir string,
	ref lineLinkRef,
	headingSlugs map[string]bool,
	addErr issueReporter,
) {
	target := ref.target
	if anchor, ok := strings.CutPrefix(target, "#"); ok {
		if !headingSlugs[anchor] {
			addErr(where, "broken anchor link %q at line %d", target, ref.lineNum)
		}
		return
	}
	if strings.HasPrefix(target, "http://") ||
		strings.HasPrefix(target, "https://") ||
		strings.HasPrefix(target, "mailto:") {
		return
	}
	relTarget, _, _ := strings.Cut(target, "#")
	if relTarget == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(docDir, filepath.FromSlash(relTarget))); err != nil {
		addErr(where, "broken relative file link %q at line %d", target, ref.lineNum)
	}
}

func stripInlineCode(line string) string {
	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		if line[i] == '`' {
			ticks := 0
			for i+ticks < len(line) && line[i+ticks] == '`' {
				ticks++
			}
			closeIdx := strings.Index(line[i+ticks:], strings.Repeat("`", ticks))
			if closeIdx >= 0 {
				i += ticks + closeIdx + ticks
				continue
			}
			b.WriteString(line[i : i+ticks])
			i += ticks
			continue
		}
		b.WriteByte(line[i])
		i++
	}
	return b.String()
}

func extractMarkdownLinkTargets(line string) []string {
	cleaned := stripInlineCode(line)
	var targets []string
	for pos := 0; pos < len(cleaned); {
		idx := strings.Index(cleaned[pos:], "](")
		if idx < 0 {
			break
		}
		start := pos + idx + 2
		end := strings.IndexByte(cleaned[start:], ')')
		if end < 0 {
			break
		}
		raw := strings.TrimSpace(cleaned[start : start+end])
		if fields := strings.Fields(raw); len(fields) > 0 {
			raw = fields[0]
		}
		if raw != "" {
			targets = append(targets, raw)
		}
		pos = start + end + 1
	}
	return targets
}
