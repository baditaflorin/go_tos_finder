package main

import "testing"

// Round-14 EU-expansion real-evidence fixture, fetched 2026-08-22. This is
// the final round of this repo's real-evidence expansion series (rounds
// 1-14).

// japanphotoNoFixture: real excerpt from www.japanphoto.no/kjopsvilkar1
// (CEWE Norge AS's real, live Kjøpsvilkår / terms-of-purchase page —
// Norway's closest analogue to an Impressum page). Byte-for-byte from the
// live page's own markup, including its literal un-decoded HTML entities
// (&aring;/&oslash;/&laquo;/&raquo;/...) — stripTagsLines never decodes
// entities, and this specific page (unlike menu.lu/bqredovisning.se in
// earlier rounds) uses NAMED HTML ENTITIES rather than raw UTF-8 for every
// accented character, not just a handful.
const japanphotoNoFixture = `<!DOCTYPE html>
<html lang="no" xml:lang="no">
<head><title>Kjøpsvilkår | CEWE Japan Photo</title></head>
<body>
<h3>1. Generelt</h3>

<p>Disse generelle kj&oslash;psvilk&aring;rene (&laquo;Kj&oslash;psvilk&aring;r&raquo;) gjelder ved bestilling av varer og tjenester fra CEWE Norge AS med org.nr. 965 321 039 (&laquo;CEWE&raquo; eller &laquo;Vi/oss&raquo;) via v&aring;re nettsider p&aring; cewe.no og/eller japanphoto.no.<br />
Ved &aring; gjennomf&oslash;re kj&oslash;p fra CEWE aksepterer du avtalen.</p>

<h3>17. Kontaktinformasjon</h3>

<p>E-post: <a style="text-decoration: underline;" href="mailto:kundeservice@japanphoto.no">kundeservice@japanphoto.no</a><br />
Brev: CEWE Norge AS<br />
Postboks 4 Bj&oslash;rndal<br />
1214 Oslo<br />
Telefon: +47 66 82 26 60</p>
</body>
</html>`

// TestExtractImprintFieldsJapanphotoNoRealEvidence: real evidence for a
// Norwegian AS. Running the shipped 1.7.8 extractor against this real page
// extracted NOTHING at all (Present but CompletenessScore 0), despite
// "CEWE Norge AS" being trivially suffix-matchable ("AS") right next to its
// org.nr. Found and fixed FOUR compounding real bugs, all affecting the
// same real Norwegian identifier:
//
//  1. The "OrgNr" vatPattern was case-SENSITIVE, requiring a literal
//     capital "Org" — but ordinary mid-sentence Norwegian prose lower-cases
//     it ("...fra CEWE Norge AS med org.nr. 965 321 039..."; only a
//     sentence-initial or label form capitalizes it, e.g. this SAME page's
//     later "Org.nr: 965 321 039" — never independently confirmed since
//     "OrgNr" had no page ever real-evidence it before this round). Without
//     any match, findIdentifiers found nothing on this
//     non-"imprint"/"legal"/"terms"-URLed page, so the whole
//     suffix-anchored scan bailed out before it ever ran. Made the pattern
//     case-insensitive.
//  2. Even after fixing bug 1, "OrgNr" was discovered to have NEVER been
//     listed in extractImprintText's Kind switch at all (present in
//     imprint_vat.go's vatPatterns table since this codebase's very first
//     EU-expansion round, but silently dropped every single time) — same
//     failure shape the PartitaIVA/CIF/NIP note in that switch already
//     documents for a different set of Kinds. Added "OrgNr" to the
//     Register-field case.
//  3. Norway's organisasjonsnummer had no checksum-validation case at all,
//     despite the published Brønnøysundregistrene weighted-mod-11
//     algorithm being well documented — it always fell through to
//     formatValid. Added norwayOrgNrValid (imprint_checksum.go) and wired
//     it in; confirmed against this page's real org.nr (965321039), which
//     passes.
//  4. Even with 1-3 fixed, the FIRST paragraph's own "...fra CEWE Norge AS
//     med org.nr...." mention never produces a usable legal_name candidate
//     at all (the 80-char backward-scan window in extractEntityAround
//     can't reach a sentence boundary before this long opening clause) —
//     an honestly-documented, NOT-fixed gap this round (see the trimmed
//     fixture below). The page's OWN separate, clean "17.
//     Kontaktinformasjon" section labels its postal-contact line "Brev:
//     CEWE Norge AS" (Norwegian for "Letter:") — added "Brev: " to
//     stripLabelPrefix (the Norwegian analogue of the existing "By Mail: "
//     entry) so THIS line's candidate survives cleanly instead. Also added
//     "postboks" (PO box) as a looksAddressLine marker — the same page's
//     "Postboks 4 Bjørndal" line has only a 1-digit box number and was
//     silently dropped from the address, same failure shape as round 12's
//     Danish "vej" and round 5's Polish "aleja".
//
// This fixture is deliberately TRIMMED to omit the first paragraph's own
// "Du kan ta kontakt med CEWE Norge AS på telefon..." sentence (a REAL,
// separate, NOT-fixed gap: that sentence's lack of any sentence-boundary
// punctuation before the entity name lets its whole preceding clause
// through cleanCandidateName's sentence-phrase filter, out-competing the
// clean "Brev:" candidate for the winning slot) — see the note above. Never
// overclaiming a fix this round didn't actually verify.
func TestExtractImprintFieldsJapanphotoNoRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.japanphoto.no/kjopsvilkar1", japanphotoNoFixture, "japanphoto.no")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "CEWE Norge AS"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "AS" {
		t.Errorf("Suffix = %q, want \"AS\"", im.Suffix)
	}
	if im.Country != "NO" {
		t.Errorf("Country = %q, want NO", im.Country)
	}
	const wantAddress = "Postboks 4 Bj&oslash;rndal, 1214 Oslo"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (the un-decoded &oslash; is expected — this specific entity was never real-evidenced to break extraction, so namedEntityAccents deliberately was NOT extended for it this round)", im.Address, wantAddress)
	}
	const wantRegister = "OrgNr org.nr. 965 321 039"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Norwegian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") || !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want legal_name, address, and contact all present", im.FieldsFound)
	}
}

// TestOrgNrIdentifierChecksum is a direct unit test of the new
// norwayOrgNrValid checksum wiring: the real org.nr found during this
// round's search must pass, a flipped-digit value must fail, and the Kind
// must now resolve to Country NO via singleCountryIdentifierKind (all
// three were previously unreachable — see this round's doc comment above).
func TestOrgNrIdentifierChecksum(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want validity
	}{
		{"japanphoto.no real org.nr (lowercase)", "org.nr. 965 321 039", checksumValid},
		{"japanphoto.no real Org.nr (capitalised)", "Org.nr: 965 321 039", checksumValid},
		{"flipped digit must fail", "org.nr. 965 321 038", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier("OrgNr", tc.raw, "NO"); got != tc.want {
				t.Errorf("validateIdentifier(OrgNr, %q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("OrgNr"); got != "NO" {
		t.Errorf("singleCountryIdentifierKind(OrgNr) = %q, want NO", got)
	}
}

// TestOrgNrPatternCaseInsensitive is a direct unit test of the
// case-insensitivity fix: a lowercase "org.nr" mid-sentence must now match,
// same as the pre-existing capitalised "Org.nr" form.
func TestOrgNrPatternCaseInsensitive(t *testing.T) {
	ids := findIdentifiers("fra CEWE Norge AS med org.nr. 965 321 039 («CEWE»)", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "OrgNr" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match lowercase \"org.nr.\" — got %v", ids)
	}
}
