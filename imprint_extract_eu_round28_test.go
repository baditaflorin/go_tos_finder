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
// name candidate forms (a well-structured <footer> with heading + address,
// the same shape as the existing proxima.ie/round-9 fixture).
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

// swetenhamsRealMarkup: byte-for-byte real HTML source from
// swetenhams.co.uk/legal-notices (curl, 2026-08-27) — the page's ACTUAL
// div-based structure (not a footer), with its real grammatically run-on
// legal-name sentence and its real un-decoded "&#160;" NBSP entity.
//
// Running extractImprintFields against this real markup — even with the
// (?i) fix above applied — originally returned LegalName="" and
// Register="", despite findIdentifiers correctly finding both
// CompaniesHouse hits in the same raw text (see
// TestFindIdentifiersCompaniesHouseRealEvidence). Two compounding causes,
// both found and fixed this round:
//
//  1. extractEntityAround's backward scan from the "Limited" suffix
//     correctly stops at the enclosing paragraph boundary, but the whole
//     paragraph text up to that point — "Swetenhams is a trading name of
//     Sequence (UK) Limited" — then fails cleanCandidateName's stop-word
//     gate: the stem "Swetenhams is a trading name of Sequence (UK)"
//     contains two sentenceStopWords hits ("is", "of"), so it's rejected
//     as sentence-like before any strip*Prefix function (stripPersonRole-
//     Prefix, stripAwardPrefix, ...) ever ran — none of the EXISTING
//     strippers recognize a UK "X is a trading name of Y" disclosure, the
//     same class of gap they already close for other preamble shapes.
//     Added stripTradingNamePrefix (imprint_name.go), wired in next to
//     stripPersonRolePrefix.
//  2. Even with that stripper added, it still didn't fire: the real raw
//     markup has a literal, un-decoded "&#160;" between "of" and
//     "Sequence" ("...trading name of&#160;Sequence (UK) Limited...") —
//     stripTagsLines never decodes NBSP-shaped entities (same deliberate
//     non-decoding as "&nbsp;"/"&quot;"/"&copy;" elsewhere in this
//     codebase) — so a plain-space marker string never matched this real
//     page at all despite looking identical when printed. Fixed by
//     matching the marker with a regex whose whitespace class accepts
//     `&#160;`/`&nbsp;` as well as real whitespace between every word.
//
// Follow-up fix (round 29): CompletenessScore originally came out 33 (only
// "legal_name"), not accounting for the address that's present in the same
// real paragraph ("Registered Office is Cumbria House, 16-20 Hockliffe
// Street, ..."), because it's embedded inline in the same run-on line as
// the name rather than on a following line or matching the one then-
// existing same-line address pattern (inlineOfAddressRE's Malta-drafting
// "<name> (<number>) of <address> (\"nickname\")" shape — this page's
// phrasing, "<name> is registered ... under company number <n>,
// Registered Office is <address>.", doesn't match it). A distinct gap in
// address-shape matching, not name-candidate formation. Fixed by adding
// registeredOfficeAddressRE (imprint_name.go) as a same-line fallback in
// extractAddressNearEntity, tried when inlineOfAddressRE doesn't match.
//
// CompletenessScore now reaches 66, not 100: GB uses the eu_baseline
// ruleset (legal_name + address + contact). This real page's real static
// content never discloses a phone number or email address anywhere (re-
// verified live, 2026-08-27: the only "tel"/"phone" hits on the actual
// page are JS callback-form plumbing, not a disclosed contact value), so
// "contact" is correctly and permanently missing for this page — 66/100 is
// the accurate ceiling, not a remaining bug.
const swetenhamsRealMarkup = `<!DOCTYPE html>
<html lang="en-GB">
<head><title>Legal Notices | Swetenhams</title></head>
<body>
  <div class="cms-page-wrapper">
  <div class="container">
    <div class="campaign--container--wrapper container-content">
      <div class="campaign--container">
        <div class="header-barcode"></div>
        <div class="campaign--content">
          <div class="campaign--title">legal</div>
          <h1>notices</h1>
          <hr>
          <img src="" class="campaign--image">
          <p>Registered in England and Wales<br />
Registration number: 4268443<br />
Registered Office: Cumbria House, 16-20 Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7 1GN<br />
Data Protection Registration Number: Z8920800<br />
Financial Services Register number: 302221&#160;<span style="font-size:1.6rem">&#160;</span></p>

<p>Swetenhams is a trading name of&#160;Sequence (UK) Limited is registered in England and Wales under company number 4268443, Registered Office is Cumbria House, 16-20 Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7 1GN. &#160;VAT Registration Number is 500 2481 05. &#160;</p>
</div>
</div>
</div>
</div>
</div>
</body>
</html>`

func TestExtractImprintFieldsSwetenhamsRealMarkupEndToEnd(t *testing.T) {
	im := extractImprintFields("https://www.swetenhams.co.uk/legal-notices", swetenhamsRealMarkup, "swetenhams.co.uk")

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
	const wantAddress = "Cumbria House, 16-20 Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7 1GN"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q", im.Address, wantAddress)
	}
	// 66, not 100: "contact" is correctly missing — this real page never
	// discloses a phone number or email address at all. See the doc
	// comment above swetenhamsRealMarkup for why 100 isn't the right
	// target here.
	if im.CompletenessScore != 66 {
		t.Errorf("CompletenessScore = %d, want 66 — FieldsFound: %v, FieldsMissing: %v", im.CompletenessScore, im.FieldsFound, im.FieldsMissing)
	}
}
