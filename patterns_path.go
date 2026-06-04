package main

import (
	"regexp"
)

// pathRE compiles a set of path-segment regexes. A path matches if a listed
// token appears as a path segment (case-insensitive): `/token`, `/token/`,
// `/token.html`, `/foo/token`, `/de/token`, etc. Locale-prefixed paths
// (`/en/privacy`, `/de/datenschutz`) match naturally because the token is a
// segment.
func pathRE(tokens ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tokens))
	for _, t := range tokens {
		re := regexp.MustCompile(`(?i)(^|/)` + regexp.QuoteMeta(t) + `(/|\.html?|\.php|\.aspx?|$)`)
		out = append(out, re)
	}
	return out
}

// pathREContains compiles regexes that match if the token appears anywhere
// inside a path segment (not only as a whole segment). Use sparingly for
// tokens distinctive enough that a substring match is still high-precision —
// "cookie"/"cookies" qualify (almost no non-cookie page embeds the word), but
// "terms" or "legal" do NOT (they collide with thermometer, paralegal, …).
func pathREContains(tokens ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tokens))
	for _, t := range tokens {
		re := regexp.MustCompile(`(?i)(^|/)[^/]*` + regexp.QuoteMeta(t) + `[^/]*(/|$)`)
		out = append(out, re)
	}
	return out
}
