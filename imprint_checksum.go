package main

import "strings"

// Vendored from go_legal_entity's checksum.go / checksum_helpers.go /
// checksum_extract.go / checksum_validate.go (v1.10.14) — real per-country
// VAT/register check-digit validation, not just regex-shape matching. See
// imprint_vat.go for the identifier table this operates on.

// validity is the structural-validation status of an extracted identifier.
// It lifts the service from "regex matched a shape" to "the identifier's
// check digit verifies under the jurisdiction's published algorithm" —
// without any external registry call.
type validity string

const (
	// checksumValid means the identifier carries a check digit and it
	// recomputes correctly under the country's published algorithm.
	checksumValid validity = "checksum_valid"
	// checksumInvalid means the identifier has a checksum algorithm we know
	// and the check digit does NOT recompute — a likely OCR/typo/false match.
	checksumInvalid validity = "checksum_invalid"
	// formatValid means the identifier matches the country's format but we
	// have no published check-digit algorithm to verify it further.
	formatValid validity = "format_valid"
)

func mod11Verdict(ok bool) validity {
	if ok {
		return checksumValid
	}
	return checksumInvalid
}

// validateIdentifier returns the structural-validation verdict for a single
// VAT/registration identifier. The check-digit algorithms below are the
// publicly documented ones used by the EU VIES system and the national
// registries; none requires a network call.
func validateIdentifier(kind, value, country string) validity {
	digits := onlyAlnum(value)
	switch kind {
	case "VAT":
		return validateVAT(country, digits)
	case "USt-IdNr":
		// German USt-IdNr label hit ("USt-IdNr.: DE123456789"). The captured
		// value carries the label, so onlyAlnum() would leave "USTIDNR…" in
		// front of the number; pull the trailing 9-digit body instead and
		// route it through the DE VAT validator so it promotes past
		// format_valid.
		d := extractDigits(value)
		if len(d) > 9 {
			d = d[len(d)-9:] // last 9 digits = the USt-IdNr body
		}
		return validateVAT("DE", d)
	case "CUI":
		// Romanian fiscal code (Cod Unic de Înregistrare / Cod de
		// Identificare Fiscală), with or without the leading "RO". Validate
		// the published check-digit algorithm.
		return mod11Verdict(roCUIValid(extractDigits(value)))
	case "CIF":
		// Spain's CIF (letter + 7 digits + check char) — a DIFFERENT
		// identifier from Romania's CUI/CIF above, despite the shared
		// "CIF" name (Romania's own regex pattern in imprint_vat.go
		// matches "CUI"/"CIF" as label synonyms but always reports
		// Kind="CUI"; imprint_vat.go's separate Spain pattern reports
		// Kind="CIF"). These used to be lumped into one switch case here,
		// which ran every Spanish CIF through Romania's roCUIValid — wrong
		// format, wrong algorithm, would have silently downgraded every
		// real Spanish CIF to checksum_invalid. Real evidence:
		// eurostarshotels.com's real aviso legal CIF (B64930910).
		// onlyAlnum(value) glues the 3-letter "CIF"/"NIF" label onto the
		// front of the 9-char code, so the trailing 9 alnum characters are
		// always the code itself.
		if len(digits) < 9 {
			return formatValid
		}
		return mod11Verdict(esCIFValid(digits[len(digits)-9:]))
	case "PartitaIVA":
		// Italy's Partita IVA — 11 digits, Luhn-valid (same algorithm
		// validateVAT's IT case already uses for the "IT"-prefixed VAT
		// form). Real evidence: simanova.it's real note legali page
		// publishes "Partita IVA" / "04869580615" as a labelled identifier
		// (not the "IT"-prefixed VAT form vatPatterns' VAT/IT entry
		// matches), which this function previously had no case for at all
		// and silently fell through to formatValid without even checking
		// the checksum.
		return mod11Verdict(luhnValid(extractDigits(value)))
	case "NIP":
		// Poland's domestic NIP is exactly the same 10-digit body and
		// weighted-mod-11 checksum as validateVAT's "PL" case (plVATValid)
		// — it's the same number, just written without the "PL" prefix on
		// a domestic imprint. Real evidence: neptun.orlen.pl's real NIP
		// (5252855028) validates correctly.
		return mod11Verdict(plVATValid(extractDigits(value)))
	case "Adószám":
		// Hungary's domestic Adószám is 11 digits ("########-#-##"), but
		// only the first 8 (the same core the "HU"-prefixed EU VAT pattern
		// matches) carry the check digit — the trailing "-#-##" is a
		// VAT-status code and county code, not part of the checksum. See
		// huAdoszamValid's doc comment for the algorithm and real-evidence
		// confirmation.
		d := extractDigits(value)
		if len(d) < 8 {
			return formatValid
		}
		return mod11Verdict(huAdoszamValid(d[:8]))
	case "ΑΦΜ":
		// Greece's domestic ΑΦΜ is exactly the same 9-digit body as
		// validateVAT's "EL" case (grVATValid) below — it's the same
		// number, just written without the "EL" country prefix on a
		// domestic imprint, same architecture as NIP/Adószám above.
		return validateVAT("EL", extractDigits(value))
	case "ЕИК":
		// Bulgaria's domestic ЕИК is exactly the same 9-digit body as
		// validateVAT's "BG" case (bgEIKValid) below — it's the same
		// number, just written without the "BG" prefix on a domestic
		// imprint, same architecture as ΑΦΜ/Adószám above.
		return validateVAT("BG", extractDigits(value))
	case "OIB":
		// Croatia's domestic OIB is exactly the same 11-digit body as
		// validateVAT's "HR" case (hrOIBValid) below — it's the same
		// number, just written without the "HR" prefix on a domestic
		// imprint, same architecture as ЕИК/ΑΦΜ/Adószám above.
		return validateVAT("HR", extractDigits(value))
	case "Įmonės kodas":
		// Lithuania's Įmonės kodas is register-only, NOT VAT-equivalent
		// (see this Kind's vatPatterns doc comment) — its own dedicated
		// checksum, unlike OIB/ЕИК/ΑΦΜ above which route through
		// validateVAT since they share their country's VAT body.
		return mod11Verdict(ltCompanyCodeValid(extractDigits(value)))
	case "NIPC":
		// Portugal's NIPC (legal-entity ID) shares the same 9-digit body
		// and weighted-mod-11 checksum as validateVAT's "PT" case
		// (ptVATValid) — same architecture as NIP/PL above. Real evidence:
		// urbana.com.pt's real NIPC (508980186).
		return mod11Verdict(ptVATValid(extractDigits(value)))
	case "CVR":
		// Denmark's CVR (Det Centrale Virksomhedsregister) business-
		// register number shares the same 8-digit body and weighted-mod-11
		// checksum as validateVAT's "DK" case (dkVATValid) — the
		// DK-prefixed VAT number IS literally "DK" + this CVR body with no
		// extra check digit. Same architecture as NIP/NIPC above. Real
		// evidence: kims.dk's real CVR (15233877) and webshop.dn.dk's real
		// CVR (60804214), both independently found during the same search
		// and both verified to pass this checksum.
		return mod11Verdict(dkVATValid(extractDigits(value)))
	case "Y-tunnus":
		// Finland's Y-tunnus (Business ID) shares the same 8-digit body
		// and weighted-mod-11 checksum as validateVAT's "FI" case
		// (fiVATValid) — the FI-prefixed VAT number IS literally "FI" +
		// this Y-tunnus body with its hyphen removed. Same architecture as
		// CVR/DK above. Real evidence: finnprotec.fi's real Y-tunnus
		// (1938183-5), verified to pass this checksum.
		return mod11Verdict(fiVATValid(extractDigits(value)))
	case "OrgNr":
		// Norway's organisasjonsnummer had NO checksum-validation case at
		// all before this fix — it always fell through to the default
		// formatValid below, despite the published mod-11 algorithm being
		// well documented (see norwayOrgNrValid's doc comment). Real
		// evidence: japanphoto.no's real org.nr (965321039), verified to
		// pass.
		return mod11Verdict(norwayOrgNrValid(extractDigits(value)))
	case "IČO":
		// Czechia's IČO had NO checksum-validation case at all before this
		// fix — see czICOValid's doc comment for the algorithm and the
		// real-evidence confirmation (onlineshop.cz's real IČO, 24717509).
		return mod11Verdict(czICOValid(extractDigits(value)))
	case "ABN":
		if abnValid(extractDigits(value)) {
			return checksumValid
		}
		return checksumInvalid
	case "ACN":
		if acnValid(extractDigits(value)) {
			return checksumValid
		}
		return checksumInvalid
	case "SIREN", "SIRET":
		d := extractDigits(value)
		// SIRET = SIREN(9) + NIC(5); validate the whole 14 or the 9.
		if len(d) == 14 && luhnValid(d) {
			return checksumValid
		}
		if len(d) >= 9 && luhnValid(d[:9]) {
			return checksumValid
		}
		return checksumInvalid
	}
	return formatValid
}

// cleanIdentifierValue strips a matched identifier down to just its code,
// discarding the label text (and any embedded whitespace/newlines) the
// vatPatterns regex had to capture alongside it to anchor the match at all.
// Needed specifically for label-anchored patterns whose regex spans from
// the label to the code (PartitaIVA, CIF) — unlike a plain "COUNTRYCODE +
// digits" VAT pattern, which matches nothing but the code itself. Real
// evidence: simanova.it's Partita IVA regex hit captured the literal string
// "Partita IVA\n\n\n\n04869580615" (label, plus the blank lines
// stripTagsLines left between the two separate <p> elements) — storing that
// verbatim as the emitted `vat` field would be visibly wrong output, not
// just an internal formatting nit.
func cleanIdentifierValue(kind, value string) string {
	switch kind {
	case "PartitaIVA":
		d := extractDigits(value)
		if len(d) > 11 {
			d = d[len(d)-11:]
		}
		return d
	case "CIF":
		a := onlyAlnum(value)
		if len(a) > 9 {
			a = a[len(a)-9:]
		}
		return a
	case "NIP":
		d := extractDigits(value)
		if len(d) > 10 {
			d = d[len(d)-10:]
		}
		return d
	case "NIPC":
		d := extractDigits(value)
		if len(d) > 9 {
			d = d[len(d)-9:]
		}
		return d
	case "Adószám":
		d := extractDigits(value)
		if len(d) > 11 {
			d = d[len(d)-11:]
		}
		return d
	case "ΑΦΜ":
		d := extractDigits(value)
		if len(d) > 9 {
			d = d[len(d)-9:]
		}
		return d
	case "ЕИК":
		d := extractDigits(value)
		if len(d) > 9 {
			d = d[len(d)-9:]
		}
		return d
	case "OIB":
		d := extractDigits(value)
		if len(d) > 11 {
			d = d[len(d)-11:]
		}
		return d
	case "VAT Number":
		// Cyprus's domestic form necessarily swallows its own anchoring
		// label text ("VAT registration number 10141156Y" — see this
		// Kind's vatPatterns doc comment in imprint_vat.go) — same
		// trailing-slice cleanup CIF already uses for the same reason.
		a := onlyAlnum(value)
		if len(a) > 9 {
			a = a[len(a)-9:]
		}
		return a
	case "VAT":
		// Now that BE/FR (and potentially others) tolerate optional
		// internal spaces to match real-world formatting, the raw match
		// can carry them through into the emitted field ("BE 0704742612")
		// unless stripped here. onlyAlnum is a no-op on the many VAT
		// patterns that never allowed spaces in the first place (already
		// clean), so this is a safe superset fix, not a behavior change
		// for those.
		return onlyAlnum(value)
	}
	return value
}

// isVATLikeIdentifierKind reports whether kind is a VAT-equivalent
// identifier — the plain "VAT" patterns plus PartitaIVA (Italy) and CIF
// (Spain), which serve the same VAT-identification role domestically even
// though they're captured by their own label-anchored patterns rather than
// vatPatterns' generic "COUNTRYCODE + digits" VAT entries.
func isVATLikeIdentifierKind(kind string) bool {
	return kind == "VAT" || kind == "PartitaIVA" || kind == "CIF" || kind == "NIP" || kind == "NIPC" || kind == "Adószám" || kind == "ΑΦΜ" || kind == "ЕИК" || kind == "OIB" || kind == "VAT Number"
}

// singleCountryIdentifierKind returns the ISO-3166-1 alpha-2 country a
// label-anchored identifier Kind unambiguously belongs to, or "" if the
// Kind isn't one of these. Unlike the generic "VAT" kind (whose 2-letter
// country prefix is embedded in the value itself — see vatCountryHint),
// these label-anchored patterns (imprint_vat.go) match bare digits with no
// country prefix at all, but the LABEL itself only ever exists in one
// country's national system — a Partita IVA or REA number cannot possibly
// be Romanian, a KvK number cannot possibly be French. Used to let
// ground-truth government-identifier evidence override a merely-
// probabilistic legal-form-suffix country guess — see the real-evidence
// comment at this function's call site in imprint.go.
func singleCountryIdentifierKind(kind string) string {
	switch kind {
	case "PartitaIVA", "REA":
		return "IT"
	case "Hoja", "CIF":
		return "ES"
	case "SIRET", "SIREN":
		return "FR"
	case "KvK":
		return "NL"
	case "NIP", "KRS", "REGON":
		return "PL"
	case "NIPC":
		return "PT"
	case "CRO":
		return "IE"
	case "RCS":
		return "LU"
	case "Organisationsnummer":
		return "SE"
	case "CVR":
		return "DK"
	case "Y-tunnus":
		return "FI"
	case "OrgNr":
		return "NO"
	case "Cégjegyzékszám", "Adószám":
		// Both Hungary-only: "Cégjegyzékszám" (company-register number) and
		// "Adószám" (domestic tax number, VAT-equivalent — see
		// isVATLikeIdentifierKind) are Hungarian-language terms with no
		// other country's business-register system using either label.
		// Real evidence: szatmari-izek.shop.hu's real Impresszum page.
		return "HU"
	case "Γ.Ε.ΜΗ.":
		// Greece's General Commercial Registry (Γενικό Εμπορικό Μητρώο) is
		// a specific Greek institution established by Greek law 3419/2005
		// — unlike "ΑΦΜ" just below (deliberately NOT a case here), this
		// names a jurisdiction-specific REGISTRY, not a generic Greek-
		// language label a different Greek-speaking jurisdiction (Cyprus)
		// might also use for its own, differently-run companies registrar.
		// Real evidence: thikishop.gr's real Όροι Χρήσης page.
		return "GR"
		// Deliberately NOT a case here: "ΑΦΜ" (Greece, round 18) is not
		// given the same single-country treatment. Greek is ALSO an
		// official language of Cyprus, and "ΑΦΜ" (literally "tax registry
		// number") reads as a generic administrative term rather than a
		// named Greek-only institution the way "Γ.Ε.ΜΗ." above is — the
		// same Czech/Slovak "IČO" caution from round 16 applies here:
		// mapping it to "GR" could mis-flag a future real Cypriot page
		// once Cyprus gets its own real-evidence round. Not needed for
		// this round's fixture anyway: the native Greek suffix forms
		// (Ι.Κ.Ε./Α.Ε./Ε.Π.Ε./Ο.Ε., imprint_suffix.go) are GR-unambiguous
		// on their own.
	case "ЕИК":
		// Bulgaria's ЕИК, unlike Greece's ΑΦΜ, has no EU-member sharing
		// risk to guard against: Bulgaria is the only EU member state
		// using Cyrillic script as an official alphabet, so there is no
		// sibling jurisdiction (the way Cyprus shares Greek, or Slovakia
		// shared Czechoslovakia's IČO) that could also use this exact
		// Cyrillic term for its own, different registry. Real evidence:
		// cressi.bg's real Общи условия page.
		return "BG"
	case "OIB", "MBS":
		// Both Croatia-only: "OIB" (Osobni identifikacijski broj) and
		// "MBS" (Matični broj subjekta, the court-register entry number)
		// are Croatian-specific administrative terms — unlike the
		// suffixTable's "d.o.o." collision risk with Slovenia (see that
		// entry's doc comment), no other country uses either of these
		// exact labels for its own registry. Real evidence: mako.hr's
		// real Opći uvjeti page.
		return "HR"
	case "Matična številka":
		// Slovenia's own company-register-number term — this is the
		// ACTUAL Croatia/Slovenia "d.o.o." collision round 20 anticipated
		// (see suffixTable's doc comment for that entry): a real Slovenian
		// page names its entity with the same "d.o.o." suffix Croatia
		// already claims in suffixTable, but this ground-truth register
		// identifier is unambiguously Slovenian, correcting the country
		// the same way OIB/MBS/CUI/etc. already do for their own
		// collisions. Real evidence: sgermobil.si's real Splošni pogoji
		// page.
		return "SI"
	case "HE", "VAT Number":
		// Both Cyprus-only: "HE" is the Registrar of Companies' own
		// file-number prefix (not used by any other country's registry),
		// and "VAT Number"'s match REQUIRES the 8-digit+letter shape that
		// is itself distinctively Cypriot among this table's VAT formats
		// (see that Kind's vatPatterns doc comment) — the English "VAT
		// ... number" wording alone is generic, but combined with that
		// shape requirement it only ever matches a real Cypriot value.
		// Real evidence: epic.com.cy's real eStore Terms & Conditions
		// page.
		return "CY"
	case "Įmonės kodas":
		// Lithuanian-language term, no other country's registry uses it.
		// Real evidence: grupinispirkimas.lt's real Taisyklės page.
		return "LT"
	case "CUI":
		// Romania's Cod Unic de Înregistrare — imprint_vat.go's "CUI"
		// vatPatterns entry matches BOTH the "CUI" and "CIF" labels but
		// always reports Kind="CUI" (Spain's own, separately-patterned CIF
		// reports Kind="CIF" — see the collision note in validateIdentifier
		// below), so Kind="CUI" is unambiguous evidence of Romania. Real
		// evidence: emag.ro's real Termeni si conditii page names
		// "DANTE INTERNATIONAL S.A." — a bare "S.A." suffix collides with
		// France's low-confidence entry in suffixTable and won
		// Country="FR" as best candidate, even though the SAME page's real
		// "cod unic de inregistrare RO 14399840" / footer "CUI: 14399840"
		// is unambiguously Romanian. Before this case existed, the
		// singleCountryIdentifierKind correction loop in imprint.go never
		// fired for Romania's own national identifier at all — CUI was
		// wired into vatPatterns and validateIdentifier's checksum switch
		// (roCUIValid) since this repo's first EU-expansion round, but,
		// like OrgNr before round 14, was never connected to the
		// country-correction path.
		return "RO"
	}
	// Deliberately NOT a case here: "IČO" (Czechia, added round 16) is NOT
	// single-country the way the cases above are. Former Czechoslovakia
	// left BOTH Czechia and Slovakia using the identical "IČO" label for
	// their (different-checksum) domestic business ID — unlike KvK/CRO/
	// CVR/Y-tunnus/OrgNr/CUI, which only ever exist in one jurisdiction's
	// own national system. Mapping Kind="IČO" to "CZ" here would silently
	// mis-flag a future real Slovak page (once Slovakia gets its own
	// real-evidence round) as Czech. This round's real evidence
	// (onlineshop.cz) didn't need this override anyway — its "s.r.o."
	// suffix is CZ-unambiguous in the current suffixTable (no Slovak
	// "s.r.o." entry exists yet) — so there is no real-evidence basis to
	// add it, and doing so pre-emptively would risk exactly the kind of
	// overclaiming this methodology avoids.
	return ""
}

// validateVAT dispatches on the 2-letter VAT prefix (or the country code when
// the value omits it) to the per-member-state check-digit routine. Returns
// formatValid for member states whose algorithm we don't implement.
func validateVAT(country, raw string) validity {
	raw = strings.ToUpper(raw)
	cc := country
	if len(raw) >= 2 && isAlpha(raw[0]) && isAlpha(raw[1]) {
		cc = raw[:2]
		raw = raw[2:]
	}
	switch cc {
	case "DE":
		return mod11Verdict(deVATValid(raw))
	case "NL":
		// NL VAT is 9 digits + "B" + 2 digits; the 9-digit body is mod-11 —
		// for companies (B.V./N.V., whose body is their RSIN). Sole traders
		// (eenmanszaak/zzp) are a real, separate case: since 2020-01-01 the
		// Belastingdienst issues them a "btw-identificatienummer" that is
		// deliberately NOT derived from the owner's BSN (a privacy reform —
		// the old BSN-derived number leaked the owner's citizen-service
		// number on every invoice) and has no published check-digit
		// algorithm at all — VIES-based validators confirm this and treat
		// post-2020 sole-trader numbers as opaque, checked only by a live
		// registry call. A body that fails the elfproef mod-11 below is NOT
		// necessarily invalid, then: it may simply be one of these. Real
		// evidence: simpelbootverhuurutrecht.nl (a real Dutch eenmanszaak)
		// publishes NL005444107B41, which fails elfproef outright despite
		// being the business's own genuine, live BTW-id — its body lands on
		// elfproef remainder 10, the one remainder the classical algorithm
		// cannot encode as any check digit at all (see nlVATIndeterminate).
		// Falling back to formatValid only for that specific ambiguous
		// remainder avoids false-flagging this real population of Dutch
		// one-person businesses — same precedent as the LU/GB case below —
		// while any other definite digit mismatch still reports
		// checksumInvalid, so a plain typo'd or fabricated NL VAT is still
		// caught.
		body := raw
		if i := strings.IndexByte(raw, 'B'); i == 9 {
			body = raw[:9]
		}
		if nlVATValid(body) {
			return checksumValid
		}
		if nlVATIndeterminate(body) {
			return formatValid
		}
		return checksumInvalid
	case "IT":
		return mod11Verdict(luhnValid(extractDigitsStr(raw)))
	case "FR":
		// FR = 2 check chars + 9-digit SIREN. Validate the SIREN (Luhn).
		if len(raw) == 11 {
			return mod11Verdict(luhnValid(raw[2:]))
		}
		return formatValid
	case "ES":
		return mod11Verdict(esCIFValid(raw))
	case "AT":
		// ATU + 8 digits; last is a check digit (weighted mod-10).
		body := strings.TrimPrefix(raw, "U")
		return mod11Verdict(atVATValid(body))
	case "BE":
		// 10-digit enterprise number (old 9-digit form is zero-padded);
		// mod-97 over the leading 8 digits.
		return mod11Verdict(beVATValid(raw))
	case "PL":
		// NIP: 10 digits, weighted mod-11 over the first 9.
		return mod11Verdict(plVATValid(raw))
	case "SE":
		// 12-digit org number; trailing two are the sequence ("01"), the
		// leading 10 are Luhn-valid.
		return mod11Verdict(seVATValid(raw))
	case "DK":
		// CVR: 8 digits, weighted mod-11 (whole number ≡ 0).
		return mod11Verdict(dkVATValid(raw))
	case "FI":
		// Y-tunnus: 8 digits, weighted mod-11 (whole number ≡ 0).
		return mod11Verdict(fiVATValid(raw))
	case "PT":
		// NIF: 9 digits, weighted mod-11 over the first 8.
		return mod11Verdict(ptVATValid(raw))
	case "IE":
		// Old and new alphanumeric formats, both checked mod-23 → check letter.
		return mod11Verdict(ieVATValid(raw))
	case "RO":
		// CUI/CIF: 2–10 digits, weighted mod-11 over all but the last digit.
		return mod11Verdict(roCUIValid(raw))
	case "EL", "GR":
		// Greece's ΑΦΜ/VAT: 9 digits, weighted mod-11 over the first 8 —
		// see grVATValid's doc comment for the algorithm and real-evidence
		// confirmation. Both "EL" (the VAT prefix ISO uses for Greece) and
		// "GR" (the ordinary country code, in case a caller passes that
		// instead) are accepted here.
		return mod11Verdict(grVATValid(raw))
	case "BG":
		// Bulgaria's VAT number is 9 OR 10 digits — 9 for a legal entity
		// (its ЕИК/BULSTAT, this round's real evidence — see bgEIKValid's
		// doc comment for the algorithm), 10 for an individual (a
		// different number, the personal EGN, with a different published
		// algorithm this round has no real value to confirm). Only the
		// 9-digit legal-entity form is checksum-verified here; a 10-digit
		// match falls through to format_valid rather than risk a false
		// checksum_invalid on an algorithm we don't actually implement.
		if len(raw) != 9 {
			return formatValid
		}
		return mod11Verdict(bgEIKValid(raw))
	case "HR":
		// Croatia's OIB/VAT: 11 digits, ISO 7064 MOD 11-10 — see
		// hrOIBValid's doc comment for the algorithm and real-evidence
		// confirmation.
		return mod11Verdict(hrOIBValid(raw))
	case "LU", "GB":
		// Recognised format; algorithms are documented but we only claim
		// format_valid here to keep the false-"invalid" rate at zero.
		return formatValid
	}
	return formatValid
}

// --- per-country algorithms -------------------------------------------------

// deVATValid verifies a German USt-IdNr (9 digits) using the documented
// "Modulo 11/10" iterative algorithm (the same one the Bundeszentralamt für
// Steuern publishes). p starts at 10; for each of the first 8 digits,
// m = (digit + p) mod 10 (10 if 0); p = (2*m) mod 11. The check digit is
// (11 - p) mod 10.
func deVATValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	p := 10
	for i := 0; i < 8; i++ {
		m := (int(s[i]-'0') + p) % 10
		if m == 0 {
			m = 10
		}
		p = (2 * m) % 11
	}
	check := (11 - p) % 10
	return check == int(s[8]-'0')
}

// nlVATValid verifies the 9-digit body of a Dutch BTW/RSIN number with the
// classic "elfproef" (eleven-test): weights 9..2 over the first 8 digits,
// the weighted sum mod 11 must equal the 9th digit (and never 10).
func nlVATValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * (9 - i)
	}
	check := sum % 11
	if check == 10 {
		return false
	}
	return check == int(s[8]-'0')
}

// nlVATIndeterminate reports whether s's elfproef weighted sum lands on a
// remainder of exactly 10 — a value the classical algorithm cannot encode
// as any single check digit (0-9) at all, so nlVATValid's "false" there
// means "the elfproef doesn't apply", not "definitely wrong". Since
// 2020-01-01 the Belastingdienst issues sole-trader (eenmanszaak/zzp)
// btw-identificatienummers independently of elfproef (a privacy reform, not
// derived from the owner's BSN), so these land on any remainder uniformly —
// including 10, about 1-in-11 of the time — with no way to tell them apart
// from an ordinary company number by structure alone. Real evidence:
// simpelbootverhuurutrecht.nl (a genuine Dutch eenmanszaak) publishes
// NL005444107B41, whose body lands on remainder 10. Used by validateVAT's
// NL case to fall back to formatValid (ambiguous) only for this specific
// remainder, while a definite digit mismatch elsewhere still reports
// checksumInvalid — a plain typo'd or fabricated NL VAT (any other wrong
// remainder) is still caught. Sibling fix in the go_legal_entity repo,
// where a pre-existing test for exactly this case (a synthetic broken NL
// VAT that must stay checksumInvalid) caught the previous blanket
// formatValid fallback as a real regression.
func nlVATIndeterminate(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * (9 - i)
	}
	return sum%11 == 10
}

// atVATValid verifies an Austrian UID (8 digits after the "U") using the
// documented weighted checksum: weights 1,2,1,2,1,2,1 over the first 7
// digits, summing digit-sums of the products; check = (96 - sum) mod 10.
func atVATValid(s string) bool {
	if len(s) != 8 || !allDigits(s) {
		return false
	}
	weights := []int{1, 2, 1, 2, 1, 2, 1}
	sum := 0
	for i := 0; i < 7; i++ {
		p := int(s[i]-'0') * weights[i]
		sum += p/10 + p%10
	}
	check := (96 - sum) % 10
	if check < 0 {
		check += 10
	}
	return check == int(s[7]-'0')
}

// plVATValid verifies a Polish NIP (10 digits) using the documented weighted
// modulo-11 algorithm: weights 6,5,7,2,3,4,5,6,7 over the first 9 digits, the
// weighted sum mod 11 must equal the 10th digit (and is never 10).
func plVATValid(s string) bool {
	if len(s) != 10 || !allDigits(s) {
		return false
	}
	weights := []int{6, 5, 7, 2, 3, 4, 5, 6, 7}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	check := sum % 11
	if check == 10 {
		return false
	}
	return check == int(s[9]-'0')
}

// seVATValid verifies a Swedish VAT number. The 12-digit form is a 10-digit
// organisationsnummer followed by a 2-digit sequence (normally "01"); the
// leading 10 digits carry a Luhn check digit. (Skatteverket / VIES.)
func seVATValid(s string) bool {
	if len(s) != 12 || !allDigits(s) {
		return false
	}
	return luhnValid(s[:10])
}

// dkVATValid verifies a Danish CVR number (8 digits) with the published
// weighted modulo-11: weights 2,7,6,5,4,3,2,1 across all 8 digits, the
// weighted sum must be divisible by 11. The first digit is never 0.
func dkVATValid(s string) bool {
	if len(s) != 8 || !allDigits(s) || s[0] == '0' {
		return false
	}
	weights := []int{2, 7, 6, 5, 4, 3, 2, 1}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	return sum%11 == 0
}

// norwayOrgNrValid verifies a Norwegian organisasjonsnummer (9 digits)
// using the published Brønnøysundregistrene weighted modulo-11: weights
// 3,2,7,6,5,4,3,2 across the first 8 digits; the check digit (9th digit) is
// (11 - remainder) when the remainder is not 0 or 1, 0 when the remainder
// is 0, and the number has no valid check digit at all when the remainder
// is 1 (rejected as invalid here, same as the elsewhere-documented "10 is
// unencodable" shape of these mod-11 schemes). Real evidence:
// japanphoto.no's real org.nr (965321039) verifies correctly under this
// algorithm.
func norwayOrgNrValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	weights := []int{3, 2, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	rem := sum % 11
	if rem == 1 {
		return false
	}
	check := 0
	if rem != 0 {
		check = 11 - rem
	}
	return check == int(s[8]-'0')
}

// czICOValid verifies a Czech IČO (Identifikační číslo osoby, 8 digits)
// using the published weighted modulo-11 algorithm: weights 8,7,6,5,4,3,2
// applied to the first 7 digits; remainder 0 maps to check digit 1,
// remainder 1 maps to check digit 0, any other remainder r maps to
// 11-r. Confirmed against onlineshop.cz's real IČO (24717509): weighted
// sum 134, 134 mod 11 = 2, check digit 11-2 = 9 — matches the real value's
// own trailing digit.
func czICOValid(s string) bool {
	if len(s) != 8 || !allDigits(s) {
		return false
	}
	weights := []int{8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	rem := sum % 11
	check := 11 - rem
	switch rem {
	case 0:
		check = 1
	case 1:
		check = 0
	}
	return check == int(s[7]-'0')
}

// huAdoszamValid verifies the 8-digit core of a Hungarian Adószám using the
// published weighted modulo-10 algorithm: weights 9,7,3,1,9,7,3 applied to
// the first 7 digits; check digit = (10 - (sum mod 10)) mod 10. Confirmed
// against TWO independent real values found on szatmari-izek.shop.hu's real
// Impresszum page: 13495413 (weighted sum 127, 127 mod 10 = 7, check
// (10-7)%10 = 3 — matches the real value's own 8th digit) and 23495919
// (weighted sum 171, 171 mod 10 = 1, check (10-1)%10 = 9 — also matches).
func huAdoszamValid(s string) bool {
	if len(s) != 8 || !allDigits(s) {
		return false
	}
	weights := []int{9, 7, 3, 1, 9, 7, 3}
	sum := 0
	for i := 0; i < 7; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	check := (10 - (sum % 10)) % 10
	return check == int(s[7]-'0')
}

// grVATValid verifies a Greek ΑΦΜ/VAT (9 digits) using the published
// weighted modulo-11 algorithm: weights 2^8,2^7,...,2^1 (256, 128, 64, 32,
// 16, 8, 4, 2) applied to the first 8 digits; the weighted sum mod 11 is
// the check digit directly (a remainder of 10 maps to check digit 0, the
// same "exceptional remainder" shape as several other checksums in this
// file — not itself hit by the one real value available to confirm this
// round, so treated as inferred-standard rather than real-evidence-tested).
// Confirmed against thikishop.gr's real ΑΦΜ (800617296): weighted sum 2338,
// 2338 mod 11 = 6 — matches the real value's own 9th digit.
func grVATValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	weights := []int{256, 128, 64, 32, 16, 8, 4, 2}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	check := sum % 11
	if check == 10 {
		check = 0
	}
	return check == int(s[8]-'0')
}

// bgEIKValid verifies a Bulgarian ЕИК/BULSTAT (9 digits) using the
// published two-pass weighted modulo-11 algorithm over the first 8 digits:
// pass 1 uses weights 1,2,3,4,5,6,7,8, remainder mod 11 is the check digit
// directly; if that remainder is 10 (not itself a valid single check
// digit), pass 2 re-weights with 3,4,5,6,7,8,9,10, ITS remainder mod 11 is
// the check digit instead (11 mapped to 0, same "exceptional remainder"
// shape as grVATValid). Confirmed against cressi.bg's real ЕИК
// (201845795): pass-1 weighted sum 208, 208 mod 11 = 10 (triggers pass 2),
// pass-2 weighted sum 280, 280 mod 11 = 5 — matches the real value's own
// 9th digit.
func bgEIKValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	weighted := func(weights []int) int {
		sum := 0
		for i := 0; i < 8; i++ {
			sum += int(s[i]-'0') * weights[i]
		}
		return sum
	}
	check := weighted([]int{1, 2, 3, 4, 5, 6, 7, 8}) % 11
	if check == 10 {
		check = weighted([]int{3, 4, 5, 6, 7, 8, 9, 10}) % 11
		if check == 10 {
			check = 0
		}
	}
	return check == int(s[8]-'0')
}

// hrOIBValid verifies a Croatian OIB (11 digits) using the published ISO
// 7064 MOD 11-10 algorithm over the first 10 digits: starting from
// remainder 10, for each digit in turn compute
// r = (digit + r) mod 10 (mapping a 0 result to 10), then r = (r*2) mod 11;
// the final check digit is (11 - r) mod 10. Confirmed against mako.hr's
// real OIB (31448356613): the running remainder after all 10 digits is 8,
// check digit (11-8) mod 10 = 3 — matches the real value's own 11th digit.
func hrOIBValid(s string) bool {
	if len(s) != 11 || !allDigits(s) {
		return false
	}
	r := 10
	for i := 0; i < 10; i++ {
		d := int(s[i] - '0')
		r = (d + r) % 10
		if r == 0 {
			r = 10
		}
		r = (r * 2) % 11
	}
	check := (11 - r) % 10
	return check == int(s[10]-'0')
}

// ltCompanyCodeValid verifies a Lithuanian Įmonės kodas (9 digits) using
// the published two-pass weighted modulo-11 algorithm over the first 8
// digits: pass 1 uses weights 1,2,3,4,5,6,7,8; if that remainder is 10
// (not itself a valid single check digit), pass 2 re-weights with
// 3,4,5,6,7,8,9,10 (remainder 10 there maps to check digit 0) — the same
// two-pass shape as Bulgaria's bgEIKValid. Confirmed against TWO
// independent real values found the same day: grupinispirkimas.lt's real
// Įmonės kodas (302983374: weighted sum 180, 180 mod 11 = 4, matches the
// real value's own 9th digit) and bigbox.lt's real Įmonės kodas
// (302662379: weighted sum 152, 152 mod 11 = 9, also matches) — neither
// happened to hit the pass-2 fallback, so that branch is inferred-standard
// rather than real-evidence-tested, same caveat as bgEIKValid's.
func ltCompanyCodeValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	weighted := func(weights []int) int {
		sum := 0
		for i := 0; i < 8; i++ {
			sum += int(s[i]-'0') * weights[i]
		}
		return sum
	}
	check := weighted([]int{1, 2, 3, 4, 5, 6, 7, 8}) % 11
	if check == 10 {
		check = weighted([]int{3, 4, 5, 6, 7, 8, 9, 10}) % 11
		if check == 10 {
			check = 0
		}
	}
	return check == int(s[8]-'0')
}

// fiVATValid verifies a Finnish Y-tunnus (8 digits) using the documented
// weighted modulo-11: weights 7,9,10,5,8,4,2,1 across all 8 digits, the
// weighted sum must be divisible by 11 (a remainder of 1 is invalid, which the
// ≡0 test already excludes).
func fiVATValid(s string) bool {
	if len(s) != 8 || !allDigits(s) {
		return false
	}
	weights := []int{7, 9, 10, 5, 8, 4, 2, 1}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * weights[i]
	}
	return sum%11 == 0
}

// ptVATValid verifies a Portuguese NIF (9 digits) with the published weighted
// modulo-11: weights 9..2 over the first 8 digits; check = 11 - (sum mod 11),
// and a check of 10 or 11 maps to 0. The first digit is never 0.
func ptVATValid(s string) bool {
	if len(s) != 9 || !allDigits(s) || s[0] == '0' {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * (9 - i)
	}
	check := 11 - (sum % 11)
	if check >= 10 {
		check = 0
	}
	return check == int(s[8]-'0')
}

// ieCheckChars is the Irish mod-23 alphabet: index 0 → 'W', 1 → 'A' … 22 → 'V'.
const ieCheckChars = "WABCDEFGHIJKLMNOPQRSTUV"

// atoiN parses a non-negative decimal string, returning (value, errored).
func atoiN(s string) (int, bool) {
	if !allDigits(s) {
		return 0, true
	}
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n, false
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// abnValid verifies an Australian Business Number (11 digits) using the ATO's
// published algorithm: subtract 1 from the leading digit, apply weights
// 10,1,3,5,7,9,11,13,15,17,19, and the weighted sum must be divisible by 89.
func abnValid(s string) bool {
	if len(s) != 11 || !allDigits(s) {
		return false
	}
	weights := []int{10, 1, 3, 5, 7, 9, 11, 13, 15, 17, 19}
	sum := 0
	for i := 0; i < 11; i++ {
		d := int(s[i] - '0')
		if i == 0 {
			d--
		}
		sum += d * weights[i]
	}
	return sum%89 == 0
}

// acnValid verifies an Australian Company Number (9 digits): weights 8..0 over
// the first 8 digits, complement = (10 - (sum mod 10)) mod 10 must equal the
// 9th digit. (ASIC's published modulus-10 algorithm.)
func acnValid(s string) bool {
	if len(s) != 9 || !allDigits(s) {
		return false
	}
	sum := 0
	for i := 0; i < 8; i++ {
		sum += int(s[i]-'0') * (8 - i)
	}
	check := (10 - (sum % 10)) % 10
	return check == int(s[8]-'0')
}

// beVATValid verifies a Belgian enterprise number / VAT (BTW-nr). The number
// is 10 digits (the historic 9-digit form is left-padded with a 0); the
// trailing 2 digits are a mod-97 check over the leading 8: the check equals
// 97 - (firstEight mod 97). (Documented FPS Finance / VIES algorithm.)
func beVATValid(s string) bool {
	if len(s) == 9 {
		s = "0" + s
	}
	if len(s) != 10 || !allDigits(s) {
		return false
	}
	body, err := atoiN(s[:8])
	if err {
		return false
	}
	check, err2 := atoiN(s[8:])
	if err2 {
		return false
	}
	return (97 - (body % 97)) == check
}

// ieVATValid verifies an Irish VAT number in both documented shapes:
//   - new style: 7 digits + a check letter (weights 8..2 over the 7 digits);
//   - old style: digit, a filler ([A-Z+*]), 5 digits, check letter — the body
//     reorders to digits[2:7]+digit[0] with weights 7..2.
//
// In both, check = mod-23 index into ieCheckChars. A trailing second letter
// ([A-W]) for "VAT group" registrations is permitted and ignored.
func ieVATValid(s string) bool {
	if len(s) == 9 && isAlpha(s[8]) {
		s = s[:8] // drop the optional VAT-group second letter
	}
	if len(s) != 8 {
		return false
	}
	check := s[7]
	if !isAlpha(check) {
		return false
	}
	idx := strings.IndexByte(ieCheckChars, check)
	if idx < 0 {
		return false
	}
	// New style: 7 leading digits.
	if allDigits(s[:7]) {
		sum := 0
		for i := 0; i < 7; i++ {
			sum += int(s[i]-'0') * (8 - i)
		}
		return idx == sum%23
	}
	// Old style: digit, filler, 5 digits, check letter.
	if isDigit(s[0]) && allDigits(s[2:7]) && (isAlpha(s[1]) || s[1] == '+' || s[1] == '*') {
		body := s[2:7] + string(s[0]) // 6 digits, weights 7..2
		sum := 0
		for i := 0; i < 6; i++ {
			sum += int(body[i]-'0') * (7 - i)
		}
		return idx == sum%23
	}
	return false
}

// esCIFValid verifies a Spanish CIF (legal-entity tax ID): a leading letter,
// 7 digits, and a control char that is either a digit or a letter depending
// on the leading-letter class. Implements the published algorithm: sum even
// positions, double odd positions (digit-sum each), control = (10 - (sum%10))
// %10; map to a letter for organisation classes that use letter controls.
func esCIFValid(s string) bool {
	if len(s) != 9 || !isAlpha(s[0]) {
		return false
	}
	digits := s[1:8]
	if !allDigits(digits) {
		return false
	}
	sum := 0
	for i := 0; i < 7; i++ {
		d := int(digits[i] - '0')
		if i%2 == 0 { // odd position (1-based) → double
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	ctrlNum := (10 - (sum % 10)) % 10
	got := s[8]
	letters := "JABCDEFGHI"
	switch s[0] {
	case 'A', 'B', 'E', 'H': // must be numeric control
		return int(got-'0') == ctrlNum
	case 'K', 'P', 'Q', 'S', 'N', 'W': // must be letter control
		return got == letters[ctrlNum]
	default: // C,D,F,G,J,R,U,V — either form accepted
		if got >= '0' && got <= '9' {
			return int(got-'0') == ctrlNum
		}
		return got == letters[ctrlNum]
	}
}

// roCUIValid verifies a Romanian CUI / CIF (Cod Unic de Înregistrare) using
// the published ANAF algorithm. The control key is 7,5,3,2,1,7,5,3,2 applied
// right-aligned to the body (all digits except the trailing check digit); the
// check digit is (sum*10) mod 11, mapped to 0 when that yields 10. The CUI body
// is 2–10 digits (the leading "RO" prefix, if present, is stripped by the
// caller). The first digit is never 0.
func roCUIValid(s string) bool {
	if len(s) < 2 || len(s) > 10 || !allDigits(s) || s[0] == '0' {
		return false
	}
	key := []int{7, 5, 3, 2, 1, 7, 5, 3, 2}
	body := s[:len(s)-1]
	check := int(s[len(s)-1] - '0')
	// Right-align the body against the 9-element key.
	sum := 0
	offset := len(key) - len(body)
	if offset < 0 {
		return false // body longer than the key — not a valid CUI shape
	}
	for i := 0; i < len(body); i++ {
		sum += int(body[i]-'0') * key[offset+i]
	}
	want := (sum * 10) % 11
	if want == 10 {
		want = 0
	}
	return want == check
}

// luhnValid runs the standard Luhn (mod-10) check over a digit string. Used by
// the French SIREN/SIRET and the Italian VAT body.
func luhnValid(s string) bool {
	if len(s) == 0 || !allDigits(s) {
		return false
	}
	sum := 0
	double := false
	for i := len(s) - 1; i >= 0; i-- {
		d := int(s[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// --- small helpers ----------------------------------------------------------

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// onlyAlnum strips everything except letters and digits, upper-cases letters.
func onlyAlnum(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// knownVATPrefixes are the EU member-state (+ GB) 2-letter VAT prefixes. EL is
// the VAT prefix for Greece (the country code is GR).
var knownVATPrefixes = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true, "DE": true,
	"DK": true, "EE": true, "EL": true, "ES": true, "FI": true, "FR": true,
	"GB": true, "HR": true, "HU": true, "IE": true, "IT": true, "LT": true,
	"LU": true, "LV": true, "MT": true, "NL": true, "PL": true, "PT": true,
	"RO": true, "SE": true, "SI": true, "SK": true,
}

// vatCountryHint returns the ISO country implied by a VAT string's 2-letter
// prefix, or "" when the string doesn't start with a recognised prefix.
func vatCountryHint(s string) string {
	u := strings.ToUpper(strings.TrimSpace(s))
	if len(u) >= 2 && knownVATPrefixes[u[:2]] {
		if u[:2] == "EL" {
			return "EL" // keep the VAT-prefix form the rest of the code uses
		}
		return u[:2]
	}
	return ""
}

// extractDigits returns the digit-only run from a value that may carry a
// label/prefix ("ABN 51 824 753 556" → "51824753556").
func extractDigits(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// extractDigitsStr returns only the digits of s (alias kept for readability at
// the VAT call site).
func extractDigitsStr(s string) string { return extractDigits(s) }
