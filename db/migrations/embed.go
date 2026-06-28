// Package migrations embeds the goose SQL migrations.
package migrations

import "embed"

// FS holds the embedded migration files.
//
//go:embed *.sql
var FS embed.FS
