package main

import "testing"

// Round-22 EU-expansion real-evidence fixture, fetched 2026-08-23:
// Slovenia. Real excerpt from https://sgermobil.si/splosni-pogoji/ (SGERM
// trgovina, storitve, posredništvo in proizvodnja d.o.o. — a real, live
// Slovenian retailer's Splošni pogoji page) — byte-for-byte from the live
// page's own markup, including its literal un-decoded HTML entity
// (&nbsp;).
//
// This is the ACTUAL Croatia/Slovenia "d.o.o." collision round 20's
// suffixTable comment anticipated: Slovenia writes its own "družba z
// omejeno odgovornostjo" with the identical "d.o.o." abbreviation Croatia
// already claims in suffixTable, tagged "HR". Running the shipped 1.7.16
// extractor against this real page extracted NOTHING at all
// (CompletenessScore 0, not even "contact" — the fixture below omits the
// page's own email line to isolate this round's actual fixes). Found and
// fixed four compounding real bugs:
//
//  1. Slovenia's "Davčna številka" (tax number) had no domestic pattern,
//     and the EXISTING "SI"-prefixed EU VAT pattern
//     (imprint_vat.go, present before this round) required the digits to
//     butt directly against "SI" with zero space — but this real page
//     writes it as "Davčna številka: SI 11597267" (space-separated),
//     same class of gap round 7's BE/FR space-tolerance fixes already
//     covered for other countries. Fixed by adding the same optional
//     space. Combined with the identifier-presence gate that guards
//     extractImprintText's suffix-anchored scan (this page's own
//     "splosni-pogoji" URL has no English isLegalPage keyword either),
//     this alone was enough to unblock the whole extraction.
//  2. Slovenia's "Matična številka" (company registration number) had no
//     identifier pattern at all. Added it, wired as Register-only (same
//     shape as Croatia's MBS) — Slovenia's own tax number needed no
//     separate domestic pattern, since its real form is already the same
//     shape the "SI"-prefixed VAT pattern matches (fixed in bug 1).
//  3. Once found, "Matična številka"/"Davčna številka" values leaked into
//     Address (both sit right after the real "2000 Maribor" postal-code/
//     city line) — added both as skip-past markers, same shape as round
//     21's Slovak markers.
//  4. This same real page's separate court-registration sentence
//     ("registrirana pri Okrožnem sodišču v Mariboru ... pod vložno
//     številko 1/12603/00") ALSO leaked into Address — its own citation
//     number cleared looksAddressLine the same way round 10's Luxembourg
//     "RCS Luxembourg n°..." citation did. Added "vložno številko" as a
//     third skip-past marker.
//
// "Matična številka" was ALSO added to singleCountryIdentifierKind
// (-> "SI") — and THIS is the concrete confirmation that round 20's
// self-healing design works end to end: this page's winning candidate's
// suffix-guessed country would otherwise have been "HR" (suffixTable's
// only "d.o.o." entry), but the ground-truth "Matična številka" evidence
// correctly overrides it to "SI", the exact mechanism (and exact
// collision) round 20's doc comment predicted before any Slovenian
// evidence existed to confirm it.
//
// One real, honestly-documented gap NOT fixed this round: Address only
// captures "2000 Maribor" (the postal-code+city line, whose "2000" 4-digit
// run clears looksAddressLine on its own) — the real street line
// ("Beloruska ulica 7") never clears looksAddressLine at all (no Slovenian
// street-word marker exists, and "7" alone is nowhere near the
// 4-consecutive-digit threshold), the same class of gap already left
// undocumented for Czech/Slovak/Croatian street lines in rounds 16/20/21.
// No checksum implemented for Slovenian VAT/Matična številka either (stays
// format_valid) — same discipline as Ireland's CRO/Croatia's MBS/
// Slovakia's DIČ.
const sgermobilSiFixture = `<!DOCTYPE html>
<html lang="sl">
<head><title>Splošni pogoji</title></head>
<body>
<div><p><b>Prodajalec oziroma ponudnik spletne trgovine&nbsp;</b></p></div>
<div><p>SGERM trgovina, storitve, posredništvo in proizvodnja d.o.o.<br />Beloruska ulica 7<br />2000 Maribor<br />Matična številka: 2153254000<br />Davčna številka: SI 11597267<br />(v nadaljevanju: »družba«)</p>
<p>Družba SGERM trgovina, storitve, posredništvo in proizvodnja d.o.o. je od dne 20.09.2005 registrirana pri Okrožnem sodišču v Mariboru kot družba z omejeno odgovornostjo pod vložno številko 1/12603/00.</p></div>
</body>
</html>`

func TestExtractImprintFieldsSgermobilSiRealEvidence(t *testing.T) {
	im := extractImprintFields("https://sgermobil.si/splosni-pogoji/", sgermobilSiFixture, "sgermobil.si")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "SGERM trgovina, storitve, posredništvo in proizvodnja d.o.o."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "d.o.o." {
		t.Errorf("Suffix = %q, want \"d.o.o.\"", im.Suffix)
	}
	if im.Country != "SI" {
		t.Errorf("Country = %q, want SI (must NOT be HR — the shared \"d.o.o.\" suffix guess corrected by the Matična številka ground-truth identifier, exactly as round 20 anticipated)", im.Country)
	}
	const wantRegister = "Matična številka 2153254000"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.VAT != "SI11597267" {
		t.Errorf("VAT = %q, want SI11597267 (space-tolerant SI VAT pattern)", im.VAT)
	}
	const wantAddress = "2000 Maribor"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (must NOT include the Matična številka/Davčna številka lines)", im.Address, wantAddress)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Slovenian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") {
		t.Errorf("FieldsFound = %v, want legal_name and address present", im.FieldsFound)
	}
}

// TestSiVATPatternToleratesSpace is a direct unit test of the space-
// tolerance fix: "SI 11597267" (space-separated, the real form this
// round's evidence uses) must match, same as the pre-existing bare-
// adjacent "SI11597267" form.
func TestSiVATPatternToleratesSpace(t *testing.T) {
	ids := findIdentifiers("Davčna številka: SI 11597267", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "VAT" && id.Country == "SI" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match \"SI 11597267\" as a VAT hit — got %v", ids)
	}
}

// TestSiCollisionSelfHeals is a direct unit test of the singleCountryIdentifierKind
// wiring: "Matična številka" must resolve to "SI", confirming the
// Croatia/Slovenia "d.o.o." suffix collision (see suffixTable's round-20
// doc comment) self-heals via this ground-truth identifier.
func TestSiCollisionSelfHeals(t *testing.T) {
	if got := singleCountryIdentifierKind("Matična številka"); got != "SI" {
		t.Errorf("singleCountryIdentifierKind(Matična številka) = %q, want SI", got)
	}
}

// TestSiCourtRegistrationMarkerSkipsNotBreaks is a direct unit test of the
// extractAddressNearEntity fix: a "vložno številko" court-registration
// citation line must be skipped (continue), not absorbed into the address.
func TestSiCourtRegistrationMarkerSkipsNotBreaks(t *testing.T) {
	text := "SGERM d.o.o.\n2000 Maribor\nDružba je registrirana pri Okrožnem sodišču v Mariboru pod vložno številko 1/12603/00."
	got := extractAddressNearEntity(text, "SGERM d.o.o.")
	const want = "2000 Maribor"
	if got != want {
		t.Errorf("extractAddressNearEntity = %q, want %q", got, want)
	}
}
