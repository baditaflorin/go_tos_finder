package main

import (
	"strings"
	"testing"
)

// Round-15 EU-expansion real-evidence fixture, fetched 2026-08-22:
// Romania. Real excerpt from https://www.emag.ro/info/termeni-si-conditii
// (eMAG's operating entity, DANTE INTERNATIONAL S.A.) — byte-for-byte from
// the live page's own markup, including its literal un-decoded HTML
// entities (&nbsp;/&copy;) and mixed-case Romanian diacritics-free prose.
// Trimmed to the "1. DEFINITII SI TERMENI" paragraph that names the
// operating entity plus the site's own footer copyright line, which is
// where the page's real "CUI: 14399840" register identifier lives (the
// main paragraph's own "cod unic de inregistrare RO 14399840" phrase has a
// space between "RO" and the digits, so it does NOT independently match
// vatPatterns' VAT-country-prefix regex — the footer's differently-worded
// "CUI:" line is the only place on this real page where the identifier
// gets picked up at all).
//
// Romania already had a native ALL-CAPS "S.R.L." suffix entry and a "CUI"
// vatPattern/checksum (imprint_suffix.go / imprint_vat.go /
// imprint_checksum.go's roCUIValid) wired up since this codebase's first
// EU-expansion round, but had never been real-evidence-verified end to end
// against an actual Romanian business page. Running the shipped 1.7.9
// extractor against this real page found TWO concrete, compounding bugs:
//
//  1. "DANTE INTERNATIONAL S.A." only carries a bare "S.A." legal-form
//     suffix on this real page (no "(România)" qualifier) — imprint_suffix.go's
//     low-confidence generic "S.A." entry maps to FR (France), so the
//     winning candidate's Country came out "FR", wrongly, even though the
//     SAME page's own "CUI: 14399840" identifier — matched, Register
//     correctly populated as "CUI 14399840" — is unambiguously Romanian.
//     The reason the ground-truth identifier never corrected the
//     suffix-guessed country: singleCountryIdentifierKind
//     (imprint_checksum.go), which lets a label-anchored identifier Kind
//     override a merely-probabilistic suffix country (the same mechanism
//     that already fixes the IT S.R.L./RO S.R.L. collision and, since
//     round 14, Norway's OrgNr), had never had a case for Kind="CUI" at
//     all — wired into vatPatterns and the checksum switch since round 1,
//     but never connected to the country-correction path. Added a "CUI"
//     case returning "RO".
//  2. Once (1) exposed the footer copyright line's real content, a second,
//     independent bug surfaced: extractAddressNearEntity's forward-scan
//     stop-marker list (the same list that already excludes NIP/KRS/REGON,
//     CVR, KvK/BTW-id, RCS, TVA lines from being absorbed into Address —
//     see that function's doc comments) had no entry for Romania's own
//     "CUI"/"CIF" label. The footer line "© 2001-2026 Dante International,
//     CUI: 14399840, Reg. Com. J2002000372404" sits a few lines below the
//     entity paragraph, its "14399840"/"J2002000372404" digit runs clear
//     looksAddressLine, and — before this fix — the whole unrelated
//     copyright/register line got appended onto im.Address via a stray
//     ", &copy; 2001-2026 Dante International, CUI: ..." tail. Added a
//     word-boundary cuiWordRE check (imprint_name.go) to the stop-marker
//     list; a bare substring check was deliberately avoided since "cui" is
//     also an ordinary standalone Romanian word ("to whom"/"nail").
//
// NOT fixed this round (honestly documented, not overclaimed): the
// remaining Address value below still absorbs the REST of the entity's own
// run-on sentence (from right after the entity name through the register
// clause) because this real page's markup has no punctuation/tag boundary
// between the street address and the following register text on that same
// line — a same-line entity-boundary issue, different code path from the
// forward-scan fix in (2) above, and out of scope for this round's
// minimal, targeted fix.
const emagRoFixture = `<!DOCTYPE html>
<html lang="ro">
<head><title>Termeni si conditii - eMAG.ro</title></head>
<body>
<p class="wp-block-paragraph"><strong>1. DEFINITII SI TERMENI</strong></p>
<p class="wp-block-paragraph"><strong>eMAG&nbsp;</strong>– este denumirea comerciala a societatii&nbsp;<strong>DANTE INTERNATIONAL S.A.,</strong>&nbsp;persoana juridica romana, avand sediul social situat in Bucuresti, Soseaua Virtutii nr. 148, Spatiul E47, sector 6, avand numar de ordine in Registrul Comertului J2002000372404, cod unic de inregistrare&nbsp; RO 14399840.</p>
<p class="wp-block-paragraph"><strong>Vanzator&nbsp;</strong>– eMAG sau orice partener din eMAG Marketplace.</p>
<div class="footer-bottom">
<p>&copy; 2001-2026 Dante International, CUI: 14399840, Reg. Com. J2002000372404</p>
</div>
</body>
</html>`

func TestExtractImprintFieldsEmagRoRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.emag.ro/info/termeni-si-conditii", emagRoFixture, "emag.ro")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "DANTE INTERNATIONAL S.A."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "S.A." {
		t.Errorf("Suffix = %q, want \"S.A.\"", im.Suffix)
	}
	if im.Country != "RO" {
		t.Errorf("Country = %q, want RO (must NOT be FR — the France/Romania \"S.A.\" suffix collision this round fixed via singleCountryIdentifierKind's new CUI case)", im.Country)
	}
	const wantRegister = "CUI 14399840"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if got := strings.Contains(im.Address, "&copy;"); got {
		t.Errorf("Address = %q, must NOT contain the unrelated footer copyright line (the cuiWordRE stop-marker fix this round)", im.Address)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Romanian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") {
		t.Errorf("FieldsFound = %v, want legal_name and address present", im.FieldsFound)
	}
}

// TestCUIIdentifierChecksum is a direct unit test of the roCUIValid wiring
// (pre-existing since round 1, confirmed here against a REAL CUI for the
// first time) plus this round's new singleCountryIdentifierKind case.
func TestCUIIdentifierChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"emag.ro real CUI (labelled, no RO prefix)", "CUI: 14399840", checksumValid},
		{"emag.ro real CUI (CIF label synonym)", "CIF 14399840", checksumValid},
		{"flipped last digit must fail", "CUI: 14399841", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("CUI", tc.raw, "RO"); got != tc.want {
				t.Errorf("validateIdentifier(CUI, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("CUI"); got != "RO" {
		t.Errorf("singleCountryIdentifierKind(CUI) = %q, want RO", got)
	}
}

// TestCUIStopsAddressAbsorption is a direct unit test of the
// extractAddressNearEntity stop-marker fix: a "CUI"/"CIF" labelled line
// following the entity's own line must NOT be absorbed into the address,
// same as the pre-existing NIP/KRS/REGON/CVR/KvK/RCS/TVA markers.
func TestCUIStopsAddressAbsorption(t *testing.T) {
	text := "Contact SRL\nStrada Exemplu 12, 010101 Bucuresti\nCUI: 12345678, Reg. Com. J40/123/2020"
	got := extractAddressNearEntity(text, "Contact SRL")
	if strings.Contains(got, "CUI") || strings.Contains(got, "Reg. Com") {
		t.Errorf("extractAddressNearEntity = %q, must not absorb the CUI/register line", got)
	}
	const want = "Strada Exemplu 12, 010101 Bucuresti"
	if got != want {
		t.Errorf("extractAddressNearEntity = %q, want %q", got, want)
	}
}
