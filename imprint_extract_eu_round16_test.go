package main

import (
	"strings"
	"testing"
)

// Round-16 EU-expansion real-evidence fixture, fetched 2026-08-22: Czechia
// (new series, rounds 15+, covering the 13 remaining EU countries after the
// original rounds 1-14). Real excerpt from
// https://www.onlineshop.cz/obchodni-podminky.html (SHOP TRADING, s.r.o. —
// a real, live Czech white-goods e-shop's Obchodní podmínky page) —
// byte-for-byte from the live page's own markup.
//
// Running the shipped 1.7.10 extractor against this real page extracted
// NOTHING at all (CompletenessScore 0), despite "SHOP TRADING, s.r.o."
// being trivially suffix-matchable ("s.r.o.", already a native, uniquely-CZ
// suffixTable entry) right next to its IČO. Found and fixed three
// compounding real bugs:
//
//  1. stripTagsLines inserts a "\n" at every tag boundary — this page
//     writes the entity name as "<strong>SHOP TRADING, s.r.o</strong>., se
//     sídlem ..." (a common real authoring pattern: bold entity name, plain
//     trailing punctuation outside the tag), so the closing "." of the
//     "s.r.o." suffix landed on its OWN line, split from the "s.r.o" that
//     precedes it. suffixTable's literal, dot-terminated "s.r.o." then
//     never matched at all, so the ENTIRE suffix-anchored scan silently
//     found nothing anywhere on the page. Fixed with a new
//     tagBoundaryPunctRE post-process step in stripTagsLines: collapses a
//     tag-boundary newline back out specifically when a Unicode LETTER
//     immediately precedes it and a bare "."/"," immediately follows —
//     i.e. only when the break falls mid-abbreviation, not at a genuine
//     sentence boundary. Gating on "letter before" (not "any char before")
//     was necessary to avoid a real regression this round's first attempt
//     caused: eurostarshotels.com's real aviso legal (round 2) has a bare
//     phone number in its own <a href="tel:..."> tag immediately followed
//     by ". Para más formas de contacto..." outside it — a DIGIT precedes
//     that break, and unconditionally collapsing it merged the isolated
//     phone-number line into the following prose, defeating
//     bareIntlPhoneRE's whole-line match and leaking the phone number into
//     the address. See tagBoundaryPunctRE's doc comment (imprint_name.go).
//  2. Once bug 1 exposed the real sentence, it turned out to be 280 runes
//     but 307 BYTES (Czech diacritics are 2 bytes each in UTF-8) — over
//     extractImprintText's existing 220-BYTE per-line cap (raised from 140
//     in round 2 for the exact same class of gap: one long, unbroken
//     civil-law-style sentence with no <br> breaks). Switched the cap from
//     len() (byte count) to utf8.RuneCountInString (visible-character
//     count) so accented-heavy languages aren't penalised relative to
//     ASCII text of the same visible length, and raised it to 300.
//  3. Czechia's IČO (Identifikační číslo osoby) had no vatPatterns entry at
//     all, so findIdentifiers found nothing on this page — and since the
//     real URL ("obchodni-podminky.html") contains no isLegalPage English
//     keyword either, extractImprintText's "useful only if VAT/register
//     IDs are present" fallback bailed out before the (now-fixed)
//     suffix-anchored scan could even run. Added a dedicated "IČO"
//     vatPattern, wired into the Register-field Kind switch (same
//     silently-dropped-Kind failure shape as round 14/15's Norwegian
//     OrgNr/Romanian CUI), and a new czICOValid weighted-mod-11 checksum,
//     confirmed against this page's real IČO (24717509).
//
// Deliberately NOT added: an "IČO" case in singleCountryIdentifierKind.
// Unlike CUI/KvK/CRO/CVR/Y-tunnus/OrgNr, "IČO" is NOT single-country —
// former Czechoslovakia left both Czechia and Slovakia using the identical
// label for their (different-checksum) domestic business ID, so mapping it
// to "CZ" here would silently mis-flag a future real Slovak page. Not
// needed for this fixture anyway: "s.r.o." is CZ-unambiguous in the
// current suffixTable (no Slovak entry exists yet). See
// singleCountryIdentifierKind's doc comment (imprint_checksum.go).
//
// One real, honestly-documented gap NOT fixed this round: Address stays
// empty on this specific page. The real address ("se sídlem Praha 3 -
// Žižkov, Hartigova 2660/141, PSČ 130 00") sits on the SAME line as the
// entity name and its register clause (no further tag boundary splits
// them), and extractAddressNearEntity only ever scans FORWARD from the
// line AFTER the entity's own line — so it never looks at the remainder of
// the entity's own line. The page's separate, cleaner "16.4. Kontaktní
// údaje prodávajícího" section ("SHOP TRADING s.r.o., Komenského 63, 543 01
// Vrchlabí") was checked as a possible substitute (following round 14's
// Norway precedent of using a cleaner alternate section) but does not
// clear looksAddressLine either — no Czech street-word marker exists in
// its vocabulary list, and Czech postal codes are conventionally written
// "XXX XX" (3+2 digits, space-separated), which never reaches the
// 4-consecutive-digit threshold hasDigitRun requires. A real fix needs
// either a Czech street-name heuristic or postal-code-shaped marker this
// round did not have evidence to design safely — left for a future round.
const onlineshopCzFixture = `<!DOCTYPE html>
<html lang="cs">
<head><title>Obchodní podmínky</title></head>
<body>
<h1>Obchodní podmínky</h1>
<p>obchodní korporace <strong>SHOP TRADING, s.r.o</strong>., se sídlem Praha 3 - Žižkov, Hartigova 2660/141, PSČ 130 00, IČO: 247 17 509, společnost zapsaná v obchodním rejstříku vedeném Městským soudem v Praze, oddíl C, vložka č. 168462, (dále jako „<strong>prodávající</strong>") pro prodej zboží prostřednictvím internetového obchodu.</p>
</body>
</html>`

func TestExtractImprintFieldsOnlineshopCzRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.onlineshop.cz/obchodni-podminky.html", onlineshopCzFixture, "onlineshop.cz")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "SHOP TRADING, s.r.o."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "s.r.o." {
		t.Errorf("Suffix = %q, want \"s.r.o.\"", im.Suffix)
	}
	if im.Country != "CZ" {
		t.Errorf("Country = %q, want CZ", im.Country)
	}
	const wantRegister = "IČO 247 17 509"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty (known, honestly-documented gap this round — see doc comment above)", im.Address)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Czech ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") {
		t.Errorf("FieldsFound = %v, want legal_name present", im.FieldsFound)
	}
}

// TestICOIdentifierChecksum is a direct unit test of the new czICOValid
// wiring: the real IČO found during this round's search must pass, and a
// flipped last digit must fail.
func TestICOIdentifierChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"onlineshop.cz real IČO (3-2-3 grouped)", "IČO: 247 17 509", checksumValid},
		{"same IČO, ungrouped", "IČO: 24717509", checksumValid},
		{"IČ label synonym (no trailing O)", "IČ: 24717509", checksumValid},
		{"flipped last digit must fail", "IČO: 24717508", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("IČO", tc.raw, "CZ"); got != tc.want {
				t.Errorf("validateIdentifier(IČO, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestTagBoundaryPunctCollapsesOnlyAfterLetter is a direct unit test of the
// stripTagsLines fix: a tag boundary immediately followed by "."/"," must
// collapse when a LETTER precedes it (the real CZ suffix-splitting bug),
// but must NOT collapse when a DIGIT precedes it (the real ES phone-number
// regression this round's first attempt caused — see the fixture's own doc
// comment above and tagBoundaryPunctRE's doc comment in imprint_name.go).
func TestTagBoundaryPunctCollapsesOnlyAfterLetter(t *testing.T) {
	letterCase := stripTagsLines(`<p><strong>SHOP TRADING, s.r.o</strong>., se sídlem Praha</p>`)
	if want := "s.r.o., se sídlem Praha"; !strings.Contains(letterCase, want) {
		t.Errorf("stripTagsLines(letter-preceded) = %q, want it to contain %q (newline must collapse)", letterCase, want)
	}

	digitCase := stripTagsLines(`<p>Tel: <a href="tel:+34932681010">932681010</a>. Para más formas de contacto</p>`)
	if strings.Contains(digitCase, "932681010. Para") {
		t.Errorf("stripTagsLines(digit-preceded) = %q, must NOT collapse the newline after a digit (would regress the eurostarshotels.com ES fixture)", digitCase)
	}
}
