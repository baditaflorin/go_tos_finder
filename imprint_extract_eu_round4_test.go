package main

import "testing"

// Round-4: dedicated national rulesets for France, Italy, Spain, and the
// Netherlands, once round-1/round-2 real evidence confirmed register-number
// extraction is reliably working for all four. See the Ruleset consts' doc
// comment in imprint.go for the legal citations and scope rationale.

// TestImprintFieldChecklistNewRulesetsRequireRegisterNotResponsiblePerson
// documents the checklist shape for all four new rulesets: base
// (legal_name/address/contact) + register, but NOT responsible_person —
// unlike de_tmg/at_ecg_medieng, none of LCEN/D.Lgs.70/2003/LSSICE/
// Handelsregisterwet require naming an individual.
func TestImprintFieldChecklistNewRulesetsRequireRegisterNotResponsiblePerson(t *testing.T) {
	for _, rs := range []string{RulesetFRLCEN, RulesetITDLgs70, RulesetESLSSICE, RulesetNLHandelsreg} {
		checklist := imprintFieldChecklist(rs)
		if !containsStr(checklist, "register") {
			t.Errorf("%s: checklist = %v, want it to contain \"register\"", rs, checklist)
		}
		if containsStr(checklist, "responsible_person") {
			t.Errorf("%s: checklist = %v, want it NOT to contain \"responsible_person\"", rs, checklist)
		}
		for _, f := range []string{"legal_name", "address", "contact"} {
			if !containsStr(checklist, f) {
				t.Errorf("%s: checklist = %v, want it to contain %q", rs, checklist, f)
			}
		}
	}
}

// TestSingleCountryIdentifierKindOverridesAmbiguousSuffixGuess is a direct
// unit test of the round-4 country-precedence fix, independent of the full
// extraction pipeline: a label-anchored, single-country identifier Kind
// (PartitaIVA/REA → IT, Hoja/CIF → ES, SIRET/SIREN → FR, KvK → NL) must be
// recognised; anything else must return "".
func TestSingleCountryIdentifierKindOverridesAmbiguousSuffixGuess(t *testing.T) {
	cases := []struct {
		kind string
		want string
	}{
		{"PartitaIVA", "IT"},
		{"REA", "IT"},
		{"Hoja", "ES"},
		{"CIF", "ES"},
		{"SIRET", "FR"},
		{"SIREN", "FR"},
		{"KvK", "NL"},
		{"VAT", ""}, // country-prefix-derived instead — handled separately
		{"HRB", ""}, // Germany already resolves correctly without this
		{"unknown", ""},
	}
	for _, c := range cases {
		if got := singleCountryIdentifierKind(c.kind); got != c.want {
			t.Errorf("singleCountryIdentifierKind(%q) = %q, want %q", c.kind, got, c.want)
		}
	}
}

// TestFindIdentifiersSIRETCaseInsensitiveAndAlternateGrouping is a direct
// unit test of the SIRET pattern fix, independent of the full extraction
// pipeline: real evidence (humresto.fr) writes "Siret" (mixed case) with
// its 14 digits grouped 3-3-3-3-2, not the "SIRET" all-caps / 3-3-3-5
// grouping the pattern originally assumed.
func TestFindIdentifiersSIRETCaseInsensitiveAndAlternateGrouping(t *testing.T) {
	text := "N° Siret : 980 184 584 000 12"
	ids := findIdentifiers(text, 5)
	found := false
	for _, id := range ids {
		if id.Kind == "SIRET" {
			found = true
			if id.Validation != string(checksumValid) {
				t.Errorf("SIRET validation = %q, want checksum_valid", id.Validation)
			}
		}
	}
	if !found {
		t.Errorf("findIdentifiers(%q) did not find a SIRET hit, got %+v", text, ids)
	}
}
