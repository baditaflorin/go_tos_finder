package main

import "testing"

// Round-1 EU-expansion real-evidence fixtures, fetched 2026-08-22. Trimmed
// to the relevant excerpt, byte-for-byte as published, following this
// repo's inline-backtick fixture convention (see
// imprint_extract_real_evidence_test.go).

// humRestoFixture: real mentions-légales page, humresto.fr (a French SARL
// caterer/restaurant, no JSON-LD, name-form written prefix-style: "SARL
// Hum!Resto" rather than the name-then-suffix order this extractor
// otherwise assumes — see extractEntityAfterSuffix in imprint_name.go).
const humRestoFixture = `<!DOCTYPE html>
<html lang="fr">
<head><title>Mentions légales - Hum ! Resto</title></head>
<body>
<section>
<p class="wp-block-paragraph"><strong>SARL Hum!Resto</strong><br>20 rue Marcel Pagnol,<br>69720 Saint Bonnet de Mure</p>
<p class="wp-block-paragraph">Numéro d&rsquo;immatriculation au RCS : 980 184 584 R.C.S de Lyon<br>N° Siret : 980 184 584 000 12<br>Code APE : Service des traiteurs 5621Z</p>
<p class="wp-block-paragraph">Mail : <a href="mailto:contact@humresto.fr">contact@humresto.fr</a><br>Numéro de téléphone pour nous contacter : <a href="tel:06 09 66 92 85">06 09 66 92 85</a><br></p>
<p class="wp-block-paragraph">Numéro d&rsquo;identification à la TVA : FR25980184584<br>Identité de l&rsquo;hébergeur : OVH (numéro de téléphone : 1007)<br>Permis d&rsquo;exploitation n° OAF20230725, titulaire de la licence restaurant délivrée à Saint Bonnet</p>
</section>
</body>
</html>`

// simpelBootverhuurFixture: real colofon page,
// simpelbootverhuurutrecht.nl/colofon/ (a Dutch sole trader / eenmanszaak
// boat rental). No JSON-LD, no legal-form suffix at all (Eenmanszaak is a
// standalone legal-form declaration on its own line, not a name suffix —
// see standaloneLegalFormCandidate in imprint_name.go), and its real
// btw-id/KvK lines precede the real street address rather than follow it.
const simpelBootverhuurFixture = `<!DOCTYPE html>
<html lang="nl">
<head><title>Colofon - Simpel Bootverhuur Utrecht &amp; Bedrijfsinfo</title></head>
<body>
<h5 style="text-align:center"><span class="desktop-font-size-14">Simpel Bootverhuur Utrecht</span><br><span class="desktop-font-size-14">Eenmanszaak.&nbsp;</span><br><span class="desktop-font-size-14">KvK-nr: 42028124</span><br><span class="desktop-font-size-14">BTW-id: NL005444107B41</span><br><span class="desktop-font-size-14">Rietveldseweg 21 a 27 4107 LJ Culemborg</span><br><span class="desktop-font-size-14">Opstapplaats Oosterkade Utrecht</span></h5>
</body>
</html>`

// TestExtractImprintFieldsHumRestoFrenchPrefixFormRealEvidence: real
// evidence that casual French SARL/SAS usage often writes the legal form
// BEFORE the trading name ("SARL Hum!Resto"), not after it — the
// name-then-suffix order imprint_suffix.go's file comment assumed was
// universal for Latin-script jurisdictions. Also exercises two address
// false positives found on this same real page: a bare "rue" street line
// with no 4-digit house number was silently dropped (looksAddressLine had
// no French street-word marker), while the "Code APE : ... 5621Z" business
// classification code was wrongly absorbed into the address purely because
// its 4-digit-plus-letter shape cleared the digit-run heuristic.
func TestExtractImprintFieldsHumRestoFrenchPrefixFormRealEvidence(t *testing.T) {
	im := extractImprintFields("https://humresto.fr/mentions-legales/", humRestoFixture, "humresto.fr")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "SARL Hum!Resto"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (prefix-form SARL usage must be recognised)", im.LegalName, wantName)
	}
	if im.Suffix != "SARL" {
		t.Errorf("Suffix = %q, want SARL", im.Suffix)
	}
	const wantAddr = "20 rue Marcel Pagnol, 69720 Saint Bonnet de Mure"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q (must include the real \"rue\" street line and must NOT absorb the \"Code APE\" business-activity code)", im.Address, wantAddr)
	}
	if im.Country != "FR" {
		t.Errorf("Country = %q, want FR", im.Country)
	}
	if im.VAT != "FR25980184584" {
		t.Errorf("VAT = %q, want FR25980184584", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (French VAT = 2 check chars + Luhn-valid SIREN)", im.VATValidation)
	}
	if im.Register != "SIRET 980 184 584 000 12" {
		t.Errorf("Register = %q, want %q (round-4: RulesetFRLCEN now requires register; also exercises the SIRET pattern's real-evidence fixes — case-insensitive \"Siret\" and the 3-3-3-3-2 digit grouping this real page uses instead of 3-3-3-5)", im.Register, "SIRET 980 184 584 000 12")
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "register") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact+register+vat_valid", im.FieldsFound)
	}
	if im.Ruleset != RulesetFRLCEN {
		t.Errorf("Ruleset = %q, want fr_lcen (round-4: France now has a dedicated ruleset)", im.Ruleset)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}

// TestExtractImprintFieldsSimpelBootverhuurDutchSoleTraderRealEvidence:
// real evidence for a Dutch eenmanszaak (sole trader) with no legal-form
// suffix at all — "Eenmanszaak" is a standalone legal-form declaration on
// its own line, not glued onto the trading name the way GmbH/SARL are.
// Also exercises: the "colofon" (no 'h') vs "colophon" isLegalPage-gate
// typo; the KvK/BTW-id lines being wrongly absorbed into the address ahead
// of the real street line; and the post-2020 Dutch sole-trader BTW-id
// checksum, which has no published check-digit algorithm and must not be
// flagged checksum_invalid.
func TestExtractImprintFieldsSimpelBootverhuurDutchSoleTraderRealEvidence(t *testing.T) {
	im := extractImprintFields("https://simpelbootverhuurutrecht.nl/colofon/", simpelBootverhuurFixture, "simpelbootverhuurutrecht.nl")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Simpel Bootverhuur Utrecht"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (standalone \"Eenmanszaak\" legal-form fallback must fire)", im.LegalName, wantName)
	}
	if im.Suffix != "" {
		t.Errorf("Suffix = %q, want empty (a sole trader has no legal-form suffix)", im.Suffix)
	}
	const wantAddr = "Rietveldseweg 21 a 27 4107 LJ Culemborg"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q (must skip past the KvK/BTW-id lines to the real street address, not absorb them)", im.Address, wantAddr)
	}
	if im.Country != "NL" {
		t.Errorf("Country = %q, want NL", im.Country)
	}
	if im.Register != "KvK nr: 42028124" {
		t.Errorf("Register = %q, want %q", im.Register, "KvK nr: 42028124")
	}
	if im.VAT != "NL005444107B41" {
		t.Errorf("VAT = %q, want NL005444107B41", im.VAT)
	}
	if im.VATValidation != string(formatValid) {
		t.Errorf("VATValidation = %q, want format_valid (post-2020 Dutch sole-trader BTW-id has no published checksum — must not be flagged checksum_invalid)", im.VATValidation)
	}
	// Bug: titleCaseNameRun must not mistake the WHOLE trading name for a
	// person's name when nothing was actually isolated from it (contrast
	// with hotelrose.at, where "zur" isolates "Franz Holzmann" as a proper
	// subset of the full legal name).
	if im.ResponsiblePerson != "" {
		t.Errorf("ResponsiblePerson = %q, want empty (the trading name has no person embedded in it)", im.ResponsiblePerson)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") || !containsStr(im.FieldsFound, "register") {
		t.Errorf("FieldsFound = %v, want legal_name+address+register", im.FieldsFound)
	}
	if im.Ruleset != RulesetNLHandelsreg {
		t.Errorf("Ruleset = %q, want nl_handelsregisterwet (round-4: Netherlands now has a dedicated ruleset)", im.Ruleset)
	}
	if im.CompletenessScore != 60 {
		t.Errorf("CompletenessScore = %d, want 60 (nl_handelsregisterwet: legal_name+address+register found (3/5) — contact absent from this trimmed fixture, vat_valid unresolved)", im.CompletenessScore)
	}
}
