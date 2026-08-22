package main

import (
	"strings"
	"testing"
)

// These fixtures are real excerpts (not synthetic) pulled verbatim from
// live pages fetched and manually compared against this service's output
// during a real-evidence verification pass: hotelrose.at/impressum_de.html
// (a genuine, compliant Austrian sole-trader Impressum) and
// brucebetcasino-at.com's homepage (which uses Cloudflare's email
// obfuscation). Following this repo's inline-backtick fixture convention
// (imprint_extract_test.go) — trimmed to the relevant excerpt rather than
// the full 60-100KB page, but otherwise byte-for-byte as published.

// hotelRoseImpressumFixture is a trimmed version of the real
// hotelrose.at/impressum_de.html: the real JSON-LD block (schema.org
// "Hotel" — Bug 1) plus the real visible-text imprint paragraph (a sole
// trader with no legal-form suffix, disclosed via "Firmenname:" — Bug 2;
// and no separately-labelled responsible-person line — Bug 3; and a
// register court named with no FN number — the register judgment call).
// Dropped from the real page: styling/scripts, the OS-dispute-resolution
// boilerplate, and image-copyright paragraphs — none of that affects
// imprint field extraction.
const hotelRoseImpressumFixture = `<!DOCTYPE html>
<html lang="de">
<head>
<title>Impressum</title>
<script type="application/ld+json">
{
    "@context": "http://schema.org",
    "@type": "Hotel",
    "name": "Aktivhotel zur Rose - Franz Holzmann",
    "address": {
        "@type": "PostalAddress",
        "addressLocality": "Steinach am Brenner",
        "postalCode": "6150",
        "streetAddress": "Brenner Straße 30",
        "addressCountry": "A"
    },
    "priceRange": "€€-€€€",
    "telephone": "+43 5272 6221",
    "email": "info@hotelrose.at",
    "url": "."
}
</script>
</head>
<body>
<h1>Impressum </h1>
<p>Firmenname: Aktivhotel Zur Rose<br>Familie Franz Holzmann<br><br><strong>Adresse:</strong><br>Brennerstraße 30<br>6150 Steinach in Tirol, Österreich<br>Tel.: 0043 5272 / 62 21, Fax: 0043 5272 / 22 24<br>info@hotelrose.at, <a href=""></a>www.hotelrose.at<br><br><strong>UID-Nr.: </strong>ATU43951103</p><p><strong>Behörde gem. ECG </strong>(E-Commerce Gesetz): Bezirkshauptmannschaft Innsbruck Land</p><p><strong>Firmengericht: </strong>Landesgericht Innsbruck</p><p><strong>Informationspflicht lt. E-Commerce-Gesetz:</strong> <a href="http://www.wko.at">www.wko.at</a></p>
</body>
</html>`

// hotelRoseJSONLD is just the JSON-LD block from hotelRoseImpressumFixture,
// isolated for a direct parseLDBlob unit test (Bug 1) independent of the
// full extraction pipeline / best-candidate selection.
const hotelRoseJSONLD = `{
    "@context": "http://schema.org",
    "@type": "Hotel",
    "name": "Aktivhotel zur Rose - Franz Holzmann",
    "address": {
        "@type": "PostalAddress",
        "addressLocality": "Steinach am Brenner",
        "postalCode": "6150",
        "streetAddress": "Brenner Straße 30",
        "addressCountry": "A"
    },
    "priceRange": "€€-€€€",
    "telephone": "+43 5272 6221",
    "email": "info@hotelrose.at",
    "url": "."
}`

// hotelRoseImprintParagraphOnly is the same real visible-text paragraph as
// above, but with NO JSON-LD anywhere on the page — isolates the Bug 2
// label-based fallback (extractImprintText) from the Bug 1 JSON-LD path,
// confirming the plain-text label alone finds the sole trader's name and
// its nearby VAT ID when structured data isn't present at all.
const hotelRoseImprintParagraphOnly = `<!DOCTYPE html>
<html lang="de">
<head><title>Impressum</title></head>
<body>
<h1>Impressum </h1>
<p>Firmenname: Aktivhotel Zur Rose<br>Familie Franz Holzmann<br><br><strong>Adresse:</strong><br>Brennerstraße 30<br>6150 Steinach in Tirol, Österreich<br>Tel.: 0043 5272 / 62 21, Fax: 0043 5272 / 22 24<br>info@hotelrose.at, <a href=""></a>www.hotelrose.at<br><br><strong>UID-Nr.: </strong>ATU43951103</p><p><strong>Firmengericht: </strong>Landesgericht Innsbruck</p>
</body>
</html>`

// atpkitzPersonAuthorJSONLD is a real, trimmed excerpt from
// atpkitz_at_privacy_jsonld_person_author.html: the @graph's first entry
// (a WebPage whose "author" is a schema.org Person), verbatim, with the
// later @graph entries (ItemList/FAQPage — irrelevant to this check)
// dropped to keep the JSON self-contained and valid. This is a
// false-positive stress test: "Friedrich Winter" is a content byline, not
// a legal controller identity, and must never be picked up as an imprint
// candidate.
const atpkitzPersonAuthorJSONLD = `{
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "WebPage",
      "@id": "https://www.atpkitz.at/",
      "url": "https://www.atpkitz.at/",
      "name": "Wettanbieter ohne LUGAS – Welcher ist der beste?",
      "description": "Was sind Wettanbieter ohne LUGAS und warum sind sie für österreichische Spieler interessant? Wettanbieter ohne LUGAS erfreuen sich bei österreichischen.",
      "inLanguage": "de",
      "author": {
        "@type": "Person",
        "name": "Friedrich Winter"
      }
    }
  ]
}`

// bruceBetCasinoCFEmailSnippet is the real Cloudflare email-obfuscation
// anchor markup from brucebetcasino_at_com_homepage_no_imprint_cf_email.html
// (a nearby phone-number anchor was trimmed out to isolate the
// email-obfuscation mechanism specifically). Both the /cdn-cgi/l/
// email-protection href and the data-cfemail span attribute encode the
// SAME real address (confirmed by decoding both by hand: they agree),
// which is completely invisible to imprintEmailRE — that's the bug.
const bruceBetCasinoCFEmailSnippet = `<a class="x0754b18" href="/cdn-cgi/l/email-protection#40292e262f002232352325222534232133292e2f6d21346e232f2d"><span class="__cf_email__" data-cfemail="f59c9b939ab597878096909790819694869c9b9ad89481db969a98">[email&#160;protected]</span></a>`

// --- Bug 1: JSON-LD type-matching too narrow (schema.org Hotel etc.) -----

// TestParseLDBlobRecognisesSchemaOrgHotel confirms the real hotelrose.at
// JSON-LD block (@type: "Hotel") now yields an Organization candidate.
// Before the fix, isOrgType's exact-match list didn't include "Hotel" (a
// LocalBusiness subtype under schema.org's real hierarchy), so this real
// page's JSON-LD silently produced zero candidates.
func TestParseLDBlobRecognisesSchemaOrgHotel(t *testing.T) {
	cands := parseLDBlob(hotelRoseJSONLD, "https://www.hotelrose.at/impressum_de.html")
	if len(cands) != 1 {
		t.Fatalf("parseLDBlob returned %d candidates, want 1 (schema.org \"Hotel\" must be recognised as an Organization subtype): %+v", len(cands), cands)
	}
	c := cands[0]
	const wantName = "Aktivhotel zur Rose - Franz Holzmann"
	if c.Name != wantName {
		t.Errorf("Name = %q, want %q", c.Name, wantName)
	}
	const wantAddr = "Brenner Straße 30, 6150, Steinach am Brenner, A"
	if c.Address != wantAddr {
		t.Errorf("Address = %q, want %q", c.Address, wantAddr)
	}
	if c.Source != "json_ld" {
		t.Errorf("Source = %q, want json_ld", c.Source)
	}
	// imprintCandidate has no dedicated telephone/email slot (those feed
	// hasImprintContact separately, over the raw page body) — this is a
	// fixture sanity check confirming the real JSON-LD text this test
	// exercises genuinely does carry both, per the real evidence.
	if !strings.Contains(hotelRoseJSONLD, `"telephone": "+43 5272 6221"`) {
		t.Fatal("fixture sanity check failed: telephone missing from the real JSON-LD text")
	}
	if !strings.Contains(hotelRoseJSONLD, `"email": "info@hotelrose.at"`) {
		t.Fatal("fixture sanity check failed: email missing from the real JSON-LD text")
	}
}

// TestIsOrgTypeSchemaOrgLocalBusinessSubtypes exercises the broadened
// accepted-type set directly: common real-world LocalBusiness subtypes
// (including ones explicitly called out in this fix's design — Hotel,
// Restaurant, Attorney, Store, ProfessionalService, MedicalBusiness,
// AutomotiveBusiness, FinancialService, LodgingBusiness,
// HomeAndConstructionBusiness) must now match, while non-organization
// schema.org types (Person, WebPage, and other JSON-LD node types commonly
// seen alongside an Organization block, like ItemList/FAQPage) must still
// be rejected.
func TestIsOrgTypeSchemaOrgLocalBusinessSubtypes(t *testing.T) {
	cases := []struct {
		typ  string
		want bool
	}{
		{"Hotel", true},
		{"Restaurant", true},
		{"Attorney", true},
		{"Store", true},
		{"ProfessionalService", true},
		{"MedicalBusiness", true},
		{"AutomotiveBusiness", true},
		{"FinancialService", true},
		{"FoodEstablishment", true},
		{"LodgingBusiness", true},
		{"HomeAndConstructionBusiness", true},
		{"Dentist", true},
		{"RealEstateAgent", true},
		{"LocalBusiness", true},
		{"Organization", true},
		{"Corporation", true},
		{"Person", false},
		{"WebPage", false},
		{"ItemList", false},
		{"FAQPage", false},
		{"BreadcrumbList", false},
		{"Question", false},
	}
	for _, c := range cases {
		if got := isOrgType(c.typ); got != c.want {
			t.Errorf("isOrgType(%q) = %v, want %v", c.typ, got, c.want)
		}
	}
}

// TestParseLDBlobIgnoresPersonAuthorInGraph is the "use your judgment"
// false-positive stress test: a JSON-LD @graph whose first node is a
// WebPage with a Person "author" (an SEO content byline, not a legal
// controller identity) must not surface any candidate — Person was never
// in isOrgType's accepted set, and parseLDBlob's visit() only recurses
// into "@graph", never into an arbitrary nested field like "author", so
// the Person node is never even inspected. This should already be correct
// by construction; this test locks it down against future changes.
func TestParseLDBlobIgnoresPersonAuthorInGraph(t *testing.T) {
	cands := parseLDBlob(atpkitzPersonAuthorJSONLD, "https://www.atpkitz.at/privacy/")
	if len(cands) != 0 {
		t.Errorf("parseLDBlob returned %d candidates, want 0 — a WebPage.author Person node is a content byline, not a legal controller identity: %+v", len(cands), cands)
	}
}

// --- Bugs 1+2+3 + register judgment call, combined end-to-end ------------

// TestExtractImprintFieldsHotelRoseSoleTraderRealEvidence runs the full
// extraction pipeline against the real hotelrose.at Impressum content and
// checks every field a compliant Austrian sole-trader imprint discloses:
// name + address from JSON-LD (Bug 1), a court-only register mention
// counted as disclosed (register judgment call — no FN number is cited
// anywhere on this real page), and the responsible person satisfied by the
// legal name itself (Bug 3 — this page has no separately-labelled
// Geschäftsführer/verantwortlich-für-den-Inhalt line at all, because a
// sole trader's trading name IS the disclosed natural person).
func TestExtractImprintFieldsHotelRoseSoleTraderRealEvidence(t *testing.T) {
	im := extractImprintFields("https://www.hotelrose.at/impressum_de.html", hotelRoseImpressumFixture, "hotelrose.at")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Aktivhotel zur Rose - Franz Holzmann"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (JSON-LD \"Hotel\" type must now be recognised and win over the lower-ranked imprint_label match)", im.LegalName, wantName)
	}
	if im.Suffix != "" {
		t.Errorf("Suffix = %q, want empty (a sole trader has no legal-form suffix)", im.Suffix)
	}
	const wantAddr = "Brenner Straße 30, 6150, Steinach am Brenner, A"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q", im.Address, wantAddr)
	}
	if im.Country != "AT" {
		t.Errorf("Country = %q, want AT", im.Country)
	}
	if im.Ruleset != RulesetATECGMedienG {
		t.Errorf("Ruleset = %q, want at_ecg_medieng", im.Ruleset)
	}
	if im.ResponsiblePerson != "Franz Holzmann" {
		t.Errorf("ResponsiblePerson = %q, want %q (Bug 3: satisfied by the legal_name itself, no separate label exists on this real page)", im.ResponsiblePerson, "Franz Holzmann")
	}
	if im.Register != "Landesgericht Innsbruck" {
		t.Errorf("Register = %q, want %q (register judgment call: a court-only mention with no FN number still counts)", im.Register, "Landesgericht Innsbruck")
	}
	// Bug 5 (1.5.2): the real page's visible text carries "UID-Nr.:
	// ATU43951103", found by the imprint_label plain-text candidate via
	// proximity attachment — but the winning candidate is JSON-LD ("Hotel"),
	// which carries no vatID field at all. Before the backfill fix, the two
	// candidates' names ("Aktivhotel zur Rose - Franz Holzmann" vs
	// "Aktivhotel Zur Rose") never matched exactly, so mergeImprintCandidates
	// never combined them and this real, present, valid VAT number was
	// silently dropped even though the extractor genuinely found it.
	if im.VAT != "ATU43951103" {
		t.Errorf("VAT = %q, want ATU43951103 (must be backfilled from the losing imprint_label candidate into the winning JSON-LD candidate)", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
	if !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want it to contain vat_valid", im.FieldsFound)
	}
	if !containsStr(im.FieldsFound, "responsible_person") {
		t.Errorf("FieldsFound = %v, want it to contain responsible_person", im.FieldsFound)
	}
	if !containsStr(im.FieldsFound, "register") {
		t.Errorf("FieldsFound = %v, want it to contain register", im.FieldsFound)
	}
	if !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want it to contain contact", im.FieldsFound)
	}
	if len(im.FieldsMissing) != 0 {
		t.Errorf("FieldsMissing = %v, want none", im.FieldsMissing)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}

// --- Bug 2: sole-trader label-based name extraction, isolated ------------

// TestExtractImprintFieldsLabeledNameNoJSONLD isolates Bug 2 from Bug 1:
// with NO JSON-LD anywhere on the page, the plain-text "Firmenname:" label
// alone must still find the sole trader's name, and the nearby VAT ID must
// still get proximity-attached to it exactly as it would for a
// suffix-anchored candidate.
func TestExtractImprintFieldsLabeledNameNoJSONLD(t *testing.T) {
	im := extractImprintFields("https://www.hotelrose.at/impressum_de.html", hotelRoseImprintParagraphOnly, "hotelrose.at")

	const wantName = "Aktivhotel Zur Rose"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (Bug 2 label-based fallback, no JSON-LD present)", im.LegalName, wantName)
	}
	if im.Suffix != "" {
		t.Errorf("Suffix = %q, want empty", im.Suffix)
	}
	if im.VAT != "ATU43951103" {
		t.Errorf("VAT = %q, want ATU43951103 (proximity-attached to the imprint_label candidate)", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
}

// TestExtractImprintTextLabeledNameVariants covers the other label
// spellings named in the bug report (Company name:/Unternehmen:/Inhaber:),
// which the one real fixture only demonstrates "Firmenname:" for, plus the
// "label alone, value on the following line" shape.
func TestExtractImprintTextLabeledNameVariants(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{
			"Firmenname (real evidence label)",
			"<p>Firmenname: Solo Handwerksbetrieb Bauer</p>",
			"Solo Handwerksbetrieb Bauer",
		},
		{
			"Company name",
			"<p>Company name: River Bend Consulting</p>",
			"River Bend Consulting",
		},
		{
			"Unternehmen",
			"<p>Unternehmen: Nordlicht Fotografie</p>",
			"Nordlicht Fotografie",
		},
		{
			"Inhaberin (owner)",
			"<p>Inhaberin: Julia Steinberg</p>",
			"Julia Steinberg",
		},
		{
			"label alone, value on next line",
			"<p>Firmenname:</p><p>Kowalski Gartenbau</p>",
			"Kowalski Gartenbau",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cands := extractImprintText(c.body, "https://example.com/impressum")
			if len(cands) != 1 {
				t.Fatalf("got %d candidates, want 1: %+v", len(cands), cands)
			}
			if cands[0].Name != c.want {
				t.Errorf("Name = %q, want %q", cands[0].Name, c.want)
			}
			if cands[0].Source != "imprint_label" {
				t.Errorf("Source = %q, want imprint_label", cands[0].Source)
			}
		})
	}
}

// TestExtractImprintTextLabeledNameFalsePositiveGuard: a page that merely
// mentions "Firmenname"/"Unternehmen" mid-sentence, with no label
// immediately followed by ": <value>" at the start of a line, must not
// produce a candidate — the false positive this fix must guard against.
func TestExtractImprintTextLabeledNameFalsePositiveGuard(t *testing.T) {
	body := `<p>Häufige Frage: Wie ändere ich meinen Firmenname? Der Firmenname muss beim Handelsregister angemeldet werden, bevor das Unternehmen loslegen kann.</p>`
	cands := extractImprintText(body, "https://example.com/impressum")
	if len(cands) != 0 {
		t.Errorf("extractImprintText returned %d candidates from unanchored prose mentioning \"Firmenname\"/\"Unternehmen\", want 0: %+v", len(cands), cands)
	}
}

// --- Bug 4: Cloudflare email-obfuscation decoding -------------------------

// TestDecodeCFEmailRealBruceBetCasino decodes both of the real hex blobs
// found on brucebetcasino-at.com (the /cdn-cgi/l/email-protection href and
// the data-cfemail span attribute) — different encodings of the SAME real
// address, confirming the XOR algorithm against real, independently
// cross-checked data.
func TestDecodeCFEmailRealBruceBetCasino(t *testing.T) {
	cases := []struct {
		name string
		hex  string
	}{
		{"cdn-cgi/l/email-protection href", "40292e262f002232352325222534232133292e2f6d21346e232f2d"},
		{"data-cfemail span attribute", "f59c9b939ab597878096909790819694869c9b9ad89481db969a98"},
	}
	const want = "info@brucebetcasino-at.com"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decodeCFEmail(c.hex); got != want {
				t.Errorf("decodeCFEmail(%q) = %q, want %q", c.hex, got, want)
			}
		})
	}
}

// TestDecodeCFEmailMalformedInput confirms the decoder degrades safely
// (empty string, never a panic) on odd-length, empty, or non-hex input.
func TestDecodeCFEmailMalformedInput(t *testing.T) {
	cases := []string{"", "z", "zzzz", "abc", "4"}
	for _, h := range cases {
		if got := decodeCFEmail(h); got != "" {
			t.Errorf("decodeCFEmail(%q) = %q, want empty", h, got)
		}
	}
}

// --- Bug 5: VAT dropped when the winning JSON-LD candidate lacks it but a --
// --- same-page plain-text candidate found it (differently-cased name) -----

// TestBackfillWinnerIdentifiersHotelRoseRealEvidencePattern is a minimal,
// isolated reproduction of the exact real-world pattern found on
// hotelrose.at: a JSON-LD candidate (ranked highest, so it wins as "best")
// whose name is a superset/differently-worded string of a lower-ranked
// plain-text candidate's name, so mergeImprintCandidates' exact
// case-insensitive match never combines them — yet the plain-text
// candidate's VAT is genuinely for the same entity, since this whole
// function only ever runs on a single already-fetched, already-verified
// imprint page (see extractImprintFields's doc comment). Constructed
// directly against extractImprintCandidates + backfillWinnerIdentifiers
// (rather than the full hotelRoseImpressumFixture, already covered by
// TestExtractImprintFieldsHotelRoseSoleTraderRealEvidence above) to pin down
// the mechanism itself, independent of the country/register/responsible-
// person logic layered on top in extractImprintFields.
func TestBackfillWinnerIdentifiersHotelRoseRealEvidencePattern(t *testing.T) {
	body := `<!DOCTYPE html>
<html lang="de">
<head>
<script type="application/ld+json">
{"@context":"http://schema.org","@type":"Hotel","name":"Aktivhotel zur Rose - Franz Holzmann","address":{"@type":"PostalAddress","streetAddress":"Brenner Straße 30","postalCode":"6150","addressLocality":"Steinach am Brenner","addressCountry":"A"}}
</script>
</head>
<body>
<p>Firmenname: Aktivhotel Zur Rose<br>UID-Nr.: ATU43951103</p>
</body>
</html>`

	cands := mergeImprintCandidates(extractImprintCandidates(body, "https://www.hotelrose.at/impressum_de.html"))
	best := bestImprintCandidate(cands)
	if best == nil {
		t.Fatal("bestImprintCandidate returned nil")
	}
	if best.Source != "json_ld" {
		t.Fatalf("test setup invalid: best.Source = %q, want json_ld (this test exists to prove JSON-LD wins while lacking the VAT another candidate found)", best.Source)
	}
	if best.VAT != "" {
		t.Fatalf("test setup invalid: best.VAT = %q, want empty before backfill (this real JSON-LD block has no vatID field)", best.VAT)
	}

	backfillWinnerIdentifiers(best, cands)

	if best.VAT != "ATU43951103" {
		t.Errorf("after backfillWinnerIdentifiers, best.VAT = %q, want ATU43951103", best.VAT)
	}
	if best.Name != "Aktivhotel zur Rose - Franz Holzmann" {
		t.Errorf("backfill must not overwrite the winning candidate's own name; Name = %q", best.Name)
	}

	// End-to-end: the same pattern through the full extraction pipeline must
	// surface the VAT in the final Imprint.
	im := extractImprintFields("https://www.hotelrose.at/impressum_de.html", body, "hotelrose.at")
	if im.VAT != "ATU43951103" {
		t.Errorf("extractImprintFields: VAT = %q, want ATU43951103", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("extractImprintFields: VATValidation = %q, want checksum_valid", im.VATValidation)
	}
}

// TestBackfillWinnerIdentifiersDoesNotBackfillFromUnrelatedName is a
// deliberately-scoped-back guard: backfillWinnerIdentifiers is intentionally
// loose about NAME matching (that's the whole point of this fix), but it
// still only ever draws from the actual same-page candidate list that
// extractImprintCandidates produced for THIS page — it is not a blind
// page-wide regex sweep. This test just pins down that the mechanism has no
// hidden extra source: with a single candidate (no second candidate to
// backfill from at all), best.VAT/Register are left exactly as extracted.
func TestBackfillWinnerIdentifiersDoesNotBackfillFromUnrelatedName(t *testing.T) {
	cands := []imprintCandidate{
		{Name: "Solo Trader GmbH", Source: "json_ld"},
	}
	best := bestImprintCandidate(cands)
	if best == nil {
		t.Fatal("bestImprintCandidate returned nil")
	}
	backfillWinnerIdentifiers(best, cands)
	if best.VAT != "" {
		t.Errorf("VAT = %q, want empty (no other candidate exists to backfill from)", best.VAT)
	}
	if best.Register != "" {
		t.Errorf("Register = %q, want empty (no other candidate exists to backfill from)", best.Register)
	}
}

// TestHasImprintContactDetectsCFObfuscatedEmail proves the actual bug: the
// plain-text email regex cannot see a Cloudflare-obfuscated address at
// all, so hasImprintContact must have been returning a false negative for
// any page using this real, extremely common anti-scraping pattern before
// the fix; after it, the decoded address satisfies the "contact" checklist
// item exactly as a plain mailto: link would.
func TestHasImprintContactDetectsCFObfuscatedEmail(t *testing.T) {
	if imprintEmailRE.MatchString(bruceBetCasinoCFEmailSnippet) {
		t.Fatal("test setup invalid: the plain-text email regex should NOT match Cloudflare-obfuscated markup — that IS the bug this test proves")
	}
	if !hasCFObfuscatedEmail(bruceBetCasinoCFEmailSnippet) {
		t.Error("hasCFObfuscatedEmail returned false for a real Cloudflare-obfuscated email snippet")
	}
	if !hasImprintContact(bruceBetCasinoCFEmailSnippet) {
		t.Error("hasImprintContact returned false — the Cloudflare-obfuscated email should now count as a functional contact channel")
	}
}

// --- Bug 6: uncorroborated third-party brand mention in comparison prose --

// atpkitzThirdPartyBrandMentionFixture is a realistic (not contrived)
// reconstruction of the exact false-positive pattern found stress-testing
// the shipped 1.5.2 extractor against atpkitz.at, a real Austrian
// sports-betting comparison site: its actual homepage runs a "Top 5"
// operator table naming several third-party licensed brands with
// legal-sounding language nearby (license claims, a GmbH-suffixed entity
// name) — a structural pattern this whole affiliate/comparison-site cohort
// exhibits, since a review page must name the operators it's comparing.
// "Winrolla GmbH" here is a brand being reviewed in marketing prose, not
// the entity that operates the page — the real subject of its own
// Impressum — and has neither an address, nor a VAT/register number,
// anywhere near it.
const atpkitzThirdPartyBrandMentionFixture = `<html><body><h1>Impressum</h1>
<p>DuoSpin Casino ist lizenziert durch die MGA (Malta Gaming Authority) und bietet
100% Bonus bis zu 200 Euro. Winrolla GmbH ist ein weiterer Top-Anbieter mit MGA-Lizenz.</p>
<p>Kontaktieren Sie uns: info@wettvergleich-beispiel.net</p>
</body></html>`

// TestExtractImprintFieldsRejectsUncorroboratedThirdPartyBrandMention is the
// real-evidence regression test for the bug itself: before this fix,
// extractImprintFields returned "Winrolla GmbH" as legal_name (suffix
// "GmbH", completeness_score 40) purely because the suffix-anchored
// plain-text scan trusted ANY capitalized-word-run-plus-legal-suffix match
// anywhere on the page, with no requirement that it sit near any other
// imprint-shaped signal. After the fix, bestImprintCandidate skips this
// candidate entirely (no address, no register, no VAT anywhere near it),
// so the page correctly ends up with no legal_name at all rather than a
// hallucinated one — the genuinely-present contact email (unrelated to
// entity-name attribution) still counts toward the "contact" checklist
// item, which is why completeness_score is low-but-nonzero rather than 0.
func TestExtractImprintFieldsRejectsUncorroboratedThirdPartyBrandMention(t *testing.T) {
	im := extractImprintFields("https://wettvergleich-beispiel.net/impressum", atpkitzThirdPartyBrandMentionFixture, "wettvergleich-beispiel.net")

	if im.LegalName == "Winrolla GmbH" {
		t.Fatal("LegalName = \"Winrolla GmbH\" — a third-party brand named in comparison prose must never be attributed as the page's own legal_name")
	}
	if im.LegalName != "" {
		t.Errorf("LegalName = %q, want empty (no candidate on this page has any nearby address/register/VAT corroboration)", im.LegalName)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty", im.Address)
	}
	if im.Suffix != "" {
		t.Errorf("Suffix = %q, want empty", im.Suffix)
	}
	if !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want it to still contain contact (the page-wide email check is independent of entity-name attribution)", im.FieldsFound)
	}
	if !containsStr(im.FieldsMissing, "legal_name") {
		t.Errorf("FieldsMissing = %v, want it to contain legal_name", im.FieldsMissing)
	}
	if im.CompletenessScore >= 40 {
		t.Errorf("CompletenessScore = %d, want well below the pre-fix 40 (no more legal_name credit)", im.CompletenessScore)
	}
}

// atpkitzMultiBrandComparisonTableFixture extends the single-brand fixture
// above to a realistic multi-operator "Top 5" table (the real atpkitz.at
// shape this bug report describes), with three distinct suffix-anchored
// brand names (GmbH/GmbH/Ltd) sitting next to a numeric rating column and
// short bonus-percentage blurbs — deliberately included because a rating
// cell like "9.4" or a blurb like "100% Bonus bis zu 500 Euro" carries bare
// digits that could themselves be mistaken for a postal address by a loose
// digit-only heuristic (see hasDigitRun's doc comment: this exact shape is
// what exposed that gap during this fix's own verification pass). None of
// the three brands has a real address/register/VAT anywhere near it.
const atpkitzMultiBrandComparisonTableFixture = `<html><body><h1>Impressum</h1>
<table>
<tr><td>DuoSpin</td><td>9.8</td><td>MGA-Lizenz</td></tr>
<tr><td>Winrolla GmbH</td><td>9.6</td><td>100% Bonus bis zu 500 Euro</td></tr>
<tr><td>Kingmaker GmbH</td><td>9.4</td><td>MGA-Lizenz</td></tr>
<tr><td>Ragnaro Ltd</td><td>9.1</td><td>UKGC-Lizenz</td></tr>
<tr><td>Opabet</td><td>8.9</td><td></td></tr>
</table>
<p>Kontaktieren Sie uns: info@wettvergleich-beispiel.net</p>
</body></html>`

// TestExtractImprintFieldsRejectsMultiBrandComparisonTable confirms the
// single per-candidate proximity-corroboration fix already generalizes to
// a real multi-operator comparison table without needing a separate
// "multiple distinct suffix-anchored candidates ⇒ reject all" heuristic:
// each of the three GmbH/Ltd-suffixed brand candidates independently fails
// proximity corroboration (none has a real address/register/VAT near it),
// so none of them wins, exactly as the single-brand case above.
func TestExtractImprintFieldsRejectsMultiBrandComparisonTable(t *testing.T) {
	im := extractImprintFields("https://wettvergleich-beispiel.net/impressum", atpkitzMultiBrandComparisonTableFixture, "wettvergleich-beispiel.net")

	if im.LegalName != "" {
		t.Errorf("LegalName = %q, want empty — none of the three comparison-table brand mentions (Winrolla GmbH, Kingmaker GmbH, Ragnaro Ltd) has any nearby address/register/VAT corroboration", im.LegalName)
	}
	if im.Address != "" {
		t.Errorf("Address = %q, want empty (a rating column and bonus-percentage blurb must not be mistaken for a postal address)", im.Address)
	}

	cands := mergeImprintCandidates(extractImprintCandidates(atpkitzMultiBrandComparisonTableFixture, "https://wettvergleich-beispiel.net/impressum"))
	names := map[string]bool{}
	for _, c := range cands {
		names[c.Name] = true
	}
	for _, want := range []string{"Winrolla GmbH", "Kingmaker GmbH", "Ragnaro Ltd"} {
		if !names[want] {
			t.Fatalf("test setup invalid: extraction never even produced a %q candidate to test corroboration against; got %+v", want, cands)
		}
	}
	best := bestImprintCandidate(cands)
	if best != nil {
		t.Errorf("bestImprintCandidate = %+v, want nil (every named candidate on this page is an uncorroborated plain-text brand mention)", best)
	}
}

// TestHasProximityCorroboration is a direct unit test of the gate itself:
// structured sources (json_ld/hcard) are always trusted regardless of
// Address/Register/VAT; imprint_text/imprint_label sources need at least
// one of the three.
func TestHasProximityCorroboration(t *testing.T) {
	cases := []struct {
		name string
		c    imprintCandidate
		want bool
	}{
		{"json_ld, no corroboration needed", imprintCandidate{Source: "json_ld"}, true},
		{"hcard, no corroboration needed", imprintCandidate{Source: "hcard"}, true},
		{"imprint_text, no corroboration", imprintCandidate{Source: "imprint_text"}, false},
		{"imprint_text, address corroborates", imprintCandidate{Source: "imprint_text", Address: "Musterstraße 1, 10115 Berlin"}, true},
		{"imprint_text, register corroborates", imprintCandidate{Source: "imprint_text", Register: "HRB 12345"}, true},
		{"imprint_text, VAT corroborates", imprintCandidate{Source: "imprint_text", VAT: "DE123456788"}, true},
		{"imprint_label, no corroboration", imprintCandidate{Source: "imprint_label"}, false},
		{"imprint_label, address corroborates", imprintCandidate{Source: "imprint_label", Address: "Brennerstraße 30, 6150 Steinach"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasProximityCorroboration(&c.c); got != c.want {
				t.Errorf("hasProximityCorroboration(%+v) = %v, want %v", c.c, got, c.want)
			}
		})
	}
}

// TestHasDigitRun is a direct unit test of the tightened digit heuristic:
// short numeric fragments common in ratings/percentages/prices must not
// qualify, while postal-code-shaped runs (this codebase's real target
// jurisdictions: DE 5-digit, AT/NL 4-digit) must.
func TestHasDigitRun(t *testing.T) {
	cases := []struct {
		s    string
		n    int
		want bool
	}{
		{"9.6", 4, false},
		{"100% Bonus bis zu 500 Euro", 4, false},
		{"9.4", 4, false},
		{"10115 Berlin", 4, true},
		{"1010 Wien", 4, true},
		{"6150 Steinach in Tirol, Österreich", 4, true},
		{"", 4, false},
	}
	for _, c := range cases {
		if got := hasDigitRun(c.s, c.n); got != c.want {
			t.Errorf("hasDigitRun(%q, %d) = %v, want %v", c.s, c.n, got, c.want)
		}
	}
}
