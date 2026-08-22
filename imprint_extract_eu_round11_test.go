package main

import "testing"

// Round-11 EU-expansion real-evidence fixture, fetched 2026-08-22.

// bqRedovisningSeFixture: real excerpt from bqredovisning.se/integritetspolicy/
// (BQ Redovisning & Rådgivning AB, a real Swedish SRF-authorised accounting
// firm's GDPR privacy-policy page — Sweden's closest analogue to an
// Impressum/aviso-legal page, since Sweden has no dedicated "imprint" legal
// tradition). Byte-for-byte from the live page, including its literal
// ampersand entity.
const bqRedovisningSeFixture = `<!DOCTYPE html>
<html lang="sv">
<head><title>Integritetspolicy - BQ Redovisning</title></head>
<body>
<article>
<h1>Integritetspolicy</h1> <p>BQ Redovisning &amp; Rådgivning AB (org.nr 559086-2809) ansvarar för behandlingen av dina personuppgifter i enlighet med EU:s dataskyddsförordning (GDPR).</p> <h3>Vilka uppgifter samlar vi in?</h3> <p>När du fyller i vårt kontaktformulär samlar vi in: namn, e-postadress, telefonnummer, företagsnamn och eventuellt meddelande.</p>
</article>
</body>
</html>`

// TestExtractImprintFieldsBqRedovisningSwedishRealEvidence: real evidence
// for a Swedish AB (Lag (2002:562) om elektronisk handel och andra
// informationssamhällets tjänster, 8 § — Sweden's e-Commerce Directive
// transposition — requires name, address, email, and "where applicable"
// organisationsnummer/VAT). Running the shipped 1.7.5 extractor against
// this real page extracted NOTHING at all: CompletenessScore 0, no
// legal_name, despite "BQ Redovisning & Rådgivning AB" being trivially
// suffix-matchable ("AB") in isolation. Found and fixed two compounding
// real bugs, plus one adjacent regression the second fix exposed:
//
//  1. The entity-corruption rejection filter (extractImprintText,
//     imprint_jsonld.go) discarded the WHOLE line for containing a literal
//     "&amp;" — but an ampersand is an entirely ordinary company-name
//     character (H&M, Procter & Gamble, ...), not corruption. Extended
//     decodeKnownAccentEntities (added in round 10) to also decode
//     "&amp;" -> "&", so the line survives and both the corruption filter
//     and cleanCandidateName's own entity-residue check see a real "&".
//  2. Sweden's domestic Organisationsnummer (10 digits, hyphenated 6-4 —
//     "org.nr 559086-2809") had no register pattern at all; the existing
//     SE VAT pattern requires the "SE...01" wrapped form, a different
//     shape. Added a dedicated "Organisationsnummer" vatPattern, distinct
//     from Norway's existing "OrgNr" Kind (different digit shape) so
//     singleCountryIdentifierKind attributes it unambiguously to SE.
//  3. Fixing bug 1 exposed a THIRD, pre-existing bug: trimAtConjunction's
//     conjunction list included " & " (never itself real-evidenced — only
//     "and"/"y"/"et"/"und"/"e" have a cited real fixture, YOOX/Meta), which
//     then wrongly truncated the now-surviving candidate name from "BQ
//     Redovisning & Rådgivning AB" down to just "Rådgivning AB". Removed
//     " & " from the conjunction list entirely — no real page has ever
//     evidenced an ampersand joining two DISTINCT entities the way "and"
//     does in the YOOX/Meta fixture, and this real Swedish page shows the
//     far more common case (ampersand as part of ONE company's own name)
//     actively breaks if it stays.
//
// No dedicated Swedish ruleset added: the 8 § register-number requirement
// is conditional ("i förekommande fall" / "where applicable"), the same
// conditionality as the EU baseline itself. This real page also has no
// physical address or contact channel on it at all (it is a privacy
// policy, not a full imprint page) — CompletenessScore reflects that
// honestly rather than overclaiming fields this fixture doesn't carry.
func TestExtractImprintFieldsBqRedovisningSwedishRealEvidence(t *testing.T) {
	im := extractImprintFields("https://bqredovisning.se/integritetspolicy/", bqRedovisningSeFixture, "bqredovisning.se")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "BQ Redovisning & Rådgivning AB"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (the entity-encoded \"&amp;\" must decode to a real \"&\", and trimAtConjunction must NOT truncate at it)", im.LegalName, wantName)
	}
	if im.Suffix != "AB" {
		t.Errorf("Suffix = %q, want \"AB\"", im.Suffix)
	}
	if im.Country != "SE" {
		t.Errorf("Country = %q, want SE", im.Country)
	}
	const wantRegister = "Organisationsnummer org.nr 559086-2809"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (Sweden's 8 § register requirement is conditional \"where applicable\", same as the EU baseline itself — no dedicated Swedish ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") {
		t.Errorf("FieldsFound = %v, want legal_name present", im.FieldsFound)
	}
	if containsStr(im.FieldsFound, "address") || containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want address/contact ABSENT — this real page genuinely has neither", im.FieldsFound)
	}
}

// TestTrimAtConjunctionKeepsAmpersandName is a direct unit test of the
// trimAtConjunction fix, independent of the full extraction pipeline: a
// name containing " & " immediately before the suffix must be kept whole,
// while the original real "and"-joins-two-entities case (YOOX/Meta) this
// function exists for must still trim correctly.
func TestTrimAtConjunctionKeepsAmpersandName(t *testing.T) {
	if got := trimAtConjunction("BQ Redovisning & Rådgivning AB", "AB"); got != "BQ Redovisning & Rådgivning AB" {
		t.Errorf("trimAtConjunction(ampersand name) = %q, want it unchanged", got)
	}
	if got := trimAtConjunction("YOOX and Meta Platforms Ireland Limited", "Limited"); got != "Meta Platforms Ireland Limited" {
		t.Errorf("trimAtConjunction(and-joined entities) = %q, want %q", got, "Meta Platforms Ireland Limited")
	}
}
