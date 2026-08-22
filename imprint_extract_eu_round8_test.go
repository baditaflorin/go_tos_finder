package main

import (
	"strings"
	"testing"
)

// Round-8 EU-expansion real-evidence fixture, fetched 2026-08-22.

// urbanaPtFixture: real "Ficha técnica" page, urbana.com.pt (a Portuguese
// Sociedade Unipessoal Lda).
const urbanaPtFixture = `<!DOCTYPE html>
<html lang="pt">
<head><title>Ficha técnica | Casas de Portugal</title></head>
<body>
<article>
<p><strong>Propriedade:</strong>&nbsp;MoonMedia Comunicação, Soc. Unipessoal Lda</p>
<p>NIPC -508980186</p>
<p><strong>Edição</strong>:MoonMedia Comunicação, Soc. Unipessoal Lda</p>
<p><strong>Sede de Redação: </strong>Rua dos Pinheiros, 47 &#8211; Casa 4 &#8211; Bicesse &ndash; 2645 &#8211; 535 Alcabideche</p>
<p><strong>Gerência:</strong>&nbsp;Maria do Amparo Santa Clara</p>
<p><strong>Diretor:</strong>&nbsp;Amparo Santa Clara<em>&nbsp;(<a href="/cdn-cgi/l/email-protection" class="__cf_email__" data-cfemail="cdaca0bdacbfa28daeacbeacbea9a8bda2bfb9b8aaaca1e3aea2a0e3bdb9">[email&#160;protected]</a>)</em></p>
<p>Registo na ERC com o nº 126793 Depósito Legal nº 86460/09.</p>
</article>
</body>
</html>`

// TestExtractImprintFieldsUrbanaPortugueseRealEvidence: real evidence for
// a Portuguese Sociedade Unipessoal Lda. Exercises three real, distinct
// bugs found while extracting this one real page: (1) NIPC (the domestic
// tax/register ID, written "NIPC -508980186" with no "PT" country prefix)
// had no pattern at all; (2) a line starting with a literal, un-decoded
// "&nbsp;" spacer entity — extremely common after a bold "<strong>Label:
// </strong>" — was entirely discarded by the entity-corruption rejection
// filter, silently dropping otherwise-valid content; (3) the
// sentence-boundary heuristic in extractEntityAround misidentified "Soc."
// (Portuguese for "Sociedade", a legal-form-adjacent abbreviation ending
// in a period) as ending a sentence, truncating the candidate name down
// to nothing usable and losing the legal_name entirely.
func TestExtractImprintFieldsUrbanaPortugueseRealEvidence(t *testing.T) {
	im := extractImprintFields("https://urbana.com.pt/ficha-tecnica/", urbanaPtFixture, "urbana.com.pt")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "MoonMedia Comunicação, Soc. Unipessoal Lda"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (the \"Soc.\" abbreviation must not be misread as a sentence boundary)", im.LegalName, wantName)
	}
	if im.Suffix != "Unipessoal Lda" {
		t.Errorf("Suffix = %q, want \"Unipessoal Lda\"", im.Suffix)
	}
	if im.Country != "PT" {
		t.Errorf("Country = %q, want PT", im.Country)
	}
	if im.VAT != "508980186" {
		t.Errorf("VAT = %q, want the clean 9-digit NIPC code", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (NIPC uses the same weighted-mod-11 algorithm as the PT-prefixed VAT form)", im.VATValidation)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact+vat_valid", im.FieldsFound)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100 (eu_baseline: Portugal has no dedicated ruleset yet — NIPC's dual VAT/register role isn't cleanly evidenced enough to require a separate register field)", im.CompletenessScore)
	}
}

// TestPrecededByKnownAbbreviationDoesNotOverreach is a direct unit test of
// the "Soc." sentence-boundary exception, independent of the full
// extraction pipeline: it must recognise the real abbreviation and must
// NOT suppress the sentence-boundary check for an ordinary sentence-ending
// word that merely happens to end in similar letters.
func TestPrecededByKnownAbbreviationDoesNotOverreach(t *testing.T) {
	text := "Foo Soc. Bar"
	periodIdx := strings.Index(text, ".")
	if !precededByKnownAbbreviation(text, periodIdx) {
		t.Errorf("expected \"Soc.\" to be recognised as a known non-sentence abbreviation")
	}
	text2 := "This is a real sentence. Another one"
	periodIdx2 := strings.Index(text2, ".")
	if precededByKnownAbbreviation(text2, periodIdx2) {
		t.Errorf("did not expect \"sentence.\" to be recognised as a known non-sentence abbreviation")
	}
}
