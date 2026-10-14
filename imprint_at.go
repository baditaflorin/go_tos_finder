package main

import (
	"regexp"
	"strings"
)

// Austrian imprint refinements (real-evidence round, 2026-10-14, 200 live .at
// domains re-enriched: 45 Austrian GmbHs got the German ruleset and 70 of 105
// Austrian-ruleset imprints had no address).
//
// 1. refineDEATCountry: a bare "GmbH"/"AG" suffix is DE-or-AT ambiguous, but it
//    used to outrank every local signal (ages.at, graz.at, herold.at: 4-digit
//    Vienna/Graz postcodes, "Handelsgericht Wien", an .at host, all labelled DE).
// 2. extractATPostalAddress: addresses were only searched right after a detected
//    legal-entity name, so associations, public bodies and universities (no
//    legal-form suffix, hence no entity) never got one although the page prints
//    it in one of three shapes: "Street 1, 1010 Wien" on one line, street and
//    "1010 Wien" on two lines, or a labelled "Anschrift: ...".
// 3. extractMedieninhaber: Mediengesetz §25 requires naming the Medieninhaber
//    (media owner); that is the legal identity of an association or institution.

var (
	// Austrian Firmenbuch number ("FN 233171 z") or an Austrian register court.
	atRegisterRE = regexp.MustCompile(`(?i)\bFN\s*\d{1,6}\s*[a-z]\b|\bFirmenbuch|\bHandelsgericht\b|\bLandesgericht\b|\bKreisgericht\b`)
	// German commercial register ("HRB 12345") or a German register court.
	deRegisterRE = regexp.MustCompile(`(?i)\bHR\s?[AB]\s*\d|\bHandelsregister\b|\bAmtsgericht\b|\bLandgericht\b`)
	// "1010 Wien", "A-1100 Wien", "4040 Linz" (4-digit Austrian postcode + a capitalised place).
	atPostcodeRE = regexp.MustCompile(`(?:^|[\s,])(?:A[-\s])?[1-9]\d{3}\s+\p{Lu}[\p{L}.\-]+`)
	// German 5-digit PLZ + a capitalised place.
	dePostcodeRE = regexp.MustCompile(`(?:^|[\s,])(?:D[-\s])?\d{5}\s+\p{Lu}[\p{L}.\-]+`)

	// "Street 12", "Street 18/2/47", "Platz 1", "Hauptstrasse 35-43", optionally a label before it.
	atStreetPart = `\p{Lu}[\p{L}.'\-]*(?:[ \-]\p{L}[\p{L}.'\-]*){0,4}\s+\d{1,4}[a-zA-Z]?(?:\s?[-/]\s?\d{1,4}[a-zA-Z]?){0,3}`
	// "1010 Wien", "A-4040 Linz", "1045 Vienna" (one or two place words).
	atCityPart = `(?:A[-\s])?[1-9]\d{3}\s+\p{Lu}[\p{L}.\-]+(?:\s\p{Lu}[\p{L}.\-]+)?`

	atInlineAddrRE = regexp.MustCompile(`(` + atStreetPart + `)\s*,\s*(` + atCityPart + `)`)
	atStreetLineRE = regexp.MustCompile(`^(?:[^:]{0,40}:\s*)?(` + atStreetPart + `)$`)
	atCityLineRE   = regexp.MustCompile(`^(` + atCityPart + `)(?:\s*,?\s*(?:Österreich|Austria))?$`)

	medieninhaberRE = regexp.MustCompile(`(?i)^\s*(?:Medieninhaber(?:in)?(?:\s+und\s+Herausgeber(?:in)?)?|Herausgeber(?:in)?(?:\s+und\s+Medieninhaber(?:in)?)?)\s*(?:\([^)]*\))?\s*(?::|-)?\s*(.*)$`)
)

// refineDEATCountry resolves a DE-vs-AT guess that came from an ambiguous legal-form suffix.
// A VAT prefix (already decided by the caller) always wins; otherwise the register type, then the
// postcode shape, then the ccTLD, outrank the suffix guess. Anything outside DE/AT is returned unchanged.
func refineDEATCountry(country, vat, register, address, text, hostname string) string {
	if vat != "" || (country != "DE" && country != "AT") {
		return country
	}
	reg := register + " " + courtMention(text)
	switch {
	case atRegisterRE.MatchString(reg) && !deRegisterRE.MatchString(reg):
		return "AT"
	case deRegisterRE.MatchString(reg) && !atRegisterRE.MatchString(reg):
		return "DE"
	}
	atAddr, deAddr := atPostcodeRE.MatchString(address), dePostcodeRE.MatchString(address)
	switch {
	case atAddr && !deAddr:
		return "AT"
	case deAddr && !atAddr:
		return "DE"
	}
	if tld := tldCountryOf(hostname); tld == "AT" || tld == "DE" {
		return tld
	}
	return country
}

// extractATPostalAddress finds the first Austrian postal address in the page text. Only used when the
// entity-anchored search found nothing, and only for pages already judged Austrian by the caller.
func extractATPostalAddress(text string) string {
	// Blank lines (and stray "\r") between the street line and the postcode line are common in real
	// markup (ams.at, boja.at), so match on the sequence of non-empty lines.
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if l = collapseSpaces(strings.TrimSpace(l)); l != "" {
			lines = append(lines, l)
		}
	}
	for i, line := range lines {
		if len(line) > 240 {
			continue
		}
		if m := atInlineAddrRE.FindStringSubmatch(line); m != nil {
			return strings.TrimSpace(m[1]) + ", " + strings.TrimSpace(m[2])
		}
		if i+1 < len(lines) {
			if s := atStreetLineRE.FindStringSubmatch(line); s != nil {
				if c := atCityLineRE.FindStringSubmatch(lines[i+1]); c != nil {
					return strings.TrimSpace(s[1]) + ", " + strings.TrimSpace(c[1])
				}
			}
		}
	}
	return ""
}

// extractMedieninhaber returns the named media owner / publisher (Mediengesetz §25), either after the
// label on the same line or, when the label stands alone, on the next non-empty line.
func extractMedieninhaber(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		line = collapseSpaces(strings.TrimSpace(line))
		if line == "" || len(line) > 200 {
			continue
		}
		m := medieninhaberRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		val := strings.Trim(strings.TrimSpace(m[1]), " ,;:-")
		if val == "" {
			for j := i + 1; j < len(lines) && j <= i+2; j++ {
				if next := collapseSpaces(strings.TrimSpace(lines[j])); next != "" {
					val = strings.Trim(next, " ,;:-")
					break
				}
			}
		}
		// A name, not another label or an address line.
		if val == "" || len(val) > 120 || strings.HasSuffix(val, ":") || hasDigitRun(val, 4) || atPostcodeRE.MatchString(val) {
			continue
		}
		return val
	}
	return ""
}
