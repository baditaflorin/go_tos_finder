package main

import "testing"

// Round-19 EU-expansion real-evidence fixture, fetched 2026-08-23: Bulgaria.
// Real excerpt from https://cressi.bg/obshti-usloviya/ (Кеме ЕООД — a real,
// live Bulgarian diving/sports-equipment retailer's Общи условия page) —
// byte-for-byte from the live page's own markup, native Cyrillic script
// throughout (no prior BG suffix entries existed at all, Latin or
// otherwise — unlike Greece in round 18, which at least had a
// Latin-transliterated quartet to extend).
//
// Running the shipped 1.7.13 extractor against this real page extracted
// almost nothing (only "contact", CompletenessScore 33), despite "Кеме
// ЕООД" being trivially suffix-matchable once a Cyrillic suffix table
// exists at all. Found and fixed three compounding real bugs, applying the
// exact lessons round 18's Greek round established:
//
//  1. suffixTable had zero Bulgarian entries of any kind. Added "ЕООД"
//     (real-evidence-confirmed on this page) plus "ООД"/"АД"/"ЕАД" (added
//     alongside it on the same well-established-sibling-forms basis round
//     18 used for Greek "Α.Ε."/"Ε.Π.Ε."/"Ο.Ε.").
//  2. Bulgaria's ЕИК (Unified Identification Code / BULSTAT) had no
//     vatPatterns entry at all. Added it, wired as VAT-equivalent (its
//     9-digit body IS the same number the "BG"-prefixed EU VAT pattern
//     matches — confirmed on this SAME page, whose "Регистрация по ЗДДС
//     BG201845795" is literally "BG" + this page's own "ЕИК 201845795"),
//     plus a new bgEIKValid two-pass weighted-mod-11 checksum (the
//     published BULSTAT algorithm), confirmed against this page's real
//     value. Applied round 18's RE2 lesson proactively this time: no
//     leading `\b` on the Cyrillic-anchored pattern (RE2's `\b` is
//     ASCII-only and would otherwise silently never match).
//  3. Even with the identifier matching, its own line ("Наименование Кеме
//     ЕООД, ЕИК 201845795;") sits BEFORE the real "Седалище и адрес на
//     управление" address line and has no stop-marker in
//     extractAddressNearEntity, so it got wrongly absorbed into Address —
//     same failure shape as round 17/18's Hungarian/Greek markers. Added
//     "еик" as a skip-past (not stop) marker.
//
// Also confirmed: the EXISTING "BG"-prefixed VAT pattern (`\bBG\d{9,10}\b`,
// present since before this round) already matched this page's own
// "Регистрация по ЗДДС BG201845795" line without any changes needed — "BG"
// is the ISO country-prefix letters, always Latin per EU-wide VAT-number
// convention, so it was never subject to the Cyrillic-\b gotcha bug 2
// found for the domestic-label pattern.
//
// ЕИК was ALSO added to singleCountryIdentifierKind (-> "BG") — unlike
// round 18's Greek "ΑΦΜ" (deliberately excluded there over a Cyprus-
// sharing risk), Bulgaria is the only EU member state using Cyrillic
// script as an official alphabet, so there is no analogous sibling-country
// risk to guard against here.
const cressiBgFixture = `<!DOCTYPE html>
<html lang="bg">
<head><title>Общи условия</title></head>
<body>
<ul>
<li>1.2. Информация за <strong>Кеме ЕООД:</strong><br>Наименование Кеме ЕООД, ЕИК 201845795;<br>Седалище и адрес на управление гр. Русе 7000, ул. Муткурова 6<br>Данни за кореспонденция e-mail: <a href="mailto:cressi.bg@gmail.com">cressi.bg@gmail.com</a>, тел.: +359 888 228716<br>Регистрация по ЗДДС BG201845795</li>
</ul>
</body>
</html>`

func TestExtractImprintFieldsCressiBgRealEvidence(t *testing.T) {
	im := extractImprintFields("https://cressi.bg/obshti-usloviya/", cressiBgFixture, "cressi.bg")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "Кеме ЕООД"
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q", im.LegalName, wantName)
	}
	if im.Suffix != "ЕООД" {
		t.Errorf("Suffix = %q, want native Cyrillic \"ЕООД\"", im.Suffix)
	}
	if im.Country != "BG" {
		t.Errorf("Country = %q, want BG", im.Country)
	}
	const wantAddress = "Седалище и адрес на управление гр. Русе 7000, ул. Муткурова 6"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (must NOT include the \"Наименование ... ЕИК ...\" line)", im.Address, wantAddress)
	}
	if im.VAT != "BG201845795" {
		t.Errorf("VAT = %q, want BG201845795", im.VAT)
	}
	if im.VATValidation != string(checksumValid) {
		t.Errorf("VATValidation = %q, want checksum_valid (real ЕИК 201845795 via the BG-prefixed VAT pattern)", im.VATValidation)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (no dedicated Bulgarian ruleset added this round)", im.Ruleset)
	}
	if im.CompletenessScore != 100 {
		t.Errorf("CompletenessScore = %d, want 100", im.CompletenessScore)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") ||
		!containsStr(im.FieldsFound, "contact") || !containsStr(im.FieldsFound, "vat_valid") {
		t.Errorf("FieldsFound = %v, want legal_name, address, contact, and vat_valid all present", im.FieldsFound)
	}
}

// TestBgEIKChecksum is a direct unit test of the new bgEIKValid wiring: the
// real ЕИК found during this round's search must pass (via both the
// domestic "ЕИК" label form and the "BG"-prefixed VAT form), and a flipped
// last digit must fail.
func TestBgEIKChecksum(t *testing.T) {
	cases := []struct {
		name string
		kind string
		raw  string
		want validity
	}{
		{"cressi.bg real ЕИК (domestic label)", "ЕИК", "ЕИК 201845795", checksumValid},
		{"cressi.bg real VAT (BG-prefixed, same body)", "VAT", "BG201845795", checksumValid},
		{"flipped last digit must fail", "ЕИК", "ЕИК 201845794", checksumInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validateIdentifier(tc.kind, tc.raw, "BG"); got != tc.want {
				t.Errorf("validateIdentifier(%s, %q) = %q, want %q", tc.kind, tc.raw, got, tc.want)
			}
		})
	}
	if got := singleCountryIdentifierKind("ЕИК"); got != "BG" {
		t.Errorf("singleCountryIdentifierKind(ЕИК) = %q, want BG", got)
	}
}

// TestBgEIKPatternHasNoLeadingWordBoundary is a direct regression test
// applying round 18's RE2 lesson proactively: confirms the shipped "ЕИК"
// vatPattern (imprint_vat.go) actually matches real Cyrillic label+value
// text (a naive `\bЕИК` would never match anything at all).
func TestBgEIKPatternHasNoLeadingWordBoundary(t *testing.T) {
	ids := findIdentifiers("ЕИК 201845795", 5)
	found := false
	for _, id := range ids {
		if id.Kind == "ЕИК" {
			found = true
		}
	}
	if !found {
		t.Errorf("findIdentifiers did not match \"ЕИК 201845795\" — got %v", ids)
	}
}

// TestBgRegisterMarkerSkipsNotBreaks is a direct unit test of the
// extractAddressNearEntity fix: an "еик" line must be skipped (continue),
// not treated as a hard stop — the real address that follows it on
// cressi.bg's real page must still be found.
func TestBgRegisterMarkerSkipsNotBreaks(t *testing.T) {
	text := "Кеме ЕООД\nНаименование Кеме ЕООД, ЕИК 201845795;\nСедалище и адрес на управление гр. Русе 7000, ул. Муткурова 6"
	got := extractAddressNearEntity(text, "Кеме ЕООД")
	const want = "Седалище и адрес на управление гр. Русе 7000, ул. Муткурова 6"
	if got != want {
		t.Errorf("extractAddressNearEntity = %q, want %q", got, want)
	}
}
