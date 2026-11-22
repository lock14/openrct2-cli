package repolint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lock14/openrct2-cli/internal/catalog"
)

func TestLintCuratedCatalogAndTemplates(t *testing.T) {
	t.Parallel()
	cat, err := catalog.LoadCurated()
	if err != nil {
		t.Fatalf("LoadCurated: %v", err)
	}
	var issues []Issue
	lintCatalogEntities(cat, func(where, _ string, _ ...any) {
		issues = append(issues, Issue{Location: where})
	})
	if len(issues) > 0 {
		t.Fatalf("unexpected curated catalog lint issues: %v", issues)
	}

	// Verify SyncReadme round-trip in temp file
	tmpReadme := filepath.Join(t.TempDir(), "README.md")
	sample := "# Title\n\n## Curated asset packs and plugins\n\nold\n\n## Next section\n\nbody\n"
	if err := os.WriteFile(tmpReadme, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SyncReadme(tmpReadme, cat); err != nil {
		t.Fatalf("SyncReadme: %v", err)
	}
	got, _ := os.ReadFile(tmpReadme)
	if !strings.Contains(string(got), "Ride Vehicle Editor") || !strings.Contains(string(got), "## Next section") {
		t.Fatalf("unexpected SyncReadme output:\n%s", string(got))
	}
}

func TestLintMarkdownDocDetectsViolations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	badReadme := "# Title\n\nSee [broken](#missing-anchor) and [bad](missing.md).\n\n---\n\n```json\n{\"unclosed\": 1\n"
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte(badReadme), 0o644); err != nil {
		t.Fatal(err)
	}

	var msgs []string
	lintMarkdownDoc(root, "README.md", func(_, format string, a ...any) {
		msgs = append(msgs, Issue{Message: strings.TrimSpace(format)}.Message)
		_ = a
	})
	if len(msgs) < 3 {
		t.Fatalf("expected broken anchor, broken file link, divider, and unclosed fence errors; got %d", len(msgs))
	}
}

func TestLintLiveRepository(t *testing.T) {
	t.Parallel()
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	issues := LintRepo(repoRoot)
	if len(issues) > 0 {
		var msgs []string
		for _, iss := range issues {
			msgs = append(msgs, iss.String())
		}
		t.Fatalf("live repository lint failed:\n%s", strings.Join(msgs, "\n"))
	}
}
