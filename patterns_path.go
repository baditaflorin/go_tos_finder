package main

import (
	"regexp"
	"strings"
)

// pathRE compiles a set of path-segment regexes. A path matches if a listed
// token appears as a path segment (case-insensitive): `/token`, `/token/`,
// `/token.html`, `/foo/token`, `/de/token`, etc. Locale-prefixed paths
// (`/en/privacy`, `/de/datenschutz`) match naturally because the token is a
// segment.
func pathRE(tokens ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tokens))
	for _, t := range tokens {
		re := regexp.MustCompile(`(?i)(^|/)` + regexp.QuoteMeta(t) + `(/|\.html?|\.php|\.aspx?|\.pdf|$)`)
		pathTokenSpecs[re] = pathTokenSpec{token: strings.ToLower(t)}
		out = append(out, re)
	}
	return out
}

// pathTokenSpec remembers, for each path regexp, the literal it was built from
// so the hot link-classification loop can use a segment/set match instead of
// running ~100 regexes per anchor (25 % of CPU on link-heavy homepages in the
// 1.8.x profile). The regexes stay the reference definition; see
// TestPathMatcherEquivalentToRegexps.
type pathTokenSpec struct {
	token    string
	contains bool
}

var pathTokenSpecs = map[*regexp.Regexp]pathTokenSpec{}

// pathMatcher is the set-based equivalent of a []*regexp.Regexp built by
// pathRE/pathREContains.
type pathMatcher struct {
	exact    map[string]struct{}
	contains []string
	regs     []*regexp.Regexp // tokens the fast path cannot express (contain '.')
}

func newPathMatcher(regs []*regexp.Regexp) *pathMatcher {
	m := &pathMatcher{exact: map[string]struct{}{}}
	for _, re := range regs {
		spec, ok := pathTokenSpecs[re]
		switch {
		case !ok || strings.Contains(spec.token, ".") || strings.Contains(spec.token, "/"):
			m.regs = append(m.regs, re)
		case spec.contains:
			m.contains = append(m.contains, spec.token)
		default:
			m.exact[spec.token] = struct{}{}
		}
	}
	return m
}

var pathExts = []string{".htm", ".html", ".php", ".asp", ".aspx", ".pdf"}

// pathSegments lower-cases a URL path and splits it into its segments.
func pathSegments(urlPath string) []string {
	return strings.Split(strings.ToLower(urlPath), "/")
}

// match reports whether any segment is a listed token (optionally followed by
// one of the page extensions) or contains a listed substring token.
func (m *pathMatcher) match(urlPath string, segs []string) bool {
	for _, seg := range segs {
		if seg == "" {
			continue
		}
		base, rest := seg, ""
		if i := strings.IndexByte(seg, '.'); i >= 0 {
			base, rest = seg[:i], seg[i:]
		}
		if _, ok := m.exact[base]; ok {
			if rest == "" {
				return true
			}
			for _, e := range pathExts {
				if strings.HasPrefix(rest, e) {
					return true
				}
			}
		}
		for _, c := range m.contains {
			if strings.Contains(seg, c) {
				return true
			}
		}
	}
	for _, re := range m.regs {
		if re.MatchString(urlPath) {
			return true
		}
	}
	return false
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
		pathTokenSpecs[re] = pathTokenSpec{token: strings.ToLower(t), contains: true}
		out = append(out, re)
	}
	return out
}
