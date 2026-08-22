package main

import "testing"

// Round-7 EU-expansion real-evidence fixture, fetched 2026-08-22.

// factuoBeFixture: real "Mentions Légales" page, factuo.be — a Belgian
// SRL ("Saubeo SRL") whose real page ALSO names its French hosting
// provider ("Amen SASU") with its own French RCS/APE/VAT block — a real
// multi-entity page, exercising candidate disambiguation (the sibling
// go_tos_finder repo's own suffix-anchored scan must pick the SITE
// OWNER's identifiers, not the unrelated hosting company's).
const factuoBeFixture = `<!DOCTYPE html>
<html lang="fr">
<head><title>Mentions Légales - Factuo</title></head>
<body>
<main>
<h3>Siège social</h3>
<p>Saubeo SRL<br />Rafhay 198<br />4630 Soumagne<br />TVA BE 0704742612</p>
<h3>Hébergeur</h3>
<p>Amen SASU<br />Immatriculation RCS Paris B 421 527 797<br />Nomenclature APE 6312Z<br />Num. de TVA : FR 29 421 527 797<br />Adresse du siège : 12-14, Rond-Point des Champs-Élysées &ndash; 75008 Paris &ndash; France</p>
<h3>Développement</h3>
<p>Saubeo SRL<br />Adresse : Rafhay 198 4630 Soumagne<br />Site Web : saubeo.be</p>
</main>
</body>
</html>`

// TestExtractImprintFieldsFactuoBelgianRealEvidence: real evidence for a
// Belgian SRL. Exercises: the extremely common spaced VAT formatting
// ("TVA BE 0704742612" — the original BE and FR patterns both required
// the digits to butt directly against the country prefix with zero
// space, which real-world usage routinely doesn't do); proximity-based
// disambiguation correctly preferring the site-owner's own nearby VAT
// over the unrelated French hosting company's VAT found elsewhere on the
// same page; and a real cross-entity address-absorption bug where
// "Nomenclature APE 6312Z" (an alternate real phrasing of the French
// business-activity code, distinct from "Code APE") dragged a completely
// different company's address, in a different country, into the
// Belgian company's address field.
//
// Known, honestly-documented gap NOT fixed this round: Address captures
// only "4630 Soumagne" (the postal-code/city line), not the preceding
// "Rafhay 198" street line — a bare "<name> <number>" with no
// recognisable street-type word at all (no "rue"/"straat"/"laan" marker
// to key off), a different and harder class of gap than the previous
// rounds' missing-marker fixes. The "address" checklist item is still
// satisfied (some real address text was found), so this doesn't affect
// completeness_score.
func TestExtractImprintFieldsFactuoBelgianRealEvidence(t *testing.T) {
	im := extractImprintFields("https://factuo.be/mentions-legales/", factuoBeFixture, "factuo.be")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Saubeo SRL"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (must pick the site-owner entity, not the unrelated French hosting company named later on the page)", im.LegalName, wantName)
	}
	if im.Suffix != "SRL" {
		t.Errorf("Suffix = %q, want SRL", im.Suffix)
	}
	const wantAddr = "4630 Soumagne"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q (must NOT absorb the VAT line, the French hosting company's activity code, or its entire unrelated address)", im.Address, wantAddr)
	}
	if im.Country != "BE" {
		t.Errorf("Country = %q, want BE", im.Country)
	}
	if im.VAT != "BE0704742612" {
		t.Errorf("VAT = %q, want BE0704742612 (the site owner's own VAT, spaced in the source as \"TVA BE 0704742612\" — not the unrelated French hosting company's \"FR 29 421 527 797\")", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name+address+vat_valid", im.FieldsFound)
	}
}

// TestFindIdentifiersVATToleratesRealWorldSpacing is a direct unit test of
// the BE/FR VAT spacing fix, independent of the full extraction pipeline.
func TestFindIdentifiersVATToleratesRealWorldSpacing(t *testing.T) {
	cases := []struct {
		text string
		want string
	}{
		{"TVA BE 0704742612", "BE0704742612"},
		{"Num. de TVA : FR 29 421 527 797", "FR29421527797"},
	}
	for _, c := range cases {
		ids := findIdentifiers(c.text, 5)
		found := false
		for _, id := range ids {
			if id.Kind == "VAT" {
				found = true
				if got := cleanIdentifierValue(id.Kind, id.Value); got != c.want {
					t.Errorf("cleanIdentifierValue(VAT, %q) = %q, want %q", id.Value, got, c.want)
				}
			}
		}
		if !found {
			t.Errorf("findIdentifiers(%q) did not find a VAT hit, got %+v", c.text, ids)
		}
	}
}
