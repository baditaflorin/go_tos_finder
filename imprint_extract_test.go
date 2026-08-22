package main

import (
	"strings"
	"testing"
)

// These fixtures follow this repo's own testing convention (confirmed by
// inspection: no testdata/ dir, no external .html files anywhere in this
// repo or go_legal_entity) — inline Go backtick HTML literals.

const deImprintFixture = `<!DOCTYPE html>
<html>
<head><title>Impressum</title></head>
<body>
<h1>Impressum</h1>
<p>Musterfirma GmbH</p>
<p>Musterstraße 1<br>10115 Berlin<br>Deutschland</p>
<p>Handelsregister: HRB 12345<br>Amtsgericht Berlin-Charlottenburg</p>
<p>USt-IdNr.: DE123456788</p>
<p>Geschäftsführer: Max Mustermann</p>
<p>Kontakt: <a href="mailto:info@musterfirma.de">info@musterfirma.de</a>, Tel: +49 30 1234567</p>
</body>
</html>`

const atImprintFixture = `<!DOCTYPE html>
<html>
<head><title>Impressum</title></head>
<body>
<h1>Impressum</h1>
<p>Musterfirma GmbH</p>
<p>Hauptstraße 5<br>1010 Wien<br>Österreich</p>
<p>Firmenbuchnummer: FN 123456a<br>Firmenbuchgericht Wien</p>
<p>UID: ATU12345675</p>
<p>Vertretungsberechtigte Geschäftsführerin: Maria Musterfrau</p>
<p>Kontakt: <a href="mailto:office@musterfirma.at">office@musterfirma.at</a></p>
</body>
</html>`

const sparseImprintFixture = `<!DOCTYPE html>
<html>
<head><title>Impressum</title></head>
<body>
<h1>Impressum</h1>
<p>Acme Consulting</p>
<p>Contact: <a href="mailto:hello@acme.example">hello@acme.example</a></p>
</body>
</html>`

const jsonldImprintFixture = `<!DOCTYPE html>
<html>
<head><title>Impressum</title>
<script type="application/ld+json">
{
  "@context": "https://schema.org",
  "@type": "Organization",
  "legalName": "Beispiel GmbH",
  "vatID": "DE123456788",
  "address": {
    "@type": "PostalAddress",
    "streetAddress": "Beispielweg 9",
    "postalCode": "80331",
    "addressLocality": "München",
    "addressCountry": "DE"
  }
}
</script>
</head>
<body>
<h1>Impressum</h1>
<p>Beispiel GmbH</p>
<p>Beispielweg 9<br>80331 München</p>
<p>Handelsregister: HRB 99999<br>Amtsgericht München</p>
<p>USt-IdNr.: DE123456788</p>
<p>Geschäftsführer: Erika Musterfrau</p>
</body>
</html>`

// blogAboutImpressumFixture is NOT a real imprint page: a blog post
// explaining what an Impressum is. It uses the same vocabulary
// (Impressum/Geschäftsführer/Handelsregister/vertretungsberechtigt) a real
// one would, in ordinary prose, with no actual structured company data.
const blogAboutImpressumFixture = `<!DOCTYPE html>
<html>
<head><title>What is an Impressum? A Guide to German Law</title></head>
<body>
<h1>What is an Impressum?</h1>
<p>Every German business needs an Impressum. This legal notice page must
disclose the Geschäftsführer and other company details, as required by
German law. Many small business owners are confused about what a
Handelsregister entry actually requires them to publish, and whether their
vertretungsberechtigt officer needs to be named.</p>
<p>In this guide we explain the basics of the German Impressum requirement
and why every company operating in Germany needs one on their website.</p>
</body>
</html>`

// TestExtractImprintFieldsGermanGmbH: a complete German imprint (GmbH + HRB
// + Amtsgericht + USt-IdNr + Geschäftsführer + full street address) should
// yield every field, ruleset de_tmg, and a 100 completeness score.
func TestExtractImprintFieldsGermanGmbH(t *testing.T) {
	im := extractImprintFields("https://musterfirma.de/impressum", deImprintFixture, "musterfirma.de")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	if im.LegalName != "Musterfirma GmbH" {
		t.Errorf("LegalName = %q, want %q", im.LegalName, "Musterfirma GmbH")
	}
	if im.Suffix != "GmbH" {
		t.Errorf("Suffix = %q, want GmbH", im.Suffix)
	}
	if im.Country != "DE" {
		t.Errorf("Country = %q, want DE", im.Country)
	}
	if im.Address != "Musterstraße 1, 10115 Berlin" {
		t.Errorf("Address = %q", im.Address)
	}
	if !strings.Contains(im.Register, "HRB 12345") || !strings.Contains(im.Register, "Amtsgericht") {
		t.Errorf("Register = %q, want it to contain HRB 12345 and the Amtsgericht", im.Register)
	}
	if im.VAT != "DE123456788" {
		t.Errorf("VAT = %q, want DE123456788", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
	if im.ResponsiblePerson != "Max Mustermann" {
		t.Errorf("ResponsiblePerson = %q, want Max Mustermann", im.ResponsiblePerson)
	}
	if im.Ruleset != RulesetDETMG {
		t.Errorf("Ruleset = %q, want de_tmg", im.Ruleset)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
	if len(im.FieldsMissing) != 0 {
		t.Errorf("FieldsMissing = %v, want none", im.FieldsMissing)
	}
}

// TestExtractImprintFieldsAustrianFN: a complete Austrian imprint (GmbH, FN
// + Firmenbuchgericht, AT VAT format ATU########, a "vertretungsberechtigt"
// mention) should classify as ruleset at_ecg_medieng — and, critically, the
// country must resolve to AT even though the bare "GmbH" suffix itself is
// DE/AT-ambiguous (the trimmed suffix table only carries a DE entry for it):
// the VAT's own ATU prefix must win over the suffix guess.
func TestExtractImprintFieldsAustrianFN(t *testing.T) {
	im := extractImprintFields("https://musterfirma.at/impressum", atImprintFixture, "musterfirma.at")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	if im.Country != "AT" {
		t.Fatalf("Country = %q, want AT (VAT prefix must win over the DE/AT-ambiguous \"GmbH\" suffix guess)", im.Country)
	}
	if im.LegalName != "Musterfirma GmbH" {
		t.Errorf("LegalName = %q, want %q", im.LegalName, "Musterfirma GmbH")
	}
	if !strings.Contains(im.Register, "FN 123456a") || !strings.Contains(im.Register, "Firmenbuchgericht") {
		t.Errorf("Register = %q, want it to contain FN 123456a and the Firmenbuchgericht", im.Register)
	}
	if im.VAT != "ATU12345675" {
		t.Errorf("VAT = %q, want ATU12345675", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid", im.VATValidation)
	}
	if im.ResponsiblePerson != "Maria Musterfrau" {
		t.Errorf("ResponsiblePerson = %q, want Maria Musterfrau", im.ResponsiblePerson)
	}
	if im.Ruleset != RulesetATECGMedienG {
		t.Errorf("Ruleset = %q, want at_ecg_medieng", im.Ruleset)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
}

// TestExtractImprintFieldsSparse: just a company name (no recognised legal
// form) plus one email, no address/register/VAT. Expect the correct
// FieldsMissing list, a low-but-nonzero completeness score, and eu_baseline
// (no DE/AT signal anywhere on the page).
func TestExtractImprintFieldsSparse(t *testing.T) {
	im := extractImprintFields("https://acme.example/impressum", sparseImprintFixture, "acme.example")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline", im.Ruleset)
	}
	// "Acme Consulting" carries no recognised legal-form suffix and no
	// structured data, so the name-extraction pipeline correctly finds
	// nothing rather than guessing.
	if im.LegalName != "" {
		t.Errorf("LegalName = %q, want empty (no legal-form suffix present)", im.LegalName)
	}
	if !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want it to contain contact", im.FieldsFound)
	}
	if !containsStr(im.FieldsMissing, "legal_name") || !containsStr(im.FieldsMissing, "address") {
		t.Errorf("FieldsMissing = %v, want legal_name and address", im.FieldsMissing)
	}
	if im.CompletenessScore == 0 || im.CompletenessScore >= 50 {
		t.Errorf("CompletenessScore = %d, want low-but-nonzero (<50, >0)", im.CompletenessScore)
	}
}

// TestExtractImprintFieldsJSONLD: a page with Schema.org JSON-LD
// Organization/legalName/vatID/address, where the SAME company is also
// named in the plain page text (with its Handelsregister number only
// present there, not in the JSON-LD block). Confirms the structured-data
// path is actually taken/preferred for name/VAT/address — the emitted
// Address matches the JSON-LD's own multi-field-joined shape, not the
// plain-text near-entity address format — while the plain-text-only
// register number is still merged in rather than lost.
func TestExtractImprintFieldsJSONLD(t *testing.T) {
	im := extractImprintFields("https://beispiel.de/impressum", jsonldImprintFixture, "beispiel.de")

	if im.LegalName != "Beispiel GmbH" {
		t.Errorf("LegalName = %q, want Beispiel GmbH", im.LegalName)
	}
	if im.VAT != "DE123456788" {
		t.Errorf("VAT = %q, want DE123456788", im.VAT)
	}
	// The JSON-LD address.go port (addressTextFrom) joins
	// streetAddress/postalCode/addressLocality/addressCountry with ", " —
	// a shape the plain-text near-entity scan (extractAddressNearEntity)
	// never produces (it never appends a bare trailing country code). This
	// exact string is only reachable via the JSON-LD path.
	const wantAddr = "Beispielweg 9, 80331, München, DE"
	if im.Address != wantAddr {
		t.Errorf("Address = %q, want %q (the JSON-LD structured address, proving json_ld outranked imprint_text)", im.Address, wantAddr)
	}
	if !strings.Contains(im.Register, "HRB 99999") {
		t.Errorf("Register = %q, want it to contain the plain-text-only HRB 99999 (same-entity candidates must merge across sources)", im.Register)
	}
	if im.Ruleset != RulesetDETMG {
		t.Errorf("Ruleset = %q, want de_tmg", im.Ruleset)
	}
}

// TestExtractImprintFieldsFalsePositiveGuard: a page that merely mentions
// "Impressum"/"Geschäftsführer"/"Handelsregister"/"vertretungsberechtigt" in
// passing prose (a blog post ABOUT the Impressum requirement) is not an
// actual imprint page. verify.go's docTypeSignalRE[DocImprint] vocabulary
// gate is what decides whether a page like this reaches extraction at all
// in production; this test covers the narrower, always-applicable
// guarantee: IF extraction runs on such a page, it must not hallucinate a
// company name, register number, or VAT from the surrounding prose.
func TestExtractImprintFieldsFalsePositiveGuard(t *testing.T) {
	im := extractImprintFields("https://blog.example/what-is-an-impressum", blogAboutImpressumFixture, "blog.example")

	if im.LegalName != "" {
		t.Errorf("LegalName = %q, want empty — no real company is named on this page", im.LegalName)
	}
	if im.Register != "" {
		t.Errorf("Register = %q, want empty", im.Register)
	}
	if im.VAT != "" {
		t.Errorf("VAT = %q, want empty", im.VAT)
	}
	if im.CompletenessScore > 20 {
		t.Errorf("CompletenessScore = %d, want very low (no real imprint content)", im.CompletenessScore)
	}
}

// TestExtractImprintFieldsNoBody covers the graceful-degradation path: the
// caller (handler_helpers.go) only has a Body to hand extractImprintFields
// when the verification phase actually re-fetched the page — a footer link
// that stayed "link_unverified" (see handler.go's DocFinding.LinkConfidence
// doc comment) has no body available. Present must stay true (still an
// honest imprint_present signal) with zero hallucinated fields.
func TestExtractImprintFieldsNoBody(t *testing.T) {
	im := extractImprintFields("https://example.com/impressum", "", "example.com")
	if !im.Present {
		t.Fatal("expected Present=true even without a body")
	}
	if im.CompletenessScore != 0 {
		t.Errorf("CompletenessScore = %d, want 0", im.CompletenessScore)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline default", im.Ruleset)
	}
	if im.LegalName != "" || im.VAT != "" || im.Register != "" {
		t.Errorf("expected no extracted fields without a body, got %+v", im)
	}
}

// TestValidateIdentifierChecksumsDEAndAT exercises the ported per-country
// check-digit algorithms directly (not just indirectly through a page
// fixture): a correct VAT must verify, a corrupted one must not, and a
// member state with no implemented algorithm must fall back to
// format_valid rather than a false checksum_invalid claim.
func TestValidateIdentifierChecksumsDEAndAT(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		value   string
		country string
		want    validity
	}{
		{"DE valid", "VAT", "DE123456788", "DE", checksumValid},
		{"DE corrupted check digit", "VAT", "DE123456789", "DE", checksumInvalid},
		{"AT valid", "VAT", "ATU12345675", "AT", checksumValid},
		{"AT corrupted check digit", "VAT", "ATU12345670", "AT", checksumInvalid},
		{"GB has no implemented algorithm", "VAT", "GB123456789", "GB", formatValid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateIdentifier(c.kind, c.value, c.country)
			if got != c.want {
				t.Errorf("validateIdentifier(%q, %q, %q) = %q, want %q", c.kind, c.value, c.country, got, c.want)
			}
		})
	}
}

// TestRulesetForCountry: only DE/AT get the stricter checklist; everything
// else — including an unresolved/empty country — falls back to
// eu_baseline rather than over-claiming a ruleset that doesn't apply.
func TestRulesetForCountry(t *testing.T) {
	cases := []struct {
		country string
		want    string
	}{
		{"DE", RulesetDETMG},
		{"AT", RulesetATECGMedienG},
		{"FR", RulesetEUBaseline},
		{"", RulesetEUBaseline},
	}
	for _, c := range cases {
		if got := rulesetFor(c.country); got != c.want {
			t.Errorf("rulesetFor(%q) = %q, want %q", c.country, got, c.want)
		}
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
