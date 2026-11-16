// Package openrct2cli provides the root embedded filesystem for curated catalog
// entities, OpenRCT2 configuration templates, and FreeDesktop integration assets.
package openrct2cli

import "embed"

// FS holds embedded catalog JSON entities, OpenRCT2 configuration templates,
// and FreeDesktop desktop/icon integration assets.
//
//go:embed catalog/*.json
//go:embed configs/openrct2/config.ini configs/openrct2/shortcuts.json
//go:embed configs/desktop/OpenRCT2.desktop configs/desktop/openrct2.png
var FS embed.FS
