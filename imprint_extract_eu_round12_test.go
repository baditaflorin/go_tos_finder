package main

import "testing"

// Round-12 EU-expansion real-evidence fixture, fetched 2026-08-22.

// kimsDkFixture: real excerpt from kims.dk/handelsbetingelser/ (KiMs
// Webshop's real, live handelsbetingelser / terms-of-sale page — Denmark's
// closest analogue to an Impressum page, naming the operating legal entity
// behind the shop, Orkla Snacks Danmark A/S). Byte-for-byte from the live
// page's own markup (including its literal <br /> line breaks and <em>
// wrapper around the address block).
const kimsDkFixture = `<!DOCTYPE html>
<html lang="da-DK">
<head><title>Handelsbetingelser | KiMs Webshop</title></head>
<body>
<div class="container"><h3>Salgs- og brugervilkår for handel på kims.dk</h3>
<h2>1. Generelt</h2><p>Salg fra <a href="https://kims.dk/">www.kims.dk</a> sker af Orkla Snacks Danmark A/S (CVR.nr. 15233877), et selskab i Orkla-koncernen og som ejer og driver sitet.</p><p>Du skal være over 18 år for at handle hos os eller have en forældre/værge tilladelse til at handle hos <strong>os.</strong></p><p>Orkla Snacks Danmark A/S<br />
(<em>CVR.nr. 15233877</em>)<em><br />
Sømarksvej 31<br />
5471 Søndersø</em></p><p><em><br />
</em><strong>Kundeservice</strong></p>
</div>
</body>
</html>`

// TestExtractImprintFieldsKimsDkRealEvidence: real evidence for a Danish
// A/S (Lov om tjenester i informationssamfundet, herunder visse aspekter af
// elektronisk handel — Denmark's e-Commerce Directive transposition, § 7 —
// requires name, address, and CVR-nr "hvor det er relevant"/where
// applicable, the same conditionality as Sweden's 8 § round-11 fixture).
// Running the shipped 1.7.6 extractor against this real page extracted
// NOTHING at all: CompletenessScore 0, despite "Orkla Snacks Danmark A/S"
// being trivially suffix-matchable ("A/S") right next to its CVR number.
// Found and fixed three compounding real bugs, all surfaced by this one
// real page:
//
//  1. Denmark's CVR-nr (Det Centrale Virksomhedsregister, the Central
//     Business Register number) had no identifier pattern at all — the
//     same class of gap as round 5's Polish NIP/KRS/REGON and round 9's
//     Swedish Organisationsnummer. Without it, findIdentifiers found
//     nothing on this non-"imprint"/"legal"/"terms"-URLed page
//     ("handelsbetingelser" doesn't match any of extractImprintText's
//     isLegalPage URL-vocabulary gate), so the whole suffix-anchored scan
//     bailed out before it ever ran. Added a dedicated "CVR" vatPattern
//     (imprint_vat.go), wired into the Register field (not VAT — same
//     precedent as Sweden's Organisationsnummer) and validated via the
//     existing "DK" weighted-mod-11 checksum (dkVATValid) since a
//     DK-prefixed VAT number IS literally "DK" + the CVR body with no
//     extra check digit — confirmed against both this page's real CVR
//     (15233877) and webshop.dn.dk's real CVR (60804214, an independently
//     found real Danish nonprofit association's page), both of which pass.
//  2. Once the identifier gate passed, the real street line ("Sømarksvej
//     31") cleared no digit-run threshold at all (only a 2-digit house
//     number) and "vej" (Danish for "road"/"way", the single most common
//     Danish street-name ending) had no address-vocabulary marker — same
//     failure shape as round 1's French "rue" and round 5's Polish
//     "aleja". Added "vej" as a marker (imprint_name.go's
//     looksAddressLine).
//  3. Fixing bug 1 exposed a THIRD, pre-existing bug: the address scan
//     starts from the FIRST line naming the winning candidate, which on
//     this real page is an earlier sentence ("...sker af Orkla Snacks
//     Danmark A/S (CVR.nr. 15233877), et selskab..."), followed by an
//     unrelated age-disclaimer sentence containing the ordinary Danish
//     verb "have" ("...eller have en forældre/værge tilladelse...") — the
//     looksAddressLine marker list's bare "ave" (meant for the US "Ave."
//     street abbreviation) substring-matched INSIDE "have", wrongly
//     absorbing that whole unrelated sentence into the address. Re-anchored
//     "ave" to its own word-boundary regexp (aveWordRE) rather than the
//     plain-substring marker loop. Also added "cvr" to
//     extractAddressNearEntity's identifier skip-list (alongside the
//     existing Dutch KvK/BTW-id precedent) since this page's own
//     "CVR.nr. 15233877" line — an 8-digit run — otherwise cleared the
//     digit-run heuristic and got absorbed into the address too.
//
// No dedicated Danish ruleset added: § 7's CVR-nr requirement is
// conditional ("hvor det er relevant"), the same conditionality as
// Sweden's 8 § — staying on eu_baseline rather than overclaiming, same as
// round 11's Swedish precedent.
func TestExtractImprintFieldsKimsDkRealEvidence(t *testing.T) {
	im := extractImprintFields("https://kims.dk/handelsbetingelser/", kimsDkFixture, "kims.dk")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Orkla Snacks Danmark A/S"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "A/S" {
		t.Errorf("Suffix = %q, want \"A/S\"", im.Suffix)
	}
	if im.Country != "DK" {
		t.Errorf("Country = %q, want DK", im.Country)
	}
	const wantAddress = "Sømarksvej 31, 5471 Søndersø"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (must NOT include the unrelated \"have\"-sentence or the CVR line)", im.Address, wantAddress)
	}
	const wantRegister = "CVR nr. 15233877"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (Denmark's § 7 CVR-nr requirement is conditional \"hvor det er relevant\" — no dedicated Danish ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") {
		t.Errorf("FieldsFound = %v, want legal_name and address present", im.FieldsFound)
	}
}

// TestCVRIdentifierChecksum is a direct unit test of the new CVR
// identifier: both real CVR numbers found during this round's search must
// pass the DK weighted-mod-11 checksum, and a value with a flipped digit
// must fail it.
func TestCVRIdentifierChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"kims.dk real CVR", "CVR.nr. 15233877", checksumValid},
		{"webshop.dn.dk real CVR", "CVR-nr.: 60804214", checksumValid},
		{"flipped digit must fail", "CVR.nr. 15233878", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("CVR", tc.raw, "DK"); got != tc.want {
				t.Errorf("validateIdentifier(CVR, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("CVR"); got != "DK" {
		t.Errorf("singleCountryIdentifierKind(CVR) = %q, want DK", got)
	}
}

// TestLooksAddressLineAveIsWordBoundary is a direct unit test of the
// aveWordRE fix: the US "Ave."/"Ave" street abbreviation must still match,
// but the ordinary word "have" (and its neighbours) must not.
func TestLooksAddressLineAveIsWordBoundary(t *testing.T) {
	if !looksAddressLine("Located on Main Ave") {
		t.Error(`looksAddressLine("Located on Main Ave") = false, want true — no digit run and no other marker in this line, so this isolates the "ave" word-boundary path`)
	}
	if !looksAddressLine("Located on Main Ave.") {
		t.Error(`looksAddressLine("Located on Main Ave.") = false, want true (trailing period)`)
	}
	if looksAddressLine("eller have en forældre/værge tilladelse") {
		t.Error(`looksAddressLine("eller have en forældre/værge tilladelse") = true, want false — "have" must not match the "ave" marker`)
	}
}
