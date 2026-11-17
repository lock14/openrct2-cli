package repolint

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/lock14/openrct2-cli/internal/catalog"
	"github.com/lock14/openrct2-cli/internal/fsutil"
)

const addonsHeading = "## Curated asset packs and plugins"

// GenerateAddonsSection renders the Markdown section for all curated asset packs and plugins.
func GenerateAddonsSection(cat *catalog.Catalog) string {
	var b strings.Builder
	b.WriteString(addonsHeading + "\n\n")
	b.WriteString("Every curated asset pack and JavaScript plugin in `catalog/addon.*.json` is verified by SHA-256 ")
	b.WriteString("checksum and installed automatically during `rct setup` ")
	b.WriteString("(or individually with `rct addons install <id>`):\n\n")
	b.WriteString("| Add-on | ID | Type | Category | Author | Version | Description |\n")
	b.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- | :--- |\n")

	if cat != nil {
		for _, ad := range cat.Addons {
			typeLabel := "Plugin"
			if ad.AddonType == catalog.AddonTypeAssetPack {
				typeLabel = "Asset Pack"
			}
			title := ad.EffectiveDisplayName()
			if ad.Homepage != "" {
				title = fmt.Sprintf("[%s](%s)", title, ad.Homepage)
			}
			fmt.Fprintf(
				&b,
				"| %s | `%s` | %s | %s | %s | `%s` | %s |\n",
				title,
				ad.ID,
				typeLabel,
				ad.Category,
				ad.Author,
				ad.Version,
				ad.Description,
			)
		}
	}
	return b.String()
}

// SyncReadme replaces the "## Curated asset packs and plugins" section in readmePath.
func SyncReadme(readmePath string, cat *catalog.Catalog) error {
	raw, err := os.ReadFile(readmePath)
	if err != nil {
		return err
	}
	text := fsutil.NormalizeLF(string(raw))
	start := strings.Index(text, addonsHeading+"\n")
	if start < 0 {
		return errors.New("could not find '## Curated asset packs and plugins' heading in README.md")
	}

	rest := text[start+len(addonsHeading)+1:]
	nextH2 := strings.Index(rest, "\n## ")
	var updated string
	section := GenerateAddonsSection(cat)
	if nextH2 >= 0 {
		updated = text[:start] + section + "\n" + rest[nextH2+1:]
	} else {
		updated = text[:start] + section
	}

	if _, _, err := fsutil.WriteTextAtomicIfChanged(readmePath, updated, 0o644); err != nil {
		return err
	}
	fsutil.RemoveBackups(readmePath)
	return nil
}
