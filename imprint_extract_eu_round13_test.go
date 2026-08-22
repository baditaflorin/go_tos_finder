package main

import "testing"

// Round-13 EU-expansion real-evidence fixture, fetched 2026-08-22.

// finnprotecFiFixture: real excerpt from
// finnprotec.fi/verkkokauppa/toimitus-ja-sopimusehdot/ (Finnprotec Oy's
// real, live delivery-and-contract-terms page — Finland's closest analogue
// to an Impressum page, naming the operating legal entity and its
// Y-tunnus). Byte-for-byte from the live page's own markup.
const finnprotecFiFixture = `<!DOCTYPE html>
<html lang="fi" prefix="og: https://ogp.me/ns#" class="no-js">
<head><title>Toimitus- ja sopimusehdot - FinnProtec</title></head>
<body>
<h2><strong>FINNPROTEC OY – TOIMITUS- JA SOPIMUSEHDOT</strong></h2>
<ol>
<li><strong> Yleiset ehdot</strong></li>
</ol>
<p>Finnprotec Oy (Y-tunnus: 1938183-5) ylläpitää verkkokauppaa osoitteessa Finnprotec.fi/webshop. Näitä sopimusehtoja sovelletaan Finnprotec Oy:n ja sen asiakkaiden välisiin tilauksiin ja toimituksiin.</p>
<p>Finnprotec Oy pidättää oikeuden muuttaa ehtoja ilman ennakkoilmoitusta. Tilaukseen sovelletaan sen tekohetkellä voimassa olevia ehtoja. Lakimuutokset astuvat voimaan sellaisenaan.</p>
<ol start="9">
<li><strong> Reklamaatiot</strong></li>
</ol>
<p>Reklamaatio tulee toimittaa kirjallisesti osoitteeseen sales@finnprotec.fi tai postitse Espoon toimipisteeseen. Ostokuitti tai vastaava tosite on esitettävä.</p>
</body>
</html>`

// TestExtractImprintFieldsFinnprotecFiRealEvidence: real evidence for a
// Finnish Oy (Laki tietoyhteiskunnan palvelujen tarjoamisesta — Finland's
// e-Commerce Directive transposition — requires name, address, and contact
// details "tarvittaessa"/where relevant, the same conditionality as
// round 11's Swedish 8 § and round 12's Danish § 7 precedents). Running
// the shipped 1.7.7 extractor against this real page failed to extract
// legal_name or register at all (CompletenessScore 33, contact-only),
// despite "Finnprotec Oy" being trivially suffix-matchable ("Oy") right
// next to its Y-tunnus. Root cause: Finland's Y-tunnus (Yritys- ja
// yhteisötunnus, the Finnish Business ID) had no domestic label-anchored
// identifier pattern at all — the existing VAT/FI pattern only matches the
// "FI"-prefixed, hyphen-free cross-border form (FI12345678), a different
// shape from the domestic hyphenated form ("1938183-5") this real page
// actually uses. Same class of gap as round 12's Danish CVR-nr and round 5's
// Polish NIP/KRS/REGON: without any identifier match, findIdentifiers found
// nothing on this non-"imprint"/"legal"/"terms"/"company"/"about"-URLed
// page ("toimitus-ja-sopimusehdot" matches none of extractImprintText's
// isLegalPage URL-vocabulary gate), so the whole suffix-anchored scan
// bailed out before it ever ran. Added a dedicated "Y-tunnus" vatPattern
// (imprint_vat.go), wired into the Register field (not VAT — same
// precedent as Sweden's Organisationsnummer/Denmark's CVR-nr) and
// validated via the existing "FI" weighted-mod-11 checksum (fiVATValid)
// since an FI-prefixed VAT number IS literally "FI" + the Y-tunnus body
// with its hyphen removed — confirmed against this page's real Y-tunnus
// (1938183-5), which passes.
//
// No dedicated Finnish ruleset added: this real page has no physical
// street address on it at all (a delivery/contract-terms page, not a full
// imprint page) — CompletenessScore reflects that honestly rather than
// overclaiming, same discipline as round 11's Swedish fixture.
func TestExtractImprintFieldsFinnprotecFiRealEvidence(t *testing.T) {
	im := extractImprintFields("https://finnprotec.fi/verkkokauppa/toimitus-ja-sopimusehdot/", finnprotecFiFixture, "finnprotec.fi")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Finnprotec Oy"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "Oy" {
		t.Errorf("Suffix = %q, want \"Oy\"", im.Suffix)
	}
	if im.Country != "FI" {
		t.Errorf("Country = %q, want FI", im.Country)
	}
	const wantRegister = "Y-tunnus 1938183-5"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Finnish ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want legal_name and contact present", im.FieldsFound)
	}
	if containsStr(im.FieldsFound, "address") {
		t.Errorf("FieldsFound = %v, want address ABSENT — this real page genuinely has no street address on it", im.FieldsFound)
	}
}

// TestYTunnusIdentifierChecksum is a direct unit test of the new Y-tunnus
// identifier: the real Y-tunnus found during this round's search must pass
// the FI weighted-mod-11 checksum, and a value with a flipped digit must
// fail it.
func TestYTunnusIdentifierChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"finnprotec.fi real Y-tunnus", "Y-tunnus: 1938183-5", checksumValid},
		{"flipped check digit must fail", "Y-tunnus: 1938183-4", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("Y-tunnus", tc.raw, "FI"); got != tc.want {
				t.Errorf("validateIdentifier(Y-tunnus, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("Y-tunnus"); got != "FI" {
		t.Errorf("singleCountryIdentifierKind(Y-tunnus) = %q, want FI", got)
	}
}
