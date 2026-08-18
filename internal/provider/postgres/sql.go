package postgres

import (
	"strings"

	"github.com/jackc/pgx/v5"
)

// QuoteIdent safely quotes a SQL identifier (database, role, schema, table,
// sequence names). It delegates to pgx's own identifier sanitizer which
// correctly escapes double quotes.
func QuoteIdent(name string) string {
	return pgx.Identifier{name}.Sanitize()
}

// QuoteLiteral safely quotes a string literal using dollar-quoting so that
// passwords and other values cannot break out of the statement, regardless
// of standard_conforming_strings.
func QuoteLiteral(s string) string {
	tag := "dbpilot"
	for strings.Contains(s, tag) {
		tag += "x"
	}
	return "$" + tag + "$" + s + "$" + tag + "$"
}
