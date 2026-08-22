package main

import (
	"strings"
	"testing"
)

// Round-21 EU-expansion real-evidence fixture, fetched 2026-08-23: Slovakia.
// Real excerpt from https://www.elektro-siete.sk/obchodne-podmienky/
// (Elektro-Siete, s.r.o. — a real, live Slovak electrical-supplies
// retailer's Obchodné podmienky page) — byte-for-byte from the live page's
// own markup, including its literal un-decoded HTML entity (&nbsp;).
//
// This is the real test of round 16's own foresight: Czechia's "IČO"
// identifier and "s.r.o." suffix are BOTH shared, letter-for-letter, with
// Slovakia (former Czechoslovakia) — round 16 deliberately did NOT wire
// "IČO" into singleCountryIdentifierKind for exactly this reason, leaving
// a comment that a future Slovak page should self-heal via the SK-prefixed
// VAT number instead. This round confirms that worked, but also surfaced
// two real, NOT-yet-covered bugs on this real page:
//
//  1. Slovak "IČO" (46775994) uses the identical published check-digit
//     algorithm as Czech "IČO" — confirmed by hand against this page's
//     real value (weighted sum 216, 216 mod 11 = 7, check 11-7 = 4,
//     matches the real value's own 8th digit) using the SAME czICOValid
//     function round 16 already shipped. No new checksum code needed.
//     Country correctly resolves to "SK" (not the suffix-guessed "CZ")
//     because this page ALSO states "IČ DPH: SK2023571528" — a plain
//     "VAT" Kind match whose "SK" prefix takes precedence over the
//     suffix guess in extractImprintFields' existing country-precedence
//     chain (imprint.go), exactly the self-healing path round 16
//     anticipated. No code change was needed for this — confirmed via
//     this round's real evidence, not assumed.
//  2. Real bug found and fixed: this page's own "IČO: 46775994<br />DIČ:
//     2023571528<br />IČ DPH: SK2023571528" block, plus a separate
//     "Číslo účtu:...", plus a separate "...Obchodnom registri...vložka
//     číslo: 42190/N" court-register citation, ALL have long digit runs
//     that cleared looksAddressLine and got wrongly absorbed into
//     Address — extractAddressNearEntity had no stop-marker for any of
//     these Slovak labels (round 16 never needed one for its own
//     fixture). Added "ičo:"/"dič:"/"ič dph"/"iban" and "číslo účtu"/
//     "obchodnom registri" as skip-past markers — the "ičo:"/"dič:"
//     forms deliberately include the trailing colon (unlike this
//     codebase's other short-abbreviation markers) since bare "ičo"/"dič"
//     substrings risk false-positiving on ordinary Czech/Slovak words
//     (e.g. "dedičom"). This retroactively benefits round 16's Czech
//     pages too, which never had this exact marker.
//
// One real, honestly-documented gap NOT fixed this round: with the
// pollution removed, Address comes back EMPTY on this page — the real
// address ("Druhá 265/47", "Rastislavice", "941 08") never clears
// looksAddressLine at all (no Slovak street-word marker exists, and the
// "941 08" postal code's 3+2 spaced digit grouping never reaches the
// 4-consecutive-digit threshold) — the SAME class of gap round 16 already
// left honestly undocumented for Czech postal codes ("PSČ 130 00").
//
// No checksum implemented for Slovak "IČ DPH"/VAT this round either
// (stays format_valid): the modern Slovak DIČ/VAT numbering scheme has
// been reformed over time and this round did not have confidence in the
// current published check-digit algorithm to verify safely against real
// data — same discipline as Ireland's CRO/Luxembourg's RCS/Croatia's MBS.
const elektroSieteSkFixture = `<!DOCTYPE html>
<html lang="sk">
<head><title>Obchodné podmienky</title></head>
<body>
<div itemprop="about">
<div>
<p>Obchodník:</p>
<p>Sídlo spoločnosti:<br />Elektro-Siete, s.r.o.<br />Druhá 265/47<br />Rastislavice<br />941 08</p>
<p>Fakturačné údaje:<br />IČO: 46775994<br />DIČ: 2023571528<br />IČ DPH: SK2023571528</p>
<p>IBAN: SK67 0900 0000 0050 3227 2273<br />Číslo účtu:&nbsp;5032272273/0900</p>
<p>Naša spoločnosť je platcom DPH</p>
<p>Spoločnosť je zapísaná v Obchodnom registri okresného súdu Nitra, vložka číslo: 42190/N</p>
</div>
</div>
</body>
</html>`

func TestExtractImprintFieldsElektroSieteSkRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.elektro-siete.sk/obchodne-podmienky/", elektroSieteSkFixture, "elektro-siete.sk")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Elektro-Siete, s.r.o."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "s.r.o." {
		t.Errorf("Suffix = %q, want \"s.r.o.\"", im.Suffix)
	}
	if im.Country != "SK" {
		t.Errorf("Country = %q, want SK (must NOT be CZ — the shared \"s.r.o.\" suffix guess corrected by the SK-prefixed VAT number)", im.Country)
	}
	const wantRegister = "IČO 46775994"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.VAT != "SK2023571528" {
		t.Errorf("VAT = %q, want SK2023571528", im.VAT)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty and must NOT contain any of IČO/DIČ/IBAN/účtu/register pollution (known, honestly-documented real-address gap this round — see doc comment above)", im.Address)
	}
	for _, leak := range []string{"IČO", "DIČ", "IBAN", "účtu", "registri"} {
		if strings.Contains(im.Address, leak) {
			t.Errorf("Address = %q, must not contain %q", im.Address, leak)
		}
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Slovak ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") {
		t.Errorf("FieldsFound = %v, want legal_name present", im.FieldsFound)
	}
}

// TestSkICOReusesCzechChecksum is a direct unit test confirming Slovak IČO
// (46775994, found on this round's real page) validates correctly through
// the SAME czICOValid function round 16 shipped for Czechia — no new
// checksum code was needed, since both countries publish the identical
// algorithm for this shared former-Czechoslovak identifier.
func TestSkICOReusesCzechChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"elektro-siete.sk real IČO", "IČO: 46775994", checksumValid},
		{"flipped last digit must fail", "IČO: 46775993", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("IČO", tc.raw, "SK"); got != tc.want {
				t.Errorf("validateIdentifier(IČO, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestSkAddressMarkersExcludeFinancialAndRegisterLines is a direct unit
// test of the new extractAddressNearEntity markers: IČO/DIČ/IČ DPH/IBAN/
// account-number/register-citation lines must all be skipped, not
// absorbed into the address.
func TestSkAddressMarkersExcludeFinancialAndRegisterLines(t *testing.T) {
	text := "Elektro-Siete, s.r.o.\nIČO: 46775994\nDIČ: 2023571528\nIČ DPH: SK2023571528\nIBAN: SK67 0900 0000 0050 3227 2273\nČíslo účtu: 5032272273/0900\nSpoločnosť je zapísaná v Obchodnom registri okresného súdu Nitra, vložka číslo: 42190/N"
	got := extractAddressNearEntity(text, "Elektro-Siete, s.r.o.")
	if got != "" {
		t.Errorf("extractAddressNearEntity = %q, want empty (every line here is a stop-marker line, not a real address)", got)
	}
}
