package main

import "testing"

// Round-5 EU-expansion real-evidence fixture, fetched 2026-08-22.

// orlenNeptunFixture: real "Dane kontaktowe i rejestrowe" (contact and
// registry data) page, neptun.orlen.pl — a Polish Sp. z o.o. (a subsidiary
// of ORLEN, but the page itself is a genuine, unmodified real corporate
// registry-disclosure page, not a synthetic fixture).
const orlenNeptunFixture = `<!DOCTYPE html>
<html lang="pl">
<head><title>Dane kontaktowe i rejestrowe</title></head>
<body>
<p>ORLEN Neptun sp. z o.o.</p>
<p>Aleja Grunwaldzka 472, 80-309 Gdańsk</p>
<p><span style="letter-spacing: 0.18px;">NIP: 5252855028<br>
 KRS: 0000888254<br>
 REGON: 388432405</span></p>
<p>Sąd Rejonowy Gdańsk - Północ w Gdańsku<br>
VII Wydział Gospodarczy Krajowego Rejestru Sądowego</p>
<p>Kapitał zakładowy:&nbsp;1 595 000,00 zł</p>
<p>neptun@orlen.pl</p>
</body>
</html>`

// TestExtractImprintFieldsOrlenNeptunPolishRealEvidence: real evidence for
// a Polish Sp. z o.o. Before this round, NIP/KRS/REGON had no patterns
// modeled at all — findIdentifiers found nothing, extractImprintText's
// isLegalPage fallback ("useful only if VAT/register IDs are present")
// then bailed out before ever reaching the suffix-anchored scan, so this
// real page extracted NOTHING at all (no legal_name, no address, no
// identifiers) despite "ORLEN Neptun sp. z o.o." being trivially
// suffix-matchable in isolation. Also exercises two address false
// positives: the real street address ("Aleja Grunwaldzka 472, 80-309
// Gdańsk") cleared no digit-run threshold and had no Polish street-word
// marker, while the newly-added NIP/KRS/REGON lines (10, 10, and 9 digits)
// would otherwise have been wrongly absorbed into the address in its
// place.
func TestExtractImprintFieldsOrlenNeptunPolishRealEvidence(t *testing.T) {
	im := extractImprintFields("https://neptun.orlen.pl/pl/kontakt/dane-kontaktowe-i-rejestrowe", orlenNeptunFixture, "neptun.orlen.pl")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "ORLEN Neptun sp. z o.o."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "sp. z o.o." {
		t.Errorf("Suffix = %q, want \"sp. z o.o.\"", im.Suffix)
	}
	const wantAddr = "Aleja Grunwaldzka 472, 80-309 Gdańsk"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q (must include the real Polish street line and must NOT absorb the NIP/KRS/REGON lines)", im.Address, wantAddr)
	}
	if im.Country != "PL" {
		t.Errorf("Country = %q, want PL", im.Country)
	}
	if im.Register != "KRS 0000888254" {
		t.Errorf("Register = %q, want %q (KRS had no pattern modeled at all before this fix)", im.Register, "KRS 0000888254")
	}
	if im.VAT != "5252855028" {
		t.Errorf("VAT = %q, want the clean 10-digit NIP code", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (Polish NIP uses the same weighted-mod-11 algorithm as the PL-prefixed VAT form)", im.VATValidation)
	}
	if im.Ruleset != RulesetPLUsude {
		t.Errorf("Ruleset = %q, want pl_usude", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "register") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact+register+vat_valid", im.FieldsFound)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}
