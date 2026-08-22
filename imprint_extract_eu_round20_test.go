package main

import "testing"

// Round-20 EU-expansion real-evidence fixture, fetched 2026-08-23: Croatia.
// Real excerpt from https://mako.hr/opci-uvjeti-koristenja-web-trgovine/
// (Mako d.o.o. — a real, live Croatian retailer's Opći uvjeti page) —
// byte-for-byte from the live page's own markup, including a literal "•"
// (U+2022 BULLET) list-marker character used as plain text.
//
// Running the shipped 1.7.14 extractor against this real page extracted
// NOTHING at all (CompletenessScore 0) — suffixTable had zero Croatian
// entries of any kind (Latin or otherwise), and no OIB/MBS identifier
// patterns existed. Found and fixed four compounding real bugs:
//
//  1. No Croatian suffix entries at all. Added "d.o.o." (real-evidence-
//     confirmed) plus "j.d.o.o."/"d.d." (added alongside it on the same
//     well-established-sibling-forms basis rounds 18-19 used for Greece/
//     Bulgaria). Known, accepted collision risk documented at the
//     suffixTable entry: "d.o.o." is shared letter-for-letter by every
//     former-Yugoslav-republic legal system, including Slovenia (a later
//     target in this series) — the same class of risk already tolerated
//     for the existing "S.A."(FR/RO)/"S.R.L."(IT/RO) collisions, self-
//     healed by ground-truth identifier evidence when present.
//  2. Croatia's OIB (personal/legal identification number) and MBS
//     (court-register entry number) had no vatPatterns entries at all.
//     Added both — "OIB" wired as VAT-equivalent (its 11-digit body IS
//     the same number the "HR"-prefixed EU VAT pattern matches, same
//     architecture as ЕИК/ΑΦΜ/Adószám), "MBS" wired as Register-only —
//     plus a new hrOIBValid ISO 7064 MOD 11-10 checksum, confirmed
//     against this page's real value (31448356613). Both use plain ASCII
//     Latin letters, so (unlike rounds 18-19's Greek/Bulgarian patterns)
//     no leading-`\b` RE2 gotcha applied here.
//  3. A literal "•" bullet-marker character (plain text in the source,
//     not CSS-generated) precedes each defined-term line on this real
//     page ("<br />\n• Mako znači Mako d.o.o., sa sjedištem..."), and
//     extractEntityAround's backward-scan had no stop condition for it —
//     unlike '\n'/'\r'/'|', which already act as hard stops for the same
//     structural reason. Without this fix, the candidate captured
//     "• Mako znači Mako d.o.o." (bullet plus the whole preceding
//     defining clause) instead of just the real name. Added "•" as a
//     stop character alongside the existing line-break stops.
//  4. Even with the bullet fixed, the defining clause itself ("Mako
//     znači Mako d.o.o." — "[Term] means [Full legal name]") still
//     duplicated the short name into the candidate. Added " znači "
//     (Croatian "means") to trimAtConjunction's marker list — not
//     grammatically a conjunction like its siblings ("and"/"y"/"et"/
//     "und"/"e"), but the same "keep only the text after the last marker,
//     right before the suffix" mechanism cleanly extracts the real name.
//
// One real, honestly-documented gap NOT fixed this round: Address stays
// empty on this specific page — the real address ("sa sjedištem u
// Osijeku, Belomanastirska 47") sits on the SAME line as the entity name
// and its MBS/OIB clause (no further line break splits them), and
// extractAddressNearEntity only ever scans FORWARD from the line AFTER
// the entity's own line — same class of gap as round 15's Romanian
// same-line address, honestly left unfixed there too.
const makoHrFixture = `<!DOCTYPE html>
<html lang="hr">
<head><title>Opći uvjeti</title></head>
<body>
<p>• Mako znači Mako d.o.o., sa sjedištem u Osijeku, Belomanastirska 47, upisano u sudski registar Trgovačkog suda u Osijeku pod matičnim brojem subjekta upisa MBS: 030013817, OIB: 31448356613 (dalje u tekstu: „Mako" ili „Prodavatelj").<br />
1.4. Pristupom web stranicama putem odgovarajućeg tehničkog sredstva pristupa i njihovim korištenjem svaki korisnik se obvezuje poštivati ove Opće uvjete.</p>
</body>
</html>`

func TestExtractImprintFieldsMakoHrRealEvidence(t *testing.T) {
	im := extractImprintFields("https://mako.hr/opci-uvjeti-koristenja-web-trgovine/", makoHrFixture, "mako.hr")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Mako d.o.o."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (must NOT include the leading \"•\" bullet or the \"znači\" defining clause)", im.LegalName, wantName)
	}
	if im.Suffix != "d.o.o." {
		t.Errorf("Suffix = %q, want \"d.o.o.\"", im.Suffix)
	}
	if im.Country != "HR" {
		t.Errorf("Country = %q, want HR", im.Country)
	}
	const wantRegister = "MBS 030013817"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.VAT != "31448356613" {
		t.Errorf("VAT = %q, want 31448356613 (the real OIB)", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty (known, honestly-documented gap this round — see doc comment above)", im.Address)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Croatian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") {
		t.Errorf("FieldsFound = %v, want legal_name present", im.FieldsFound)
	}
}

// TestHrOIBChecksum is a direct unit test of the new hrOIBValid wiring: the
// real OIB found during this round's search must pass (via both the
// domestic "OIB" label form and the "HR"-prefixed VAT form), and a flipped
// last digit must fail.
func TestHrOIBChecksum(t *testing.T) {
	cases := []struct {
		name string
		kind string
		raw  string
		want validity
	}{
		{"mako.hr real OIB (domestic label)", "OIB", "OIB: 31448356613", checksumValid},
		{"mako.hr real VAT (HR-prefixed, same body)", "VAT", "HR31448356613", checksumValid},
		{"flipped last digit must fail", "OIB", "OIB: 31448356612", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier(tc.kind, tc.raw, "HR"); got != tc.want {
				t.Errorf("validateIdentifier(%s, %q) = %q, want %q", tc.kind, tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("OIB"); got != "HR" {
		t.Errorf("singleCountryIdentifierKind(OIB) = %q, want HR", got)
	}
	if got := singleCountryIdentifierKind("MBS"); got != "HR" {
		t.Errorf("singleCountryIdentifierKind(MBS) = %q, want HR", got)
	}
}

// TestBulletMarkerStopsEntityScan is a direct unit test of the
// extractEntityAround fix: a literal "•" character must stop the backward
// scan the same way a newline does.
func TestBulletMarkerStopsEntityScan(t *testing.T) {
	name := extractEntityAround("unrelated prose\n• Mako znači Mako d.o.o.", "d.o.o.")
	const want = "Mako d.o.o."
	if name != want {
		t.Errorf("extractEntityAround = %q, want %q", name, want)
	}
}
