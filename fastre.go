package main

import (
	"html"
	"regexp"
	"regexp/syntax"
	"strings"
	"unicode/utf8"
)

// This file holds the CPU-oriented text primitives of the detector.
//
// Profiling the service on 150 real domains (see CHANGELOG 1.9.0) showed ~77 %
// of all CPU inside Go's regexp NFA: every verification re-ran 8-12 `(?i)`
// alternation regexes over a 64 KB body (and homepageBlockRE over up to 4 MB),
// and `(?i)` makes the NFA call unicode.SimpleFold per rune. Go's regexp has no
// multi-literal prefilter, so a body that does NOT match (the common case)
// costs a full NFA scan.
//
// The fix keeps every regular expression (so the accepted language is
// unchanged) and puts a sound, cheap necessary-condition in front of it:
// fastRE extracts, from the parsed regexp, a set of lowercase literals of which
// at least one MUST occur in any match, and only runs the NFA when the
// lower-cased text contains one of them. text-without-the-vocabulary → one
// strings.Contains pass per literal (SIMD-accelerated), no NFA.

// fastRE is a (?i) regexp plus a prefilter. anchors == nil means no sound
// prefilter could be derived, so the regexp always runs.
type fastRE struct {
	re      *regexp.Regexp
	anchors []string
}

// newFastRE wraps re. The regexp must be case-insensitive for the prefilter to
// be sound (all literals are compared lower-cased); a case-sensitive regexp
// simply gets no prefilter.
func newFastRE(re *regexp.Regexp) *fastRE {
	f := &fastRE{re: re}
	prog, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return f
	}
	if set, ok := requiredLiterals(prog); ok {
		f.anchors = set
	}
	return f
}

// minAnchorBytes is the shortest literal worth prefiltering on: shorter ones
// occur in almost every page and would only add a scan.
const minAnchorBytes = 3

// requiredLiterals returns a set S of lowercase literals such that every string
// matched by n contains at least one element of S, or ok=false when no such
// non-trivial set exists.
func requiredLiterals(n *syntax.Regexp) (set []string, ok bool) {
	switch n.Op {
	case syntax.OpLiteral:
		if n.Flags&syntax.FoldCase == 0 {
			// Case-sensitive literal: only sound against lower-cased text if it
			// has no upper-case rune. Be conservative.
			if strings.ToLower(string(n.Rune)) != string(n.Rune) {
				return nil, false
			}
		}
		s := strings.ToLower(string(n.Rune))
		if len(s) < minAnchorBytes {
			return nil, false
		}
		return []string{s}, true
	case syntax.OpCapture, syntax.OpPlus:
		return requiredLiterals(n.Sub[0])
	case syntax.OpRepeat:
		if n.Min >= 1 {
			return requiredLiterals(n.Sub[0])
		}
		return nil, false
	case syntax.OpConcat:
		// All children are mandatory, so any one child's set is a valid set for
		// the concatenation; keep the most selective (longest shortest literal).
		var best []string
		bestScore := -1
		for _, c := range n.Sub {
			s, ok := requiredLiterals(c)
			if !ok {
				continue
			}
			score := 1 << 30
			for _, x := range s {
				if len(x) < score {
					score = len(x)
				}
			}
			// Prefer fewer alternatives at equal length.
			score = score*16 - len(s)
			if score > bestScore {
				best, bestScore = s, score
			}
		}
		if best == nil {
			return nil, false
		}
		return best, true
	case syntax.OpAlternate:
		var all []string
		for _, c := range n.Sub {
			s, ok := requiredLiterals(c)
			if !ok {
				return nil, false
			}
			all = append(all, s...)
		}
		return all, len(all) > 0
	}
	return nil, false
}

// Match reports whether the regexp matches text. lower MUST be
// strings.ToLower(text); it is passed separately so one lower-cased copy is
// shared by many regexes.
func (f *fastRE) Match(text, lower string) bool {
	if f.anchors != nil {
		hit := false
		for _, a := range f.anchors {
			if strings.Contains(lower, a) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}
	return f.re.MatchString(text)
}

// MatchString is the convenience form for callers with a single short text.
func (f *fastRE) MatchString(text string) bool {
	if f.anchors != nil && len(text) > 256 {
		return f.Match(text, strings.ToLower(text))
	}
	return f.re.MatchString(text)
}

// ---------------------------------------------------------------------------
// Linear-time HTML helpers (replace regexp-based tag stripping / extraction).
// ---------------------------------------------------------------------------

// indexFold returns the index of the first case-insensitive occurrence of the
// ASCII needle (already lower-case) in s at or after from, or -1.
func indexFold(s, needle string, from int) int {
	n := len(needle)
	if n == 0 || from < 0 {
		return -1
	}
	first := needle[0]
	upper := first
	if first >= 'a' && first <= 'z' {
		upper = first - 32
	}
	for i := from; i+n <= len(s); i++ {
		c := s[i]
		if c != first && c != upper {
			continue
		}
		if equalFoldASCII(s[i:i+n], needle) {
			return i
		}
	}
	return -1
}

func equalFoldASCII(s, lowerNeedle string) bool {
	if len(s) != len(lowerNeedle) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		if c != lowerNeedle[i] {
			return false
		}
	}
	return true
}

func isTagNameEnd(c byte) bool {
	return c == '>' || c == '/' || c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// invisibleTags are elements whose content is never user-visible text.
var invisibleTags = []string{"script", "style", "noscript", "svg", "template", "iframe", "object"}

// visibleText returns the human-visible text of an HTML document: tags,
// comments and the content of <script>/<style>/<noscript>/<svg>/... removed,
// entities decoded, whitespace collapsed to single spaces. At most limit bytes
// of text are produced (0 = unlimited), which bounds every downstream scan.
//
// It is the single replacement for the old regexp stripTags on bodies: the old
// version left <script> text in place, so JavaScript such as
// console.log("element not found") tripped the soft-404 gate on genuine legal
// pages, and cookie-banner scripts confirmed /cookies pages that had no cookie
// text at all.
func visibleText(src string, limit int) string {
	var b strings.Builder
	if limit > 0 {
		b.Grow(min(limit, len(src)))
	} else {
		b.Grow(len(src) / 4)
	}
	lastSpace := true
	i := 0
	for i < len(src) {
		if limit > 0 && b.Len() >= limit {
			break
		}
		c := src[i]
		if c != '<' {
			// Plain text run up to the next '<'.
			j := strings.IndexByte(src[i:], '<')
			end := len(src)
			if j >= 0 {
				end = i + j
			}
			for k := i; k < end; k++ {
				if limit > 0 && b.Len() >= limit {
					break
				}
				ch := src[k]
				if ch == ' ' || ch == '\n' || ch == '\t' || ch == '\r' || ch == '\f' || ch == '\v' {
					if !lastSpace {
						b.WriteByte(' ')
						lastSpace = true
					}
					continue
				}
				// U+00A0 (c2 a0) counts as whitespace too.
				if ch == 0xC2 && k+1 < end && src[k+1] == 0xA0 {
					if !lastSpace {
						b.WriteByte(' ')
						lastSpace = true
					}
					k++
					continue
				}
				b.WriteByte(ch)
				lastSpace = false
			}
			i = end
			continue
		}
		// c == '<'
		if strings.HasPrefix(src[i:], "<!--") {
			e := strings.Index(src[i+4:], "-->")
			if e < 0 {
				break
			}
			i += 4 + e + 3
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		// Is this a real tag? (letter or '/' + letter or '!' follows)
		if i+1 >= len(src) {
			break
		}
		nx := src[i+1]
		isTag := (nx >= 'a' && nx <= 'z') || (nx >= 'A' && nx <= 'Z') || nx == '/' || nx == '!' || nx == '?'
		if !isTag {
			b.WriteByte('<')
			lastSpace = false
			i++
			continue
		}
		skipped := false
		if nx != '/' && nx != '!' && nx != '?' {
			for _, name := range invisibleTags {
				if i+1+len(name) < len(src) && equalFoldASCII(src[i+1:i+1+len(name)], name) && isTagNameEnd(src[i+1+len(name)]) {
					// Skip to the matching close tag.
					closeAt := indexFold(src, "</"+name, i+1+len(name))
					if closeAt < 0 {
						i = len(src)
					} else {
						gt := strings.IndexByte(src[closeAt:], '>')
						if gt < 0 {
							i = len(src)
						} else {
							i = closeAt + gt + 1
						}
					}
					skipped = true
					break
				}
			}
		}
		if skipped {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		gt := strings.IndexByte(src[i:], '>')
		if gt < 0 {
			break
		}
		i += gt + 1
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	out := strings.TrimSpace(b.String())
	if strings.IndexByte(out, '&') >= 0 {
		out = html.UnescapeString(out)
		out = collapseWS(out)
	}
	return out
}

// collapseWS maps every whitespace run (including U+00A0, which
// html.UnescapeString yields for &nbsp;) to one ASCII space and trims.
func collapseWS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastSpace := true
	for _, r := range s {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' || r == '\f' || r == '\v' || r == 0xA0 {
			if !lastSpace {
				b.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}

// stripTags is the linear-time replacement for the old regexp implementation:
// text content with tags removed, entities decoded and whitespace collapsed.
// (Unlike visibleText it keeps <script>/<style> text; it is used for short
// fragments such as anchor inner-HTML.)
func stripTags(src string) string {
	if strings.IndexByte(src, '<') < 0 && strings.IndexByte(src, '&') < 0 {
		return collapseWS(src)
	}
	var b strings.Builder
	b.Grow(len(src))
	in := false
	start := 0
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case in:
			if c == '>' {
				in = false
				b.WriteByte(' ')
			}
		case c == '<':
			in = true
			start = i
		default:
			b.WriteByte(c)
		}
	}
	if in {
		// Unterminated tag: like the old `<[^>]+>` regexp, leave the dangling
		// "<..." remainder in place.
		b.WriteString(src[start:])
	}
	return collapseWS(html.UnescapeString(b.String()))
}

// tagInner returns the raw inner HTML of the first <name ...>...</name> element
// in src (case-insensitive), or "". Linear time; replaces
// `(?is)<name[^>]*>(.*?)</name>`.
func tagInner(src, name string) string {
	open := "<" + name
	from := 0
	for {
		i := indexFold(src, open, from)
		if i < 0 {
			return ""
		}
		after := i + len(open)
		if after >= len(src) {
			return ""
		}
		if !isTagNameEnd(src[after]) {
			from = after
			continue
		}
		gt := strings.IndexByte(src[after:], '>')
		if gt < 0 {
			return ""
		}
		start := after + gt + 1
		end := indexFold(src, "</"+name, start)
		if end < 0 {
			return ""
		}
		return src[start:end]
	}
}

// cleanText decodes entities and collapses whitespace of a raw HTML fragment.
func cleanText(raw string) string { return stripTags(raw) }

// truncateUTF8 cuts s to at most n bytes without splitting a rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
