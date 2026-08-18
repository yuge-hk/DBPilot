package postgres

import "testing"

func TestQuoteIdent(t *testing.T) {
	cases := map[string]string{
		"simple":     `"simple"`,
		`we"ird`:     `"we""ird"`,
		"my_table":   `"my_table"`,
		`a;DROP x--`: `"a;DROP x--"`,
	}
	for in, want := range cases {
		if got := QuoteIdent(in); got != want {
			t.Errorf("QuoteIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuoteLiteral(t *testing.T) {
	got := QuoteLiteral("secret")
	if got != "$dbpilot$secret$dbpilot$" {
		t.Errorf("QuoteLiteral = %q", got)
	}

	// special chars must round-trip inside the literal untouched
	tricky := `pa'ss$word\x; DROP TABLE x; --`
	got = QuoteLiteral(tricky)
	if got != "$dbpilot$pa'ss$word\\x; DROP TABLE x; --$dbpilot$" {
		t.Errorf("QuoteLiteral(tricky) = %q", got)
	}

	// a value containing the tag must trigger a longer tag
	got = QuoteLiteral("has dbpilot inside")
	if got == "$dbpilot$has dbpilot inside$dbpilot$" {
		t.Errorf("tag collision not handled: %q", got)
	}
}

func TestIsProtectedAccount(t *testing.T) {
	if !IsProtectedAccount("postgres") {
		t.Error("postgres must be protected")
	}
	if !IsProtectedAccount("pg_signal_backend") {
		t.Error("pg_* must be protected")
	}
	if IsProtectedAccount("myapp") {
		t.Error("myapp must not be protected")
	}
}
