package main

import (
	"golang.org/x/text/unicode/norm"
	"regexp"
)

// matchTextOrScript reports whether s matches either the ASCII-anchored
// textRegex/titleRegex (re) OR the non-Latin scriptRegex (sr). Either being nil
// is treated as "no match for that arm". This is the single place that fuses
// Latin and non-Latin link/title/body matching.
func matchTextOrScript(re, sr *regexp.Regexp, s string) bool {
	s = norm.NFC.String(s)
	if re != nil && re.MatchString(s) {
		return true
	}
	if sr != nil && sr.MatchString(s) {
		return true
	}
	return false
}

// matchPathOnly tests whether `urlPath` (case-folded) appears to match any of
// `regs`.
func matchPathOnly(urlPath string, regs []*regexp.Regexp) bool {
	for _, re := range regs {
		if re.MatchString(urlPath) {
			return true
		}
	}
	return false
}

// unicodePhraseRE uses full Unicode word boundaries. Go's \b is ASCII-only:
// it cannot terminate confidentialité and can match privacy inside préprivacy.
// Combining marks, digits and underscores continue a word, just as letters do.
func unicodePhraseRE(phrase string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\p{L}\p{N}\p{M}_])` + phrase + `(?:$|[^\p{L}\p{N}\p{M}_])`)
}
