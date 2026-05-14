// Package locales embeds the TOML message catalogs shipped with opps.
// The CLI does not consume these directly; internal/i18n loads the FS
// and report-layer code (M2+) calls i18n.T.
package locales

import "embed"

//go:embed *.toml
var FS embed.FS
