package redact

import "regexp"

var (
	dsnPattern = regexp.MustCompile(`(postgresql://[^:\s/@]+):([^@\s]+)@`)
	pwPattern  = regexp.MustCompile(`(?i)(password\s*[=:]\s*)([^\s&,;]+)`)
)

// String masks common secret-bearing patterns in a log line or error string.
// It is a best-effort defense-in-depth measure; callers must avoid logging
// secrets in the first place.
func String(s string) string {
	out := dsnPattern.ReplaceAllString(s, `${1}:***@`)
	out = pwPattern.ReplaceAllString(out, `${1}***`)
	return out
}
