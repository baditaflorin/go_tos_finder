package main

import (
	"strings"
	"testing"
)

// Round-17 EU-expansion real-evidence fixture, fetched 2026-08-22: Hungary.
// Real excerpt from https://www.szatmari-izek.shop.hu/impresszum/ (Szatmári
// Ízek Kft. — a real, live Hungarian specialty-foods e-shop's Impresszum
// page) — byte-for-byte from the live page's own markup, including its
// literal un-decoded HTML entity (&#8211;, an en-dash) and real Hungarian
// diacritics.
//
// Running the shipped 1.7.11 extractor against this real page extracted
// almost nothing (only "contact", CompletenessScore 33), despite "Szatmári
// Ízek Kft." being trivially suffix-matchable ("Kft.", an existing native
// HU suffixTable entry) right next to its register number and tax number.
// Found and fixed four compounding real bugs:
//
//  1. extractImprintText's isLegalPage keyword gate checks for the English
//     "impressum" (and "imprint"/"legal"/"colophon"/"colofon"/"terms"/
//     "company"/"about") but NOT the Hungarian spelling "impresszum" —
//     which is NOT a substring of "impressum" (the Hungarian doubles the
//     "sz" where German doubles the "s": "...re-ssz-um" vs "...re-ss-um").
//     Same failure shape as round 14's Dutch "colofon" gap. Combined with
//     bug 3 below (no domestic identifier pattern to trigger the
//     "useful only if VAT/register IDs present" fallback either), this
//     page's own real "/impresszum/" URL bailed the WHOLE suffix-anchored
//     scan out before it ever ran. Added "impresszum" to the keyword list.
//  2. Once bug 1+3 were fixed and the suffix scan started finding real
//     text, the entity line itself ("Név: Szatmári Ízek Kft.") still
//     carried its "Név: " (Hungarian "Name:") label prefix into
//     LegalName — no Hungarian entry existed in stripLabelPrefix's list
//     (which already has a Norwegian "Brev: " analogue from round 14 but
//     nothing Hungarian). Added "Név: ".
//  3. Hungary's Adószám (domestic tax number, VAT-equivalent — see below)
//     and Cégjegyzékszám (company-register number) had no vatPatterns
//     entries at all, so findIdentifiers found nothing on this page.
//     Added dedicated patterns for both, wired "Cégjegyzékszám" into the
//     Register-field Kind switch (same shape as Czechia's IČO, round 16)
//     and "Adószám" into the VAT-equivalent Kind switch (same shape as
//     Poland's NIP — Adószám's 8-digit core IS the same number the
//     "HU"-prefixed EU VAT pattern matches), plus a new huAdoszamValid
//     weighted-mod-10 checksum, confirmed against TWO independent real
//     values found on this same page (13495413 and 23495919 — see
//     huAdoszamValid's doc comment).
//  4. Fixing bug 3 exposed a fourth bug: the page's real
//     "Cégjegyzékszám: ...<br />Adószám: ...<br />Székhely: ..." sequence
//     has the real address (Székhely) AFTER both identifier lines —
//     extractAddressNearEntity had no stop-and-skip marker for either
//     Hungarian label, so both got wrongly absorbed into Address ahead of
//     the real address (same failure shape as round 5's Polish NIP/KRS/
//     REGON). Added both as "continue" (skip-past, not break) markers,
//     matching the NIP/KRS/REGON precedent rather than the RCS/TVA
//     break-markers, since here too the real address comes AFTER the
//     identifier lines, not before.
//
// Both "Cégjegyzékszám" and "Adószám" were also added to
// singleCountryIdentifierKind (-> "HU"): both are Hungarian-language terms
// with no other country's business-register system using either label —
// unlike round 16's Czech "IČO", which is deliberately NOT single-country
// (shared with Slovakia).
const szatmariHuFixture = `<!DOCTYPE html>
<html lang="hu">
<head><title>Impresszum</title></head>
<body>
<div class="module module-text">
<div class="tb_text_wrap">
<p><strong>Szatmári Ízek Korlátolt Felelősségű Társaság</strong></p>
<p>Név: Szatmári Ízek Kft.<br />Cégjegyzékszám: 15 09 069897 &#8211; Nyíregyházi Törvényszék Cégbírósága<br />Adószám: 13495413-2-15<br />Székhely: Magyarország, 4765 Csenger, Ady Endre u. 133.<br />E-mail cím: info.szatizek@gmail.com<br />Webhely: <a href="http://www.szatmari-izek.hu/">szatmari-izek.hu</a></p>
</div>
</div>
</body>
</html>`

func TestExtractImprintFieldsSzatmariHuRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.szatmari-izek.shop.hu/impresszum/", szatmariHuFixture, "szatmari-izek.shop.hu")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Szatmári Ízek Kft."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (must NOT include the \"Név: \" label prefix)", im.LegalName, wantName)
	}
	if im.Suffix != "Kft." {
		t.Errorf("Suffix = %q, want \"Kft.\"", im.Suffix)
	}
	if im.Country != "HU" {
		t.Errorf("Country = %q, want HU", im.Country)
	}
	const wantRegister = "Cégjegyzékszám 15 09 069897"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	const wantAddress = "Székhely: Magyarország, 4765 Csenger, Ady Endre u. 133."
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (must NOT include the Cégjegyzékszám/Adószám lines)", im.Address, wantAddress)
	}
	if strings.Contains(im.Address, "Cégjegyzékszám") || strings.Contains(im.Address, "Adószám") {
		t.Errorf("Address = %q, must not absorb the register/tax-number lines", im.Address)
	}
	if im.VAT == "" {
		t.Error("expected a non-empty VAT (Adószám, treated as VAT-equivalent)")
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (real Adószám 13495413-2-15)", im.VATValidation)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Hungarian ruleset added this round)", im.Ruleset)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name, address, contact, and vat_valid all present", im.FieldsFound)
	}
}

// TestAdoszamIdentifierChecksum is a direct unit test of the new
// huAdoszamValid wiring: both real Adószám values found on
// szatmari-izek.shop.hu's real page must pass, and a flipped last digit
// must fail.
func TestAdoszamIdentifierChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"szatmari-izek.shop.hu real Adószám (entity's own)", "Adószám: 13495413-2-15", checksumValid},
		{"a second real Adószám on the same page", "Adószám: 23495919-2-41", checksumValid},
		{"flipped last digit of the core must fail", "Adószám: 13495414-2-15", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("Adószám", tc.raw, "HU"); got != tc.want {
				t.Errorf("validateIdentifier(Adószám, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("Cégjegyzékszám"); got != "HU" {
		t.Errorf("singleCountryIdentifierKind(Cégjegyzékszám) = %q, want HU", got)
	}
	if got := singleCountryIdentifierKind("Adószám"); got != "HU" {
		t.Errorf("singleCountryIdentifierKind(Adószám) = %q, want HU", got)
	}
}

// TestHungarianRegisterMarkersSkipNotBreak is a direct unit test of the
// extractAddressNearEntity fix: "cégjegyzékszám"/"adószám" lines must be
// skipped (continue), not treated as a hard stop (break) — the real address
// that follows them on szatmari-izek.shop.hu's real page must still be
// found.
func TestHungarianRegisterMarkersSkipNotBreak(t *testing.T) {
	text := "Szatmári Ízek Kft.\nCégjegyzékszám: 15 09 069897\nAdószám: 13495413-2-15\nSzékhely: Magyarország, 4765 Csenger, Ady Endre u. 133."
	got := extractAddressNearEntity(text, "Szatmári Ízek Kft.")
	const want = "Székhely: Magyarország, 4765 Csenger, Ady Endre u. 133."
	if got != want {
		t.Errorf("extractAddressNearEntity = %q, want %q", got, want)
	}
}
