package migrations

import "embed"

// FS contains numbered SQL migration files applied in lexical order.
//
//go:embed *.sql
var FS embed.FS
