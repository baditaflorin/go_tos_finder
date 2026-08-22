package main

import "testing"

// Round-27 EU-expansion real-evidence fixture, fetched 2026-08-23: Malta —
// the LAST of the 13 target EU countries for this multi-round expansion.
// Real excerpt from https://artemisialtd.com/pages/terms-of-sale
// (Artemisia Fine Arts & Antiques Limited, a real, live Maltese art
// gallery/e-commerce site's Terms of Sale page) — byte-for-byte from the
// live page's own markup.
//
// Running the shipped 1.7.21 extractor against this real page found
// Present=true but everything else empty (CompletenessScore=0). Found and
// fixed THREE compounding real bugs:
//
//  1. Malta's own VAT format uses an optional dash between the two 4-digit
//     halves ("MT2336-6423") — the existing bare-adjacent `\bMT\d{8}\b`
//     pattern never matched it. Loosened to `\bMT\d{4}-?\d{4}\b`.
//  2. Malta's "Company Registration Number" (Malta Business Registry's own
//     "C"+4-6-digits convention, e.g. "C 71943") had no identifier pattern
//     at all. Added, anchored on the English label text (not the bare
//     "C NNNNN" shape alone, which would be dangerously generic to match
//     unlabelled anywhere in text) — wired Register-only (same "format-
//     match, no invented checksum" discipline as Ireland's CRO/Croatia's
//     MBS) and connected to singleCountryIdentifierKind so it self-heals
//     Country away from the suffix-guessed "GB" (a bare "Limited" suffix
//     collides with suffixTable's existing GB entry) to the correct "MT".
//  3. The real page states the whole "<Name> (<company number>) of
//     <address> (\"<nickname>\") is a ..." clause on ONE line (UK/Malta-
//     style company-law drafting) — extractAddressNearEntity's forward
//     per-line scan only ever looks at lines AFTER the one naming the
//     entity, so it never saw an address on the SAME line. Added a
//     same-line inline extraction (inlineOfAddressRE) anchored to the
//     "of " token immediately following the name/company-number clause,
//     capturing up to the next "(" (which always opens the nickname
//     clause in this drafting style).
//
// Also fixed formatRegister to strip a leading "is "/"Is " token, since
// this Kind's pattern deliberately keeps the optional "is" inside its RAW
// match (findIdentifiers requires the raw match for proximity-attachment;
// see formatRegister's own doc comment for why cleanup happens only at
// display time).
//
// One honestly-documented non-fix this round: no VAT checksum algorithm
// was independently verified against a real fetched value, so
// VATValidation stays "format_valid" (not "checksum_valid") — same
// discipline as Ireland/Croatia/Slovakia/Cyprus/Luxembourg/Latvia before
// it. Also NOT matched: the page's OTHER, bare-parenthetical restatement
// of the same company number, "(C 71943)" directly after the entity name
// with no label at all — deliberately excluded as too generic to match
// unlabelled (see singleCountryIdentifierKind's "Company Registration
// Number" case for the collision this override exists to prevent).
const artemisiaMtFixture = "<!DOCTYPE html>\n<html lang=\"en\">\n<head><title>Terms of Sale</title></head>\n<body>\n<h1>Terms of Sale</h1>\n<h2>1. INTRODUCTION</h2>\n<p><strong>1.1</strong> Artemisia Fine Arts &amp; Antiques Limited (C 71943) of ‘Ridge View’, Triq is-Sagra Familja, Bidnija, Mosta MST 5012, Malta (“Artemisia”; “we”; “us”; or “our”) is a specialist vendor for fine arts and antiques, and is the owner and operator of the physical art gallery Artemisia Fine Arts &amp; Antiques Ltd situated at Ridgeview, Triq is-Sagra Familja, Bidnija, MST 5012, Malta.</p>\n<h2>3. OUR DETAILS</h2>\n<p><strong>3.1</strong> Our company registration number is C 71943 and our registered office is at ‘Ridge View’, Triq is-Sagra Familja, Bidnija, Mosta MST 5012, Malta. Our registered VAT number is MT2336-6423.</p>\n</body>\n</html>"

func TestExtractImprintFieldsArtemisiaMtRealEvidence(t *testing.T) {
	im := extractImprintFields("https://artemisialtd.com/pages/terms-of-sale", artemisiaMtFixture, "artemisialtd.com")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Artemisia Fine Arts & Antiques Limited"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "Limited" {
		t.Errorf("Suffix = %q, want \"Limited\"", im.Suffix)
	}
	if im.Country != "MT" {
		t.Errorf("Country = %q, want MT (must self-heal away from the suffix-guessed GB via singleCountryIdentifierKind)", im.Country)
	}
	const wantAddress = "‘Ridge View’, Triq is-Sagra Familja, Bidnija, Mosta MST 5012, Malta"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q", im.Address, wantAddress)
	}
	const wantRegister = "Company Registration Number C 71943"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.VAT != "MT23366423" {
		t.Errorf("VAT = %q, want MT23366423 (dash stripped by cleanIdentifierValue)", im.VAT)
	}
	if im.VATValidation != string(formatValid) {
		t.Errorf("VATValidation = %q, want format_valid (no MT checksum algorithm implemented)", im.VATValidation)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Maltese ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") {
		t.Errorf("FieldsFound = %v, want legal_name and address present", im.FieldsFound)
	}
	if !containsStr(im.FieldsMissing, "vat_valid") {
		t.Errorf("FieldsMissing = %v, want vat_valid present (no MT checksum algorithm implemented)", im.FieldsMissing)
	}
}

// TestMtCompanyRegistrationNumberIsRegisterOnly is a direct unit test
// confirming Malta's "Company Registration Number" is wired Register-only
// (not VAT-equivalent — the two are genuinely separate numbers on the real
// page: "C 71943" vs "MT2336-6423") and resolves to MT via
// singleCountryIdentifierKind, correcting away from a suffix-guessed GB.
func TestMtCompanyRegistrationNumberIsRegisterOnly(t *testing.T) {
	if isVATLikeIdentifierKind("Company Registration Number") {
		t.Error("isVATLikeIdentifierKind(Company Registration Number) = true, want false (it's a genuinely separate number from Malta's own VAT)")
	}
	if got := singleCountryIdentifierKind("Company Registration Number"); got != "MT" {
		t.Errorf("singleCountryIdentifierKind(Company Registration Number) = %q, want MT", got)
	}
	ids := findIdentifiers("Our company registration number is C 71943", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "Company Registration Number" && id.Country == "MT" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match the labelled Company Registration Number form — got %v", ids)
	}
}

// TestMtVatToleratesDash is a direct unit test of the MT VAT pattern's
// dash-tolerance fix — real evidence: artemisialtd.com's real Terms of
// Sale page writes its own VAT number grouped as "MT2336-6423", not the
// bare-adjacent 8-digit form the pattern originally required.
func TestMtVatToleratesDash(t *testing.T) {
	ids := findIdentifiers("Our registered VAT number is MT2336-6423.", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "VAT" && id.Country == "MT" && id.Value == "MT2336-6423" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match the dash-grouped MT VAT form — got %v", ids)
	}
}

// TestMtInlineOfAddressSameLine is a direct unit test of the
// extractAddressNearEntity same-line fix: a UK/Malta-style
// "<Name> (<number>) of <address> (\"<nickname>\")" clause sitting on ONE
// line must still yield the address, even though the forward per-line
// scan never looks at the line containing the entity name itself.
func TestMtInlineOfAddressSameLine(t *testing.T) {
	text := "Acme Trading Limited (C 12345) of 27 Republic Street, Valletta VLT 1112, Malta (“Acme”) is a specialist vendor."
	got := extractAddressNearEntity(text, "Acme Trading Limited")
	const want = "27 Republic Street, Valletta VLT 1112, Malta"
	if got != want {
		t.Errorf("extractAddressNearEntity = %q, want %q", got, want)
	}
}
