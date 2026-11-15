// Package schema embeds the JSON Schemas of spec/01 section 11 so the binary
// carries them. These files are the only artifact shared between the Go
// ledger and the Python reference verifier (lane note for M1.3).
package schema

import "embed"

//go:embed *.schema.json
var FS embed.FS
