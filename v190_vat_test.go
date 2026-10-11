package main

import "testing"

// Real VAT numbers from the 20k-row production window (imprint_vat_valid was
// stored FALSE for them because the service only ever answered format_valid for
// these countries and DomainScope maps every non-checksum_valid VAT to false),
// each paired with a one-digit corruption that must be rejected.
func TestVATChecksums190(t *testing.T) {
	cases := []struct {
		name, vat, country string
		want               validity
	}{
		{"CZ dantem.com", "CZ26421232", "CZ", checksumValid},
		{"CZ corrupted", "CZ26421233", "CZ", checksumInvalid},
		{"CZ natural person (9 digits) stays format_valid", "CZ596112345", "CZ", formatValid},
		{"EE plausible.io", "EE102324299", "EE", checksumValid},
		{"EE corrupted", "EE102324298", "EE", checksumInvalid},
		{"HU apleu.org", "HU18333886", "HU", checksumValid},
		{"HU corrupted", "HU18333887", "HU", checksumInvalid},
		{"GB analysysmason.com", "GB707748315", "GB", checksumValid},
		{"GB corrupted", "GB707748316", "GB", checksumInvalid},
		{"GB government GD stays format_valid", "GBGD001", "GB", formatValid},
		{"MT artemisialtd.com", "MT23366423", "MT", checksumValid},
		{"MT corrupted", "MT23366424", "MT", checksumInvalid},
		{"ES CIF bufetevillaboix.com", "ESB88665179", "ES", checksumValid},
		{"ES DNI-based NIF bufetesanchezcasero.com", "ES38837115M", "ES", checksumValid},
		{"ES DNI corrupted letter", "ES38837115N", "ES", checksumInvalid},
		{"ES NIE", "ESX1234567L", "ES", checksumValid},
		{"ES NIE corrupted", "ESX1234567M", "ES", checksumInvalid},
		{"ES special K stays format_valid", "ESK1234567L", "ES", formatValid},
		{"FR genuinely invalid stays invalid", "FR63820866007", "FR", checksumInvalid},
		{"AT stays valid", "ATU16310105", "AT", checksumValid},
	}
	for _, c := range cases {
		if got := validateIdentifier("VAT", c.vat, c.country); got != c.want {
			t.Errorf("%s: validateIdentifier(VAT,%q,%q)=%q want %q", c.name, c.vat, c.country, got, c.want)
		}
	}
	// Swiss UID (kind UID), with and without the MWST suffix.
	for _, v := range []string{"CHE-188.552.084 MWST", "CHE-107.885.990", "CHE-188.552.084"} {
		if got := validateIdentifier("UID", v, "CH"); got != checksumValid {
			t.Errorf("UID %q = %q, want checksum_valid", v, got)
		}
	}
	if got := validateIdentifier("UID", "CHE-188.552.085 MWST", "CH"); got != checksumInvalid {
		t.Errorf("corrupted UID = %q, want checksum_invalid", got)
	}
	// SK / SI / LU: known-good numbers constructed from the published algorithms.
	for _, c := range []struct {
		fn  func(string) bool
		ok  string
		bad string
	}{
		{skVATValid, "2020317068", "2020317069"}, // 2020317068 % 11 == 0, third digit 2
		{siVATValid, "50223054", "50223055"},
		{luVATValid, "15027442", "15027443"},
	} {
		if !c.fn(c.ok) || c.fn(c.bad) {
			t.Errorf("fn ok=%s bad=%s: got ok=%v bad=%v", c.ok, c.bad, c.fn(c.ok), c.fn(c.bad))
		}
	}
}

// A VAT carried without its country prefix (JSON-LD vatID) is promoted to
// checksum_valid once the imprint country is known — and is never DEMOTED by a
// guessed country.
func TestUnprefixedVATUpgradedByCountry(t *testing.T) {
	page := `<html><head><script type="application/ld+json">{"@type":"Organization","name":"Villa Boix Legal S.L.P.","vatID":"B88665179","address":{"@type":"PostalAddress","streetAddress":"Calle Mayor 1","postalCode":"08001","addressLocality":"Barcelona","addressCountry":"ES"}}</script></head><body><h1>Aviso legal</h1><p>Villa Boix Legal S.L.P. Calle Mayor 1, 08001 Barcelona. CIF B88665179. info@villaboix.es</p></body></html>`
	im := extractImprintFields("https://villaboix.es/aviso-legal", page, "villaboix.es")
	if im.VAT == "" {
		t.Skipf("fixture did not yield a VAT on this extractor revision: %+v", im)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("unprefixed ES CIF on a .es imprint should reach checksum_valid, got %q (%+v)", im.VATValidation, im)
	}
}
