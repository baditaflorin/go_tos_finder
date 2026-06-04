package main

import (
	"regexp"
)

// matchTextOrScript reports whether s matches either the ASCII-anchored
// textRegex/titleRegex (re) OR the non-Latin scriptRegex (sr). Either being nil
// is treated as "no match for that arm". This is the single place that fuses
// Latin and non-Latin link/title/body matching.
func matchTextOrScript(re, sr *regexp.Regexp, s string) bool {
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
