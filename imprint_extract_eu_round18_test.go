package main

import (
	"strings"
	"testing"
)

// Round-18 EU-expansion real-evidence fixture, fetched 2026-08-23: Greece.
// Real excerpt from https://thikishop.gr/terms-of-use/ (MASTER ACCESSORIES
// Ι.Κ.Ε. — a real, live Greek phone-accessories retailer's Όροι Χρήσης
// page) — byte-for-byte from the live page's own markup, including its
// literal un-decoded HTML entity (&#8211;, an en-dash) and native Greek
// script throughout (NOT the Latin transliteration).
//
// suffixTable already carried Latin-transliterated Greek legal forms
// ("A.E.", "E.P.E.", "O.E.", "I.K.E.") with an explicit comment: "native
// Greek forms out of scope here". This round's real evidence is exactly
// that gap: the page names its entity "MASTER ACCESSORIES Ι.Κ.Ε." using
// NATIVE Greek Unicode letters (Ι/Κ/Ε — Greek Iota/Kappa/Epsilon, U+0399/
// U+039A/U+0395 — completely different code points from the visually
// similar Latin "I"/"K"/"E"). Running the shipped 1.7.12 extractor against
// this real page extracted almost nothing (only "contact",
// CompletenessScore 33). Found and fixed five compounding real bugs:
//
//  1. suffixTable had no native-Greek-script suffix entries at all. Added
//     "Ι.Κ.Ε." (real-evidence-confirmed on this page) plus "Α.Ε."/"Ε.Π.Ε."/
//     "Ο.Ε." (added alongside it as the native-script counterparts of the
//     already-existing Latin quartet — zero collision risk, Greek script
//     appears nowhere else in the table).
//  2. Greece's ΑΦΜ (domestic tax number) and Γ.Ε.ΜΗ. (General Commercial
//     Registry number) had no vatPatterns entries at all. Added both —
//     "ΑΦΜ" wired as VAT-equivalent (its 9-digit body IS the same number
//     the "EL"-prefixed EU VAT pattern matches, same architecture as
//     Hungary's Adószám), "Γ.Ε.ΜΗ." wired as Register-only — plus a new
//     grVATValid weighted-mod-11 checksum for ΑΦΜ, confirmed against this
//     page's real value (800617296).
//  3. A significant, general Go regexp gotcha surfaced while writing bug 2's
//     patterns: RE2's `\b` (word boundary) is ASCII-only — it never
//     recognises a Greek (or any non-ASCII) letter as a "word" character,
//     so a naive `\bΑΦΜ\b`-style pattern can NEVER match anywhere at all.
//     Every non-ASCII-labelled pattern added in earlier rounds (IČO,
//     Adószám, Cégjegyzékszám) happened to start/end on a plain ASCII
//     character, so this never surfaced before. Fixed by dropping the
//     leading `\b` on both new Greek patterns (the trailing `\b`, anchored
//     on an ASCII digit, stays safe).
//  4. Once bug 2+3 let the patterns actually match, both real label+value
//     pairs ("<strong>ΑΦΜ:</strong> 800617296",
//     "<strong>Αρ. Γ.Ε.ΜΗ.:</strong> 132389607000") sit across a
//     stripTagsLines tag-boundary newline — the SAME general HTML shape
//     round 8's Portuguese fixture and round 16's Czech suffix-splitting
//     bug both hit, but a NEW variant: here the newline falls right after
//     the label's own colon, with a bare digit (not letter-then-punct)
//     following. Added labelColonDigitBoundaryRE (imprint_name.go),
//     narrowly scoped to "colon, newline, optional space, DIGIT" so a
//     genuine section-heading-then-prose break stays untouched. This also
//     exposed a related latent bug: the matched identifier's raw Value
//     (needed byte-exact for extractImprintText's strings.Index proximity
//     lookup) still carried the embedded newline through to
//     formatRegister's OUTPUT — fixed by collapsing whitespace inside
//     formatRegister itself (display time), not inside findIdentifiers
//     (which must keep the raw match for the lookup to keep working).
//  5. Even with the label now reachable on the same line as its value,
//     extractAddressNearEntity had no stop-marker for "αφμ"/"γ.ε.μη.", so
//     both lines (which sit BEFORE the real "Έδρα/Διεύθυνση:" address line
//     on this page) got wrongly absorbed into Address — same failure shape
//     as round 17's Hungarian markers. Added both as skip-past (not stop)
//     markers.
//
// Deliberately NOT added to singleCountryIdentifierKind: "ΑΦΜ" — Greek is
// also an official language of Cyprus, and "ΑΦΜ" reads as a generic
// administrative term (unlike "Γ.Ε.ΜΗ.", a named Greek-only institution,
// which WAS added -> "GR"). Same Czech/Slovak "IČO" caution from round 16.
const thikishopGrFixture = `<!DOCTYPE html>
<html lang="el">
<head><title>Όροι Χρήσης</title></head>
<body>
<div class="terms-section"><h2>2. Στοιχεία Επιχείρησης – Επικοινωνία</h2>
<p>Το Thikishop.gr ανήκει στην εταιρεία:</p>
<div class="company-info"><h3>MASTER ACCESSORIES Ι.Κ.Ε.</h3>
<p><strong>ΑΦΜ:</strong> 800617296 &#8211; Δ.Ο.Υ. Νίκαιας</p>
<p><strong>Αρ. Γ.Ε.ΜΗ.:</strong> 132389607000</p>
<p><strong>Έδρα/Διεύθυνση:</strong> Λεωφ. Θηβών 228, Άγιος Ιωάννης Ρέντης, Τ.Κ. 18233, Αττική, Ελλάδα</p>
<p>Τηλέφωνο: 210 49 29 089</p>
<p>E-mail: <a href="mailto:info@thikishop.gr">info@thikishop.gr</a></p>
</div>
</div>
</body>
</html>`

func TestExtractImprintFieldsThikishopGrRealEvidence(t *testing.T) {
	im := extractImprintFields("https://thikishop.gr/terms-of-use/", thikishopGrFixture, "thikishop.gr")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "MASTER ACCESSORIES Ι.Κ.Ε."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "Ι.Κ.Ε." {
		t.Errorf("Suffix = %q, want native Greek \"Ι.Κ.Ε.\"", im.Suffix)
	}
	if im.Country != "GR" {
		t.Errorf("Country = %q, want GR", im.Country)
	}
	const wantAddress = "Λεωφ. Θηβών 228, Άγιος Ιωάννης Ρέντης, Τ.Κ. 18233, Αττική, Ελλάδα"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (must NOT include the ΑΦΜ/Γ.Ε.ΜΗ. lines)", im.Address, wantAddress)
	}
	if strings.Contains(im.Register, "\n") {
		t.Errorf("Register = %q, must not contain a literal newline", im.Register)
	}
	if !strings.Contains(im.Register, "132389607000") {
		t.Errorf("Register = %q, want it to contain the real Γ.Ε.ΜΗ. value 132389607000", im.Register)
	}
	if im.VAT != "800617296" {
		t.Errorf("VAT = %q, want 800617296 (the real ΑΦΜ)", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Greek ruleset added this round)", im.Ruleset)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}

// TestGrVATChecksum is a direct unit test of the new grVATValid wiring: the
// real ΑΦΜ found during this round's search must pass, and a flipped last
// digit must fail.
func TestGrVATChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"thikishop.gr real ΑΦΜ", "ΑΦΜ: 800617296", checksumValid},
		{"flipped last digit must fail", "ΑΦΜ: 800617295", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("ΑΦΜ", tc.raw, "EL"); got != tc.want {
				t.Errorf("validateIdentifier(ΑΦΜ, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("Γ.Ε.ΜΗ."); got != "GR" {
		t.Errorf("singleCountryIdentifierKind(Γ.Ε.ΜΗ.) = %q, want GR", got)
	}
	if got := singleCountryIdentifierKind("ΑΦΜ"); got != "" {
		t.Errorf("singleCountryIdentifierKind(ΑΦΜ) = %q, want empty (deliberately not single-country — see doc comment)", got)
	}
}

// TestGreekVATPatternHasNoLeadingWordBoundary is a direct regression test
// for the RE2 ASCII-only-\b gotcha this round found: a naive `\bΑΦΜ` would
// never match Greek text at all. Confirms the actual shipped pattern
// (vatPatterns, imprint_vat.go) matches real Greek label+value text.
func TestGreekVATPatternHasNoLeadingWordBoundary(t *testing.T) {
	ids := findIdentifiers("ΑΦΜ: 800617296", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "ΑΦΜ" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match \"ΑΦΜ: 800617296\" — got %v", ids)
	}
}

// TestLabelColonDigitBoundaryCollapse is a direct unit test of the new
// stripTagsLines fix: a tag boundary right after a label's colon, with a
// digit following, must collapse (letting the label and value land on the
// same line) — but a colon-newline followed by non-digit prose (a genuine
// section-heading break) must NOT collapse.
func TestLabelColonDigitBoundaryCollapse(t *testing.T) {
	digitCase := stripTagsLines(`<p><strong>ΑΦΜ:</strong> 800617296</p>`)
	if !strings.Contains(digitCase, "ΑΦΜ: 800617296") {
		t.Errorf("stripTagsLines(colon+digit) = %q, want it to contain \"ΑΦΜ: 800617296\" (newline must collapse)", digitCase)
	}

	proseCase := stripTagsLines(`<p><strong>Steps:</strong> First, read the manual.</p>`)
	if strings.Contains(proseCase, "Steps: First") {
		t.Errorf("stripTagsLines(colon+prose) = %q, must NOT collapse a colon-then-prose break", proseCase)
	}
}

// TestGreekRegisterMarkersSkipNotBreak is a direct unit test of the
// extractAddressNearEntity fix: "αφμ"/"γ.ε.μη." lines must be skipped
// (continue), not treated as a hard stop — the real address that follows
// them on thikishop.gr's real page must still be found.
func TestGreekRegisterMarkersSkipNotBreak(t *testing.T) {
	text := "MASTER ACCESSORIES Ι.Κ.Ε.\nΑΦΜ: 800617296\nΑρ. Γ.Ε.ΜΗ.: 132389607000\nΈδρα/Διεύθυνση: Λεωφ. Θηβών 228, Τ.Κ. 18233, Αττική, Ελλάδα"
	got := extractAddressNearEntity(text, "MASTER ACCESSORIES Ι.Κ.Ε.")
	const want = "Έδρα/Διεύθυνση: Λεωφ. Θηβών 228, Τ.Κ. 18233, Αττική, Ελλάδα"
	if got != want {
		t.Errorf("extractAddressNearEntity = %q, want %q", got, want)
	}
}
