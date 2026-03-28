package migrations

import "embed"

// FS embeds the SQL migration files shipped with the backend.
//
//go:embed *.sql
var FS embed.FS
