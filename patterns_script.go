package main

import (
	"regexp"
	"strings"
)

// scriptRE compiles a case-insensitive alternation of literal non-Latin-script
// phrases with NO ASCII word boundaries. CJK / Cyrillic / Greek / Arabic /
// Devanagari / Thai have no `\b` notion (and Go's `\b` is ASCII-only), so these
// phrases must be matched as bare substrings. The phrases listed are distinctive
// multi-character legal terms (e.g. 利用規約, 개인정보처리방침, Пользовательское
// соглашение) that do not collide with ordinary prose, so a substring match is
// high-precision. Returns nil when no phrases are supplied.
func scriptRE(phrases ...string) *regexp.Regexp {
	if len(phrases) == 0 {
		return nil
	}
	escaped := make([]string, len(phrases))
	for i, p := range phrases {
		escaped[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(?i)(?:` + strings.Join(escaped, "|") + `)`)
}

// scriptRegexFor returns the non-Latin-script confirmation regex for a doc
// type, if any. Used by content verification to confirm a CJK/Cyrillic title or
// body (where the ASCII-anchored titleRegex can never fire).
func scriptRegexFor(t DocType) *regexp.Regexp {
	if p, ok := patternByType[t]; ok {
		return p.scriptRegex
	}
	return nil
}
