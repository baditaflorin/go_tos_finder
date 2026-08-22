package main

import "testing"

// Round-10 EU-expansion real-evidence fixture, fetched 2026-08-22.

// menuLuFixture: real "Mentions légales" excerpt from menu.lu/fr/mentions-legales
// (WeServices S.à r.l., a real Luxembourg restaurant-directory/booking
// company). Byte-for-byte from the live page, including its literal,
// un-decoded named HTML entities (&agrave;, &egrave;, &eacute;, &deg;, ...).
const menuLuFixture = `<!DOCTYPE html>
<html lang="fr">
<head><title>Mentions légales | Menu.lu</title></head>
<body>
<div class="wysiwyg">
<p>Derni&egrave;re mise &agrave; jour : mai 2026</p>
<h2>1. Mentions l&eacute;gales</h2>
<p>Le site Internet <strong>https://www.menu.lu</strong> ainsi que les applications mobiles MENU.LU sont &eacute;dit&eacute;s par :</p>
<p><strong>WeServices S.&agrave; r.l.</strong><br /> Si&egrave;ge social : 5, Rue Aldringen, L-1118 Luxembourg (Grand-Duch&eacute; de Luxembourg)<br /> RCS Luxembourg n&deg; B258641<br /> N&deg; TVA intracommunautaire : LU33554515<br /> Courriel : info@menu.lu</p>
<p><strong>Directeur de la publication :</strong> le repr&eacute;sentant l&eacute;gal de WeServices S.&agrave; r.l.</p>
<p><strong>H&eacute;bergement :</strong> le site et les applications sont h&eacute;berg&eacute;s par WeServices S.&agrave; r.l., 5, Rue Aldringen, L-1118 Luxembourg. Les donn&eacute;es sont stock&eacute;es sur des serveurs situ&eacute;s au sein de l&rsquo;Union europ&eacute;enne.</p>
</div>
</body>
</html>`

// TestExtractImprintFieldsMenuLuRealEvidence: real evidence for a
// Luxembourg S.à r.l. (Loi du 14 août 2000 relative au commerce
// électronique, Art. 4 — Luxembourg's e-Commerce Directive transposition —
// requires name, address, contact, and the trade register + registration
// number "where applicable"). Running the shipped 1.7.4 extractor against
// this real page extracted NOTHING at all: no legal_name, no candidates,
// despite "WeServices S.à r.l." being trivially suffix-matchable in
// isolation. Found and fixed three compounding real bugs:
//
//  1. suffixTable's Luxembourg entry ("S.à r.l.", imprint_suffix.go)
//     contains a raw "à" character, but stripTagsLines never decodes HTML
//     entities and this real page renders that letter as its named entity
//     — "WeServices S.&agrave; r.l." — so detectSuffix could never match
//     it at all. Added a small, targeted decodeKnownAccentEntities step
//     (imprint_jsonld.go), applied page-wide so the extracted candidate
//     name and the later strings.Contains(line, name) address lookup both
//     see the same (decoded) text.
//  2. Luxembourg's RCS (Registre de Commerce et des Sociétés) company
//     number had no pattern at all — "RCS Luxembourg n&deg; B258641".
//     Added a dedicated "RCS" vatPattern requiring a letter immediately
//     before the digits (Luxembourg's shape), which keeps it from
//     colliding with France's differently-shaped, all-digit RCS citation.
//  3. Once the register line was found, it sat immediately after the real
//     street address with no intervening HTML tag boundary, so it cleared
//     looksAddressLine's digit-run heuristic and got absorbed into the
//     Address field as if it were a continuation postal line. Added "rcs"
//     to extractAddressNearEntity's stop-marker list.
//
// All three had to be fixed together for this one real page to extract
// correctly. No dedicated Luxembourg ruleset added: Art. 4's
// register-number requirement is conditional ("where applicable"), the
// same conditionality as the EU baseline itself.
func TestExtractImprintFieldsMenuLuRealEvidence(t *testing.T) {
	im := extractImprintFields("https://menu.lu/fr/mentions-legales", menuLuFixture, "menu.lu")

	if !im.Present {
		t.Fatal("expected Present=true")
	}
	const wantName = "WeServices S.à r.l."
	if im.LegalName != wantName {
		t.Errorf("LegalName = %q, want %q (the entity-encoded \"S.&agrave; r.l.\" must decode to a real suffix match)", im.LegalName, wantName)
	}
	if im.Suffix != "S.à r.l." {
		t.Errorf("Suffix = %q, want \"S.à r.l.\"", im.Suffix)
	}
	if im.Country != "LU" {
		t.Errorf("Country = %q, want LU", im.Country)
	}
	// Only "à" is in the decode allowlist (real-evidence-driven, see the
	// doc comment above) — "è"/"é" stay literal HTML entities in the
	// output, same as the rest of this codebase's accepted un-decoded-
	// entity behavior (round 8's Portuguese fixture left "&#8211;"
	// un-decoded the same way).
	const wantAddress = "Si&egrave;ge social : 5, Rue Aldringen, L-1118 Luxembourg (Grand-Duch&eacute; de Luxembourg)"
	if im.Address != wantAddress {
		t.Errorf("Address = %q, want %q (the RCS register line must NOT be absorbed into it)", im.Address, wantAddress)
	}
	const wantRegister = "RCS Luxembourg n&deg; B258641"
	if im.Register != wantRegister {
		t.Errorf("Register = %q, want %q", im.Register, wantRegister)
	}
	if im.VAT != "LU33554515" {
		t.Errorf("VAT = %q, want \"LU33554515\"", im.VAT)
	}
	if im.VATValidation != string(formatValid) {
		t.Errorf("VATValidation = %q, want format_valid (LU deliberately only claims format_valid — see validateVAT's doc comment)", im.VATValidation)
	}
	if im.Ruleset != RulesetEUBaseline {
		t.Errorf("Ruleset = %q, want eu_baseline (Luxembourg's Art. 4 register requirement is conditional \"where applicable\", same as the EU baseline itself — no dedicated Luxembourg ruleset added this round)", im.Ruleset)
	}
	if !containsStr(im.FieldsFound, "legal_name") || !containsStr(im.FieldsFound, "address") || !containsStr(im.FieldsFound, "contact") {
		t.Errorf("FieldsFound = %v, want legal_name+address+contact", im.FieldsFound)
	}
}

// TestDecodeKnownAccentEntities is a direct unit test of the new decoder,
// independent of the full extraction pipeline.
func TestDecodeKnownAccentEntities(t *testing.T) {
	if got := decodeKnownAccentEntities("S.&agrave; r.l."); got != "S.à r.l." {
		t.Errorf("decodeKnownAccentEntities(%q) = %q, want %q", "S.&agrave; r.l.", got, "S.à r.l.")
	}
	// Deliberately untouched: &nbsp;/&quot;/&copy;/&amp; must survive so the
	// entity-corruption rejection filter in extractImprintText still sees
	// them (see decodeKnownAccentEntities' own doc comment).
	if got := decodeKnownAccentEntities("A&nbsp;B&amp;C"); got != "A&nbsp;B&amp;C" {
		t.Errorf("decodeKnownAccentEntities(%q) = %q, want it unchanged", "A&nbsp;B&amp;C", got)
	}
}
