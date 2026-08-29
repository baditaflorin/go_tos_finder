package main

import "testing"

// Round-29 real-evidence fixtures, fetched 2026-08-29, part of the
// domainscope register-number-extraction improvement pass: a direct
// read-only audit of prod found essentially zero UK or Ireland domains
// with a populated imprint_register_number despite hundreds having a
// detected imprint page, even after round 28's GB case-insensitivity fix
// shipped. These three fixtures are real pages sampled from that audit
// that the shipped (pre-round-29) patterns still missed.

// rboRealText: real body text from rbo.org.uk (Royal Opera House Covent
// Garden Foundation's real terms page), fetched via curl 2026-08-29. A
// real, live UK company whose Companies House number is an unpadded
// 6-digit number — incorporated long before Companies House's modern
// 8-digit zero-padded convention took hold, and the website reflects that,
// not a 7-8 digit run.
const rboRealText = "Royal Opera House Covent Garden Foundation, a charitable company limited by guarantee incorporated in\nEngland and Wales (Company number 480523) Charity Registered (Number 211775)"

// TestFindIdentifiersCompaniesHouseSixDigit: running findIdentifiers
// against rboRealText with the pre-round-29 \d{7,8} pattern matches
// NOTHING (480523 is 6 digits). Fixed by widening to \d{6,8}. The adjacent
// "Charity Registered (Number 211775)" must NOT match — no
// CompaniesHouse-shaped label ("Company No/Number", "Registered ... No/
// Number", "Registration No/Number") precedes "Number 211775", so it's
// correctly excluded regardless of the digit-range widening.
func TestFindIdentifiersCompaniesHouseSixDigit(t *testing.T) {
	hits := findIdentifiers(rboRealText, 20)

	var chHits []identifierHit
	for _, h := range hits {
		if h.Kind == "CompaniesHouse" {
			chHits = append(chHits, h)
		}
	}
	if len(chHits) != 1 {
		t.Fatalf("got %d CompaniesHouse hits, want 1 — hits: %+v", len(chHits), hits)
	}
	if chHits[0].Country != "GB" {
		t.Errorf("hit %+v: Country = %q, want GB", chHits[0], chHits[0].Country)
	}
	if chHits[0].Value != "Company number 480523" {
		t.Errorf("hit Value = %q, want %q", chHits[0].Value, "Company number 480523")
	}
}

// yoplaitRealText: real body text from yoplait.ie's real privacy policy
// (naming Yoplait UK Ltd, a real Companies-House-registered entity —
// despite the .ie TLD, this is a UK/GB entity, not an Irish one), fetched
// via curl 2026-08-29.
const yoplaitRealText = "Your personal data are processed by Yoplait UK Ltd, Simplified Joint Stock company, Company Reg Number 02597128, having its registered office at Harman House, 1 George Street, Uxbridge UB8 1QQ, as well as any other entity forming SODIAAL Group,"

// TestFindIdentifiersCompaniesHouseRegAbbreviation: running findIdentifiers
// against yoplaitRealText with the pre-round-29 pattern matches NOTHING —
// "Company Reg Number" satisfies neither the "Company\s*(?:No|Number)"
// branch (an extra "Reg" token sits between "Company" and "Number") nor
// the "Registration\s+(?:No|Number)" branch (the label is the abbreviation
// "Reg", not the full word "Registration"). Fixed by accepting an optional
// "Reg(?:istration)?" between "Company" and "No"/"Number".
func TestFindIdentifiersCompaniesHouseRegAbbreviation(t *testing.T) {
	hits := findIdentifiers(yoplaitRealText, 20)

	var chHits []identifierHit
	for _, h := range hits {
		if h.Kind == "CompaniesHouse" {
			chHits = append(chHits, h)
		}
	}
	if len(chHits) != 1 {
		t.Fatalf("got %d CompaniesHouse hits, want 1 — hits: %+v", len(chHits), hits)
	}
	if chHits[0].Value != "Company Reg Number 02597128" {
		t.Errorf("hit Value = %q, want %q", chHits[0].Value, "Company Reg Number 02597128")
	}
}

// boiRealText: real body text from boi.com (Bank of Ireland Group plc's
// real corporate-information disclosure), fetched via curl 2026-08-29.
// 593672 is Bank of Ireland Group plc's real CRO (Companies Registration
// Office) number.
const boiRealText = "Bank of Ireland Group plc is a public limited company incorporated in Ireland, with its registered office at 2 College Green, Dublin, D02 VR66 and registered number 593672. Bank of Ireland Group plc, whose shares are listed on the main markets of the Irish Stock Exchange plc and the London Stock Exchange plc, is the holding company of Bank of Ireland."

// TestFindIdentifiersCROBareRegisteredNumber: running findIdentifiers
// against boiRealText with the pre-round-29 pattern matches NOTHING — the
// register number is disclosed with neither a "CRO" nor a "Compan(y|ies)
// Registration" label, just the bare Companies Act-style phrase
// "registered number". Fixed by adding a bare "registered\s+number"
// alternative. The preceding "registered office" on the same real
// sentence must NOT match — it's a different word ("office", not
// "number") right after "registered".
func TestFindIdentifiersCROBareRegisteredNumber(t *testing.T) {
	hits := findIdentifiers(boiRealText, 20)

	var croHits []identifierHit
	for _, h := range hits {
		if h.Kind == "CRO" {
			croHits = append(croHits, h)
		}
	}
	if len(croHits) != 1 {
		t.Fatalf("got %d CRO hits, want 1 — hits: %+v", len(croHits), hits)
	}
	if croHits[0].Country != "IE" {
		t.Errorf("hit %+v: Country = %q, want IE", croHits[0], croHits[0].Country)
	}
	if croHits[0].Value != "registered number 593672" {
		t.Errorf("hit Value = %q, want %q", croHits[0].Value, "registered number 593672")
	}
}
