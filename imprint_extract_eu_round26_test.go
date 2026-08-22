package main

import "testing"

// Round-26 EU-expansion real-evidence fixture, fetched 2026-08-23: Estonia.
// Real excerpt from https://voluaed.ee/muugitingimused/ (OÜ Iru Aiakeskus —
// a real, live Estonian garden-supplies e-shop's Müügitingimused page) —
// byte-for-byte from the live page's own markup, including its literal raw
// U+00A0 (NBSP) characters (a real, non-entity Unicode character in the
// source, unlike most other rounds' &nbsp; HTML-entity form).
//
// Running the shipped 1.7.20 extractor against this real page found the
// entity name, suffix, country, and VAT already correctly — "OÜ" is an
// existing high-confidence suffixTable entry, and the existing bare
// "EE"-prefixed EU VAT pattern already matches "KMKR nr: EE101490959"
// without needing a label anchor. Found and fixed two compounding real
// bugs:
//
//  1. Estonia's "registrikood" (company registry code) had no identifier
//     pattern at all. Added it, wired as Register-only — a genuinely
//     SEPARATE 8-digit number from Estonia's own 9-digit KMKR nr (VAT
//     number), same architecture as Lithuania's Įmonės kodas/Latvia's
//     split (not Bulgaria/Greece/Croatia's shared-body architecture). No
//     dedicated checksum implemented this round.
//  2. Once found, the real page's "KMKR nr: EE101490959" line (which sits
//     right after the entity's own line) still leaked into Address — the
//     same failure shape every prior round's identifier labels hit.
//     Added "kmkr"/"registrikood" as skip-past markers.
//
// One real, honestly-documented gap NOT fixed this round: Address stays
// empty — the real address ("asukohaga Miku tee 11, Tallinn") sits on the
// SAME line as the entity name and its own registrikood parenthetical, the
// same class of same-line gap already left undocumented for Romania/
// Croatia/Slovenia/Lithuania. Also honestly note-worthy (not a bug):
// LegalName includes the source's own parenthetical register annotation
// verbatim ("OÜ Iru Aiakeskus (registrikood 12178134)") — a faithful quote
// of how the real page names its own entity, left as-is rather than
// stripped, since the register NUMBER is already separately and correctly
// captured in Register regardless.
const voluaedEeFixture = "<!DOCTYPE html>\n<html lang=\"et\">\n<head><title>Müügitingimused</title></head>\n<body>\n<p><strong>VÕLUAED E-POE MÜÜGITINGIMUSED</strong></p>\n<p>&nbsp;</p>\n<p>Veebipoe  <strong>voluaed.ee/e-pood</strong>  (edaspidi Veebipood) omanik on:<br />\nOÜ Iru Aiakeskus (registrikood 12178134), asukohaga Miku tee 11, Tallinn.<br />\nKMKR nr: EE101490959<br />\nE-posti aadress: pood@voluaed.ee<br />\nTelefon: 6011430</p>\n</body>\n</html>"

func TestExtractImprintFieldsVoluaedEeRealEvidence(t *testing.T) {
	im := extractImprintFields("https://voluaed.ee/muugitingimused/", voluaedEeFixture, "voluaed.ee")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "OÜ Iru Aiakeskus (registrikood 12178134)"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "OÜ" {
		t.Errorf("Suffix = %q, want \"OÜ\"", im.Suffix)
	}
	if im.Country != "EE" {
		t.Errorf("Country = %q, want EE", im.Country)
	}
	const wantRegister = "registrikood 12178134"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.VAT != "EE101490959" {
		t.Errorf("VAT = %q, want EE101490959", im.VAT)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty and must NOT contain the KMKR nr line (known, honestly-documented gap this round — see doc comment above)", im.Address)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Estonian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want legal_name and contact present", im.FieldsFound)
	}
}

// TestEeRegistrikoodIsRegisterOnly is a direct unit test confirming
// "registrikood" is wired as Register-only (not VAT-equivalent, unlike
// Bulgaria's ЕИК/Greece's ΑΦΜ/Croatia's OIB) and resolves to EE via
// singleCountryIdentifierKind.
func TestEeRegistrikoodIsRegisterOnly(t *testing.T) {
	if isVATLikeIdentifierKind("registrikood") {
		t.Error("isVATLikeIdentifierKind(registrikood) = true, want false (it's a genuinely separate number from Estonia's own VAT)")
	}
	if got := singleCountryIdentifierKind("registrikood"); got != "EE" {
		t.Errorf("singleCountryIdentifierKind(registrikood) = %q, want EE", got)
	}
	ids := findIdentifiers("registrikood 12178134", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "registrikood" && id.Country == "EE" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match \"registrikood 12178134\" — got %v", ids)
	}
}

// TestEeKmkrMarkerSkipsNotBreaks is a direct unit test of the
// extractAddressNearEntity fix: a "KMKR nr" line must be skipped
// (continue), not absorbed into the address.
func TestEeKmkrMarkerSkipsNotBreaks(t *testing.T) {
	text := "OÜ Iru Aiakeskus\nKMKR nr: EE101490959\nE-posti aadress: pood@voluaed.ee"
	got := extractAddressNearEntity(text, "OÜ Iru Aiakeskus")
	if got != "" {
		t.Errorf("extractAddressNearEntity = %q, want empty (must not absorb the KMKR nr line)", got)
	}
}
