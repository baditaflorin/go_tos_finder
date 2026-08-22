package main

import "testing"

// Round-9 EU-expansion real-evidence fixture, fetched 2026-08-22.

// proximaIeFixture: real footer excerpt from proxima.ie/company-registered-address/
// (Proxima Tax Services Ltd, a real Irish company-formation/registered-office
// agent). Byte-for-byte from the live page's Elementor-rendered footer.
const proximaIeFixture = `<!DOCTYPE html>
<html lang="en">
<head><title>Company Registered Address | Proxima</title></head>
<body>
<footer>
<h4>Proxima Tax Services Ltd</h4><p>Unit 3G North Point House<br /> North Point Business Park<br /> Mallow Road, Cork<br /> Ireland</p><p><i class="fa fa-envelope"></i><strong> info@proxima.ie</strong></p><p><i class="fa fa-phone"></i> <strong>021 237 30 87</strong></p>
<div class="elementor-text-editor elementor-clearfix"><p>Company Registration No: 613314<br /> Revenue Tax Agent No: 76472U<br /> CRO Electronic Filing Agent No: 613314C</p></div>
</footer>
</body>
</html>`

// TestExtractImprintFieldsProximaIrishRealEvidence: real evidence for an
// Irish limited company (S.I. No. 68/2003, reg. 6(a) — Ireland's
// transposition of e-Commerce Directive Art. 5 — requires the trade
// register + registration number "where applicable", alongside
// name/address/contact). Running the shipped 1.7.3 extractor against this
// real page surfaced two compounding bugs:
//
//  1. Ireland's CRO (Companies Registration Office) company number had no
//     pattern at all — "Company Registration No: 613314" is only 6 digits,
//     which the existing UK "CompaniesHouse" pattern's \d{7,8} requirement
//     cannot match. Added a dedicated "CRO" vatPattern (imprint_vat.go).
//  2. Even after adding the CRO pattern, Country still came out "GB" instead
//     of "IE": suffixTable (imprint_suffix.go) maps bare "Ltd" only to GB
//     (Ireland's own "Ltd" companies are not modelled there at all — a real,
//     live ambiguity), AND the CRO identifier's country never reached the
//     winning candidate's country-correction loop: the plain-text line
//     "Company Registration No: 613314" itself false-matches suffixTable's
//     low-confidence generic "Company"->US entry (a bare English word, not
//     a real US legal-form suffix) and became its own throwaway candidate
//     sitting at proximity distance ~0 from the identifier — winning the
//     proximity-attachment race in extractImprintCandidates over the real
//     winning candidate ("Proxima Tax Services Ltd") a few hundred bytes
//     away. backfillWinnerIdentifiers (imprint.go) already backfilled the
//     Register STRING onto the winner, but never carried over the
//     underlying identifierHit, so singleCountryIdentifierKind's ground-
//     truth country correction never ran. Fixed by having the Register
//     backfill also copy the matching identifierHit, mirroring the existing
//     VAT backfill just below it.
func TestExtractImprintFieldsProximaIrishRealEvidence(t *testing.T) {
	im := extractImprintFields("https://proxima.ie/company-registered-address/", proximaIeFixture, "proxima.ie")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	if im.LegalName != "Proxima Tax Services Ltd" {
		t.Errorf("LegalName = %q, want %q", im.LegalName, "Proxima Tax Services Ltd")
	}
	if im.Suffix != "Ltd" {
		t.Errorf("Suffix = %q, want \"Ltd\"", im.Suffix)
	}
	if im.Country != "IE" {
		t.Errorf("Country = %q, want IE (the real, live \"Ltd\"-is-GB-or-IE ambiguity must be corrected by the unambiguous domestic CRO register number, not left at the suffix guess)", im.Country)
	}
	const wantRegister = "CRO Company Registration No: 613314"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (Ireland's S.I. 68/2003 reg. 6 register requirement is conditional \"where applicable\", same conditionality as the EU baseline itself — no dedicated Irish ruleset added this round, consistent with round 7/8's Belgium/Portugal discipline)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") || !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact", im.FieldsFound)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100 (eu_baseline: legal_name+address+contact all present)", im.CompletenessScore)
	}
}
