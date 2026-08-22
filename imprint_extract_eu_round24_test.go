package main

import "testing"

// Round-24 EU-expansion real-evidence fixture, fetched 2026-08-23:
// Lithuania. Real excerpt from
// https://grupinispirkimas.lt/uab-grupinis-pirkimas-taisykles/ (UAB
// "GRUPINIS PIRKIMAS" — a real, live Lithuanian group-buying e-shop's
// Taisyklės page) — byte-for-byte from the live page's own markup,
// including its literal un-decoded HTML entities (&#8211; en-dash,
// &#8222;/&#8221; low-9/right-double quotes, &nbsp;).
//
// Lithuania writes its legal form BEFORE the trading name ("UAB
// „GRUPINIS PIRKIMAS"" — the same prefix-form convention this codebase's
// file-level comment already names for Russian "ООО «Яндекс»", but never
// previously exercised against a real fetched page). Running the shipped
// 1.7.18 extractor against this real page extracted NOTHING at all
// (CompletenessScore 0), despite "UAB" being an existing, already-high-
// confidence suffixTable entry. Found and fixed six compounding real
// bugs:
//
//  1. Lithuania's "Įmonės kodas" (legal-entity code) had no identifier
//     pattern at all. Added it, wired as Register-only (a genuinely
//     SEPARATE number from Lithuania's own PVM kodas/VAT code — unlike
//     Bulgaria's ЕИК/Greece's ΑΦΜ/Croatia's OIB, which ARE their
//     country's VAT body), plus a new ltCompanyCodeValid two-pass
//     weighted-mod-11 checksum, confirmed against TWO independent real
//     values (302983374 on this page, 302662379 independently found the
//     same day on bigbox.lt — though that page's own occurrence sits
//     inside a <script> block and so isn't usable as a fixture itself).
//  2. Applying round 18/19's RE2 lesson: the "Įmonės kodas" pattern
//     needed NO leading `\b` either — "Į" (U+012E) is Latin Extended-A,
//     confirming the ASCII-only-`\b` gotcha is not script-specific,
//     verified directly this round rather than assumed.
//  3. This page's own real markup ("Įmonės kodas:&nbsp;<i>302983374")
//     has a literal, un-decoded "&nbsp;" between the label's colon and
//     the digits (a tag boundary sits right after it) — `\s*` alone
//     bridges the resulting newline but not the 6-character "&nbsp;"
//     text itself. Added an optional-"&nbsp;" tolerance to the pattern.
//  4. extractEntityAround's prefix-form fallback (extractEntityAfterSuffix)
//     was gated on the ENTIRE text preceding the suffix being blank — but
//     this real page writes "1.2. Pardavėjas &#8211; UAB „...\"", where
//     "Pardavėjas &#8211;" (a role label plus dash) precedes "UAB" and is
//     not blank, so the fallback never even attempted the prefix-form
//     extraction. Widened the gate to also try the fallback when the
//     preceding text ends in a dash-style separator (Unicode dash
//     characters AND their common un-decoded HTML-entity spellings, since
//     this page's own dash is the literal "&#8211;" entity, never
//     decoded by stripTagsLines).
//  5. Once reachable, the prefix-form name itself is quote-wrapped
//     ("„GRUPINIS PIRKIMAS"", literally "&#8222;GRUPINIS
//     PIRKIMAS&#8221;" un-decoded) — cleanCandidateName's entity-
//     corruption filter (which correctly rejects genuinely broken "&#..."
//     residue) was rejecting this well-formed quoted name outright, since
//     the quote delimiters are themselves un-decoded numeric entities.
//     Added a new stripQuoteDelimiters helper that trims a known
//     leading/trailing quote-delimiter pair (both the un-decoded entity
//     forms and their real Unicode equivalents) BEFORE the corruption
//     check ever sees them, rather than loosening that filter itself.
//  6. Even with a winning candidate, formatRegister leaked the same
//     un-decoded "&nbsp;" (bug 3) straight into the displayed Register
//     string ("Įmonės kodas &nbsp; 302983374") — collapseSpaces (added in
//     round 18 for embedded newlines) doesn't touch literal "&nbsp;" text
//     since it isn't whitespace. Added explicit "&nbsp;"-prefix stripping
//     to formatRegister's existing cleanup pass.
//
// One real, honestly-documented gap NOT fixed this round: Address stays
// empty — the real address ("buveinės adresas GIEDRIŲ G. 141, Bučiūnų k.,
// LT-70192, Vilkaviškio r.") sits on the SAME line as the entity name and
// its Įmonės kodas/PVM kodas clause, the same class of same-line gap
// already left undocumented for Romania/Croatia/Slovenia. No checksum
// implemented for Lithuania's PVM kodas/VAT either (stays format_valid).
const grupinisLtFixture = `<!DOCTYPE html>
<html lang="lt">
<head><title>Taisyklės</title></head>
<body>
<h5 class="uppercase">1. PAGRINDINĖS SĄVOKOS</h5>
<p class="uppercase"><br>1.1. Elektroninė parduotuvė &#8211; ši elektroninė parduotuvė, esanti adresu www.GRUPINISPIRKIMAS.lt.<br>1.2. Pardavėjas &#8211; UAB &#8222;GRUPINIS PIRKIMAS&#8221;, buveinės adresas GIEDRIŲ G. 141, <i>Bučiūnų k., LT-70192, Vilkaviškio r.</i> Įmonės kodas:&nbsp;<i>302983374 </i>PVM kodas:&nbsp;<i>LT100007453916</i>.<br>1.3. Paskyra &#8211; Pirkėjo Elektroninės parduotuvės vartotojo paskyra.</p>
</body>
</html>`

func TestExtractImprintFieldsGrupinisLtRealEvidence(t *testing.T) {
	im := extractImprintFields("https://grupinispirkimas.lt/uab-grupinis-pirkimas-taisykles/", grupinisLtFixture, "grupinispirkimas.lt")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "UAB GRUPINIS PIRKIMAS"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (prefix-form: legal form before the quoted trading name)", im.LegalName, wantName)
	}
	if im.Suffix != "UAB" {
		t.Errorf("Suffix = %q, want \"UAB\"", im.Suffix)
	}
	if im.Country != "LT" {
		t.Errorf("Country = %q, want LT", im.Country)
	}
	const wantRegister = "Įmonės kodas 302983374"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q (must NOT contain a literal \"&nbsp;\")", im.Register, wantRegister)
	}
	if im.VAT != "LT100007453916" {
		t.Errorf("VAT = %q, want LT100007453916", im.VAT)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty (known, honestly-documented gap this round — see doc comment above)", im.Address)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Lithuanian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") {
		t.Errorf("FieldsFound = %v, want legal_name present", im.FieldsFound)
	}
}

// TestLtCompanyCodeChecksum is a direct unit test of the new
// ltCompanyCodeValid wiring: both real Įmonės kodas values found during
// this round's search must pass, and a flipped last digit must fail.
func TestLtCompanyCodeChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"grupinispirkimas.lt real Įmonės kodas", "Įmonės kodas: 302983374", checksumValid},
		{"a second real Įmonės kodas found the same day", "Įmonės kodas: 302662379", checksumValid},
		{"flipped last digit must fail", "Įmonės kodas: 302983375", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("Įmonės kodas", tc.raw, "LT"); got != tc.want {
				t.Errorf("validateIdentifier(Įmonės kodas, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("Įmonės kodas"); got != "LT" {
		t.Errorf("singleCountryIdentifierKind(Įmonės kodas) = %q, want LT", got)
	}
}

// TestPrefixFormDashFallback is a direct unit test of the widened
// extractEntityAround fallback gate: a role label ending in a dash (this
// round's real evidence has it as an un-decoded HTML-entity spelling,
// "&#8211;") before a prefix-form suffix must still reach the quoted
// trading name after it, with the quote delimiters stripped.
func TestPrefixFormDashFallback(t *testing.T) {
	text := "Pardavėjas &#8211; UAB &#8222;GRUPINIS PIRKIMAS&#8221;, buveinės"
	got := extractEntityAround(text, "UAB")
	const want = "UAB GRUPINIS PIRKIMAS"
	if got != want {
		t.Errorf("extractEntityAround(%q) = %q, want %q", text, got, want)
	}
}

// TestFormatRegisterStripsNbsp is a direct unit test of the formatRegister
// fix: a literal "&nbsp;" between a register label and its value must not
// leak into the formatted output.
func TestFormatRegisterStripsNbsp(t *testing.T) {
	got := formatRegister("Įmonės kodas", "Įmonės kodas:&nbsp;\n302983374")
	const want = "Įmonės kodas 302983374"
	if got != want {
		t.Errorf("formatRegister = %q, want %q", got, want)
	}
}
