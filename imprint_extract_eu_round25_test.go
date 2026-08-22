package main

import "testing"

// Round-25 EU-expansion real-evidence fixture, fetched 2026-08-23: Latvia.
// Real excerpt from
// https://gatavosana.lv/lv/component/k2/item/271-juridiska-informacija.html
// (SIA SOLLER.LV — a real, live Latvian household-goods e-shop's
// Juridiskā informācija page) — byte-for-byte from the live page's own
// markup, including its literal un-decoded HTML entity (&#8211; en-dash).
//
// Running the shipped 1.7.19 extractor against this real page extracted
// NOTHING at all (CompletenessScore 0), despite "SIA" being an existing,
// already-high-confidence suffixTable entry. Found and fixed three
// compounding real bugs:
//
//  1. Latvia's "Reģistrācijas numurs" (registration number) had no
//     identifier pattern at all. Added it, wired as VAT-equivalent (its
//     11-digit body IS the same number the "LV"-prefixed EU VAT pattern
//     matches — confirmed on this SAME page, whose "PVN maksātāja
//     numurs: LV40103719642" is literally "LV" + this page's own
//     "Reģistrācijas numurs: 40103719642"), same architecture as
//     Bulgaria's ЕИК/Greece's ΑΦΜ/Croatia's OIB — UNLIKE round 24's
//     Lithuanian Įmonės kodas, which is a genuinely separate number from
//     its own country's VAT. No checksum implemented (stays
//     format_valid — several plausible weight sequences for the
//     published Latvian algorithm were tried by hand against the one
//     real value available and none matched, so nothing was guessed).
//     Both boundary characters in the pattern are plain ASCII, so
//     (unlike round 24's Lithuanian "Į"-leading pattern) no RE2 `\b`
//     workaround was needed here — verified directly, not assumed.
//  2. A real, general bug in extractEntityAfterSuffix (the prefix-form
//     name extractor round 24 already exercises for "UAB „Name""-style
//     jurisdictions): this page's own real entity name is "SIA
//     SOLLER.LV" — a domain-style brand name that itself contains a
//     period. The function's "stop at the first period" rule truncated
//     the real candidate to just "SOLLER", silently dropping its own
//     genuine ".LV" suffix. Added a narrow, structural check
//     (looksLikeDomainSuffixPeriod): a period immediately followed by
//     2-4 uppercase letters and then a non-letter (or end of string)
//     reads as a domain suffix, not a sentence boundary — a genuine
//     sentence-ending "SARL. Fondée en 2020..." still stops the scan
//     exactly as before.
//  3. Once the identifier and name extraction worked, "Reģistrācijas
//     numurs"/"PVN maksātāja numurs" values leaked into Address (both
//     sit right BEFORE the real "Juridiskā adrese:" line) — added both
//     as skip-past markers, same shape as every prior round's equivalent
//     fix.
//
// This round's real page also confirms the SAME suffix-then-VAT
// self-healing precedent established for rounds 20-23 remains unneeded
// here: "SIA" only ever maps to "LV" in suffixTable (no cross-country
// collision to resolve for this specific entry, unlike Croatia/Slovenia's
// shared "d.o.o." or Cyprus/GB's shared "ltd").
const gatavosanaLvFixture = `<!DOCTYPE html>
<html lang="lv">
<head><title>Juridiskā informācija</title></head>
<body>
<div class="itemFullText">
<div class="legal-info-page">
<h2>Juridiskā informācija</h2>
<h3>1. Informācija par uzņēmumu</h3>
<p>Šo tīmekļa vietni pārvalda uzņēmums <strong>SIA SOLLER.LV</strong>, kas ir reģistrēts Latvijas Republikā.</p>
<p>Reģistrācijas numurs: 40103719642<br />
PVN maksātāja numurs: LV40103719642<br />
Juridiskā adrese: Dzelzavas iela 88&#8211;6, Rīga, LV-1082, Latvija<br />
</p>
</div>
</div>
</body>
</html>`

func TestExtractImprintFieldsGatavosanaLvRealEvidence(t *testing.T) {
	im := extractImprintFields("https://gatavosana.lv/lv/component/k2/item/271-juridiska-informacija.html", gatavosanaLvFixture, "gatavosana.lv")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "SIA SOLLER.LV"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (must NOT be truncated to \"SIA SOLLER\" — the real name's own \".LV\" domain-style suffix)", im.LegalName, wantName)
	}
	if im.Suffix != "SIA" {
		t.Errorf("Suffix = %q, want \"SIA\"", im.Suffix)
	}
	if im.Country != "LV" {
		t.Errorf("Country = %q, want LV", im.Country)
	}
	if im.VAT != "LV40103719642" {
		t.Errorf("VAT = %q, want LV40103719642", im.VAT)
	}
	const wantAddress = "Juridiskā adrese: Dzelzavas iela 88&#8211;6, Rīga, LV-1082, Latvija"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (must NOT include the Reģistrācijas numurs/PVN maksātāja numurs lines)", im.Address, wantAddress)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Latvian ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") {
		t.Errorf("FieldsFound = %v, want legal_name and address present", im.FieldsFound)
	}
}

// TestLvRegistrationNumberIsVATEquivalent is a direct unit test confirming
// "Reģistrācijas numurs" is wired as VAT-equivalent and resolves to LV via
// singleCountryIdentifierKind.
func TestLvRegistrationNumberIsVATEquivalent(t *testing.T) {
	if !isVATLikeIdentifierKind("Reģistrācijas numurs") {
		t.Error("isVATLikeIdentifierKind(Reģistrācijas numurs) = false, want true")
	}
	if got := singleCountryIdentifierKind("Reģistrācijas numurs"); got != "LV" {
		t.Errorf("singleCountryIdentifierKind(Reģistrācijas numurs) = %q, want LV", got)
	}
	if got := cleanIdentifierValue("Reģistrācijas numurs", "Reģistrācijas numurs: 40103719642"); got != "40103719642" {
		t.Errorf("cleanIdentifierValue = %q, want 40103719642", got)
	}
}

// TestDomainStyleSuffixNotTreatedAsSentenceEnd is a direct unit test of the
// looksLikeDomainSuffixPeriod fix: a domain-style period ("SOLLER.LV")
// must not truncate the candidate, but a genuine sentence-ending period
// right after the suffix must still stop the scan.
func TestDomainStyleSuffixNotTreatedAsSentenceEnd(t *testing.T) {
	got := extractEntityAfterSuffix("SIA SOLLER.LV, kas ir reģistrēts", "SIA", 0)
	const want = "SIA SOLLER.LV"
	if got != want {
		t.Errorf("extractEntityAfterSuffix domain-suffix case = %q, want %q", got, want)
	}

	// A genuine sentence boundary right after the suffix must still stop
	// the scan: "SARL. Fondée en 2020..." — "Fondée" is lowercase-first
	// mid-word after decode but starts with capital F here; the point is
	// the period is NOT followed by 2-4 uppercase letters, so it must
	// still be treated as a sentence end.
	got2 := extractEntityAfterSuffix("SARL. Fondée en 2020 à Paris", "SARL", 0)
	if got2 != "" {
		t.Errorf("extractEntityAfterSuffix sentence-boundary case = %q, want empty (genuine sentence end must still stop the scan)", got2)
	}
}
