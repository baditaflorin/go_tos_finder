package main

import "testing"

// Round-28 EU-expansion real-evidence fixture, fetched 2026-08-27.
//
// The UK's "CompaniesHouse" register-number pattern (imprint_vat.go) has
// been in this codebase since the very first structured-imprint-extraction
// commit — but unlike every other country's pattern in that file (each
// carries a "real evidence: domain's real page" citation), GB was never
// actually validated against a real UK page.
//
// swetenhamsRealText: byte-for-byte real body text from
// swetenhams.co.uk/legal-notices (Sequence (UK) Limited, a real UK estate
// agency trading as "Swetenhams"), fetched via curl 2026-08-27.
const swetenhamsRealText = "Registered in England and Wales\nRegistration number: 4268443\nRegistered Office: Cumbria House, 16-20 Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7 1GN\nData Protection Registration Number: Z8920800\n\nSwetenhams is a trading name of Sequence (UK) Limited is registered in England and Wales under company number 4268443, Registered Office is Cumbria House, 16-20 Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7 1GN.  VAT Registration Number is 500 2481 05."

// TestFindIdentifiersCompaniesHouseRealEvidence: running findIdentifiers
// against this real text with the shipped pattern (no `(?i)` flag) matches
// NOTHING — verified in isolation first (regexp.MustCompile without vs.
// with `(?i)` against these exact sentences: 0 matches vs. 2 correct
// matches). Root cause: the pattern requires literal-case "No"/"Number",
// but this real page — like ordinary prose generally — writes lowercase
// "Registration number:" and "company number 4268443". Same class of bug
// already fixed for Norway's OrgNr and others in this file (case-sensitive
// patterns silently missing real lowercase prose). Fixed by adding `(?i)`.
// No new false positives introduced: the VAT line ("VAT Registration
// Number is 500 2481 05") and the Data Protection Registration Number
// line ("...Z8920800") on the same real page correctly still don't match
// — the VAT digits are space-grouped rather than one 7-8-digit run, and
// "Z8920800" starts with a letter, neither shape the `\d{7,8}` capture
// accepts.
func TestFindIdentifiersCompaniesHouseRealEvidence(t *testing.T) {
	hits := findIdentifiers(swetenhamsRealText, 20)

	var chHits []identifierHit
	for _, h := range hits {
		if h.Kind == "CompaniesHouse" {
			chHits = append(chHits, h)
		}
	}
	if len(chHits) != 2 {
		t.Fatalf("got %d CompaniesHouse hits, want 2 (one from each real register-number sentence on the page) — hits: %+v", len(chHits), hits)
	}
	for _, h := range chHits {
		if h.Country != "GB" {
			t.Errorf("hit %+v: Country = %q, want GB", h, h.Country)
		}
	}
	wantValues := map[string]bool{"Registration number: 4268443": false, "company number 4268443": false}
	for _, h := range chHits {
		if _, ok := wantValues[h.Value]; ok {
			wantValues[h.Value] = true
		}
	}
	for v, found := range wantValues {
		if !found {
			t.Errorf("expected a CompaniesHouse hit with Value %q, got none — hits: %+v", v, chHits)
		}
	}
}

// TestExtractImprintFieldsCompaniesHouseCleanFooter: end-to-end proof that
// a fixed identifierHit reaches the final Imprint.Register field once a
// name candidate actually forms (a well-structured <footer> with heading +
// address, the same shape as the existing proxima.ie/round-9 fixture).
// Real-world register-number VALUES are taken from swetenhams.co.uk (see
// above); the surrounding HTML structure is reconstructed rather than
// byte-for-byte, because swetenhams.co.uk's own real markup exposes a
// SEPARATE, pre-existing bug (see below) that prevents this specific page
// from forming ANY name candidate at all — orthogonal to the
// case-sensitivity fix this round targets, so it's flagged, not fixed,
// here.
const chFooterFixture = `<!DOCTYPE html>
<html lang="en-GB">
<head><title>Legal Notices | Swetenhams</title></head>
<body>
<footer>
<h4>Sequence (UK) Limited</h4>
<p>Cumbria House, 16-20 Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7 1GN<br />
Registered in England and Wales<br />
Registration number: 4268443</p>
</footer>
</body>
</html>`

func TestExtractImprintFieldsCompaniesHouseCleanFooter(t *testing.T) {
	im := extractImprintFields("https://www.swetenhams.co.uk/legal-notices", chFooterFixture, "swetenhams.co.uk")

	if im.LegalName != "Sequence (UK) Limited" {
		t.Errorf("LegalName = %q, want %q", im.LegalName, "Sequence (UK) Limited")
	}
	if im.Suffix != "Limited" {
		t.Errorf("Suffix = %q, want \"Limited\"", im.Suffix)
	}
	if im.Country != "GB" {
		t.Errorf("Country = %q, want GB", im.Country)
	}
	const wantRegister = "CompaniesHouse Registration number: 4268443"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
}

// NOTE — separate, pre-existing bug found but NOT fixed this round:
// running extractImprintFields against swetenhams.co.uk's REAL raw markup
// (the actual page structure, not the reconstructed clean footer above)
// returns LegalName="" and Register="" even with this round's (?i) fix
// applied — despite findIdentifiers correctly finding both CompaniesHouse
// hits (see TestFindIdentifiersCompaniesHouseRealEvidence above). No name
// candidate forms at all, so there's no "winner" for
// backfillWinnerIdentifiers to attach the identifier to. The real page's
// legal-name sentence is grammatically run-on ("Swetenhams is a trading
// name of Sequence (UK) Limited is registered in England and Wales..." —
// missing a "which" before the second "is"), which plausibly confuses
// candidate formation, but this wasn't root-caused before time ran out
// this round — flagged for follow-up rather than guessed at.
