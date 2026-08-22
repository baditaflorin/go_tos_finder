package main

import "testing"

// Round-2 EU-expansion real-evidence fixtures, fetched 2026-08-22. Trimmed
// to the relevant excerpt, byte-for-byte as published, following this
// repo's inline-backtick fixture convention (see
// imprint_extract_real_evidence_test.go).

// simanovaFixture: real note legali page, simanova.it (Italian S.R.L.).
// The JSON-LD "name" field is styled "SIMANOVA S.R.L." (all caps, no
// spaces, dotted) — a real-world literal-string collision with Romania's
// suffixTable entry of the exact same shape (see country=RO note in the
// test below). The visible page text instead reads "SIMANOVA S. R. L."
// (each letter separately dotted and spaced).
const simanovaFixture = `<!DOCTYPE html>
<html lang="it">
<head>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"LocalBusiness","address":{"@type":"PostalAddress","addressLocality":"Caserta","streetAddress":"Via Tanucci 73","addressRegion":"Campania"},"url":"https://simanova.it","name":"SIMANOVA S.R.L.","sameAs":[],"email":"info@simanova.it"}
</script>
<title>Note legali - simanova.it</title>
</head>
<body>
<p style="text-align:center;"><strong>Azienda responsabile</strong></p>
<p style="text-align:center;">SIMANOVA S. R. L.</p>
<p style="text-align:center;">Via Tanucci 73, 81100 Caserta (CE)</p>
<p style="text-align:center;"><strong>Contatti</strong></p>
<p style="text-align:center;">info@simanova.it</p>
<p style="text-align:center;">simanova@pec.it</p>
<p style="text-align:center;"><strong>Iscrizione nel registro delle imprese</strong></p>
<p style="text-align:center;">Iscrizione nel registro delle imprese di Caserta (CE)</p>
<p style="text-align:center;">REA CE 362094</p>
<p style="text-align:center;"><strong>Partita IVA</strong></p>
<p style="text-align:center;">04869580615</p>
</body>
</html>`

// eurostarsFixture: real aviso legal page, eurostarshotels.com (a Spanish
// SL). Written as one long unbroken sentence with no <br> line breaks
// (common Spanish/civil-law legal-boilerplate style) — name, register
// entry, CIF, and phone all in a single 172-character paragraph.
const eurostarsFixture = `<!DOCTYPE html>
<html lang="es">
<head><title>Aviso legal</title></head>
<body>
<div class="sta-landingAvisoLegal-eh_content">
El propietario de esta web es: EUROSTARS HOTEL COMPANY SL, inscrita en el Registro Mercantil de Barcelona tomo 40703, Folio 196, Hoja B-372183, con CIF B64930910, teléfono <a href="tel:+34932681010">(+34)932681010</a>. Para más formas de contacto acceda a nuestra <a href="https://www.eurostarshotels.com/contactar.html">página de contacto principal.</a><br/><br/>
C/Mallorca 351, 08013 Barcelona, España<br/><br/>
EUROSTARS es una compañía de GRUPO HOTUSA, más información en <a href="https://www.grupohotusa.com/">www.grupohotusa.com.</a>
</div>
</body>
</html>`

// TestExtractImprintFieldsSimanovaItalianRealEvidence: real evidence for
// an Italian S.R.L. Exercises: a JSON-LD name in ALL-CAPS "S.R.L." form
// (distinct from mixed-case "S.r.l."); the "S. R. L." spaced visible-text
// stylization, which had no suffix-table entry at all; Partita IVA and REA
// (the Italian company register number, previously unmodeled entirely)
// being found by findIdentifiers but silently dropped before reaching the
// winning candidate; and a Kind-name collision where Spain's CIF used to be
// validated with Romania's CUI algorithm (see the Spain test below — this
// fixture only exercises the PartitaIVA side of that same switch).
//
// Country resolution (round 4 fix): the JSON-LD candidate's ALL-CAPS
// "S.R.L." is a genuine literal-string collision with Romania's own native
// "S.R.L." suffixTable entry (imprint_suffix.go) — Romanian companies
// write it exactly this way natively too, so this was never a pure
// Italian-vs-Romanian bug fixable by reordering/renaming table entries
// (that risked shadowing genuine Romanian detections, or colliding with
// the SEPARATE real France-vs-Italy "S.A.S." ambiguity elsewhere in the
// same table — see singleCountryIdentifierKind's doc comment in
// imprint_checksum.go for the full collision audit). Fixed instead by
// giving ground-truth government-identifier evidence (a Partita IVA/REA
// number cannot possibly be Romanian) precedence over the merely-
// probabilistic suffix guess, once backfillWinnerIdentifiers pulls those
// identifiers onto the winning JSON-LD candidate.
func TestExtractImprintFieldsSimanovaItalianRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.simanova.it/note-legali/", simanovaFixture, "simanova.it")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "SIMANOVA S.R.L."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	const wantAddr = "Via Tanucci 73, Caserta, Campania"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q", im.Address, wantAddr)
	}
	if im.Country != "IT" {
		t.Errorf("Country = %q, want IT (must not resolve to Romania despite the ALL-CAPS \"S.R.L.\" suffix collision — see singleCountryIdentifierKind)", im.Country)
	}
	if im.Register != "REA CE 362094" {
		t.Errorf("Register = %q, want %q (REA had no pattern modeled at all before this fix)", im.Register, "REA CE 362094")
	}
	if im.VAT != "04869580615" {
		t.Errorf("VAT = %q, want the clean 11-digit code (not the raw regex match with its label+newlines glued on)", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (Partita IVA is Luhn-valid)", im.VATValidation)
	}
	if im.Ruleset != RulesetITDLgs70 {
		t.Errorf("Ruleset = %q, want it_dlgs70 (round-4: Italy now has a dedicated ruleset — was unreachable until Country resolved correctly)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "register") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact+register+vat_valid", im.FieldsFound)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}

// TestExtractImprintFieldsEurostarsSpanishRealEvidence: real evidence for
// a Spanish SL. Exercises: the undotted "SL" suffix stylization (no entry
// existed at all before this fix); a 172-character unbroken sentence that
// exceeded the old 140-char per-line cap and was silently skipped
// entirely; a "the owner of this website is: X" prose lead-in that must be
// stripped from the front of the extracted name; a bare, unlabelled
// international phone number on its own line (from an <a href="tel:...">
// anchor) false-positiving into the address; and the CIF/CUI Kind-name
// collision — Spain's CIF used to be routed through Romania's roCUIValid
// checksum algorithm.
func TestExtractImprintFieldsEurostarsSpanishRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.eurostarshotels.com/aviso-legal.html", eurostarsFixture, "eurostarshotels.com")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "EUROSTARS HOTEL COMPANY SL"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (must strip the \"El propietario de esta web es:\" lead-in)", im.LegalName, wantName)
	}
	if im.Suffix != "SL" {
		t.Errorf("Suffix = %q, want SL", im.Suffix)
	}
	const wantAddr = "C/Mallorca 351, 08013 Barcelona, España"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q (must NOT include the bare phone number line)", im.Address, wantAddr)
	}
	if im.Country != "ES" {
		t.Errorf("Country = %q, want ES", im.Country)
	}
	if im.VAT != "B64930910" {
		t.Errorf("VAT = %q, want B64930910 (clean CIF code, not the raw \"CIF ...\" regex match)", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (a real Spanish CIF validated via esCIFValid, not Romania's roCUIValid)", im.VATValidation)
	}
	if im.Register != "Hoja B-372183" {
		t.Errorf("Register = %q, want %q (round-4: Spain's Registro Mercantil \"Hoja\" citation had no pattern modeled at all before this fix)", im.Register, "Hoja B-372183")
	}
	if im.Ruleset != RulesetESLSSICE {
		t.Errorf("Ruleset = %q, want es_lssice (round-4: Spain now has a dedicated ruleset)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "register") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact+register+vat_valid", im.FieldsFound)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}

// TestValidateIdentifierCIFDoesNotUseRomanianAlgorithm is a direct unit
// test of the CUI/CIF Kind-name collision fix, independent of the full
// extraction pipeline: a genuine Spanish CIF must validate via esCIFValid,
// and a genuine Romanian CUI must still validate via roCUIValid — both
// continuing to work after the switch case was split.
func TestValidateIdentifierCIFDoesNotUseRomanianAlgorithm(t *testing.T) {
	if got := validateIdentifier("CIF", "CIF B64930910", "ES"); got != checksumValid {
		t.Errorf("Spanish CIF B64930910: validateIdentifier(\"CIF\", ...) = %q, want checksum_valid", got)
	}
	if got := validateIdentifier("CUI", "CUI RO10000008", "RO"); got != checksumValid {
		t.Errorf("Romanian CUI 10000008 (mod-11 valid): validateIdentifier(\"CUI\", ...) = %q, want checksum_valid", got)
	}
}
