package main

import "testing"

// TestDetectSuffix_UnicodeBoundaryCases pins the migration of this file's
// former vendored containsSuffix/isWordRune/prevRune trio onto go-common's
// wordbound package (see imprint_suffix.go's file-level comment). The bare
// "SL" entry (Spain, medium confidence) is exactly the shape
// go_gdpr_compliance's real-evidence round 26/29 found a false positive on
// ("S.-Sv. Slēgts", Latvian business-hours text) — this pins that
// wordbound.ContainsToken keeps this repo safe from the same class of bug,
// and that real suffix matches are unaffected by the migration.
func TestDetectSuffix_UnicodeBoundaryCases(t *testing.T) {
	if _, _, _, ok := detectSuffix("S.-Sv. Slēgts"); ok {
		t.Errorf(`detectSuffix("S.-Sv. Slēgts") matched a suffix, want no match (real Latvian business-hours text, not a company name)`)
	}

	suf, cc, _, ok := detectSuffix("EUROSTARS HOTEL COMPANY SL")
	if !ok || suf != "SL" || cc != "ES" {
		t.Errorf(`detectSuffix("EUROSTARS HOTEL COMPANY SL") = (%q, %q, ok=%v), want ("SL", "ES", true)`, suf, cc, ok)
	}
}
