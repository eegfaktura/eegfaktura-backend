// Package public embeds the default mail templates that ship in the binary.
//
// They are the last-resort source for mail rendering: at runtime the data
// volume overrides them (see parser.resolveTemplateSource) — a per-tenant
// templates dir first, then the global one. The embed guarantees a working
// default, so a fresh deployment renders the activation and completion mails
// without any template being seeded onto the PVC.
package public

import "embed"

//go:embed templates
var Templates embed.FS
