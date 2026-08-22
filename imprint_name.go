package main

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Vendored (adapted) from go_legal_entity's entity.go / entity_clean.go /
// entity_strip.go / entity_extract.go / entity_looks.go / entity_is.go /
// extract.go (v1.10.14) — the false-positive-hardened text cleanup this
// fleet uses to turn "<suffix found on this line>" into a real, sane
// candidate entity name. This is the single FP gate every text-derived
// imprint candidate clears before extractImprintFields will use it.
//
// stripTagsLines is a NEW name for a function upstream also calls
// stripTags — deliberately renamed here because go_tos_finder already has
// its own stripTags (discover.go), which collapses ALL whitespace
// (including newlines) to single spaces for link-text cleanup. That
// destroys the line structure extractEntityAround/extractAddressNearEntity
// need to do their line-based scanning, so this port keeps its own
// line-preserving variant under a distinct name rather than silently
// changing the behaviour of the existing one.

// stripTagsLines is a cheap HTML-to-text converter that preserves line
// structure: every tag boundary becomes a newline, so block-ish content
// naturally lands on its own line for the line-based name/address scanners
// below. We don't need markup fidelity, only text plus line breaks.
func stripTagsLines(body string) string {
	var b strings.Builder
	b.Grow(len(body))
	inTag := false
	inScript := false
	inStyle := false
	i := 0
	for i < len(body) {
		if !inTag {
			if !inScript && !inStyle && strings.HasPrefix(strings.ToLower(safeSlice(body, i, i+8)), "<script") {
				inScript = true
				inTag = true
				i++
				continue
			}
			if !inScript && !inStyle && strings.HasPrefix(strings.ToLower(safeSlice(body, i, i+7)), "<style") {
				inStyle = true
				inTag = true
				i++
				continue
			}
			if body[i] == '<' {
				inTag = true
				i++
				continue
			}
			b.WriteByte(body[i])
			i++
			continue
		}
		// inside a tag
		if body[i] == '>' {
			inTag = false
			if inScript {
				j := strings.Index(strings.ToLower(body[i+1:]), "</script>")
				if j < 0 {
					return b.String()
				}
				i = i + 1 + j + len("</script>")
				inScript = false
				continue
			}
			if inStyle {
				j := strings.Index(strings.ToLower(body[i+1:]), "</style>")
				if j < 0 {
					return b.String()
				}
				i = i + 1 + j + len("</style>")
				inStyle = false
				continue
			}
			b.WriteByte('\n')
			i++
			continue
		}
		i++
	}
	return b.String()
}

func safeSlice(s string, i, j int) string {
	if i < 0 {
		i = 0
	}
	if j > len(s) {
		j = len(s)
	}
	if i >= j {
		return ""
	}
	return s[i:j]
}

func containsCI(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

// looksAddressLine is a loose heuristic for "this line is part of a postal
// address": either it carries a digit (house number / postal code) or a
// recognisable address-vocabulary marker.
func looksAddressLine(s string) bool {
	low := strings.ToLower(s)
	if strings.ContainsAny(s, "0123456789") {
		return true
	}
	for _, marker := range []string{"street", "strasse", "straße", "road", "avenue", "ave", "boulevard", "suite", "floor", "germany", "united states", "romania", "france", "netherlands", "poland"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// extractAddressNearEntity collects up to 4 address-looking lines
// immediately following the line naming `name`, stopping early at a
// VAT/register/contact line (those belong to a different field, not the
// address).
func extractAddressNearEntity(text, name string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.Contains(line, name) {
			continue
		}
		var parts []string
		for j := i + 1; j < len(lines) && j <= i+10 && len(parts) < 4; j++ {
			clean := collapseSpaces(strings.TrimSpace(lines[j]))
			if clean == "" {
				continue
			}
			low := strings.ToLower(clean)
			// Stop before absorbing an identifier/register/contact line
			// into the address — those are separate fields (Register/VAT
			// are found independently via findIdentifiers+proximity in
			// extractImprintText, contact via hasImprintContact in
			// imprint.go). "firmenbuch"/"fn "/"uid" cover the Austrian
			// register/VAT labels (Firmenbuchnummer, FN ..., UID) that
			// upstream's DE-centric marker list didn't recognise.
			if strings.Contains(low, "vat") || strings.Contains(low, "ust-id") ||
				strings.Contains(low, "hrb") || strings.Contains(low, "company number") ||
				strings.Contains(low, "email") || strings.Contains(low, "@") ||
				strings.Contains(low, "firmenbuch") || strings.Contains(low, "uid") ||
				strings.Contains(low, "fn ") {
				break
			}
			if looksAddressLine(clean) {
				parts = append(parts, clean)
			}
		}
		if len(parts) > 0 {
			return collapseSpaces(strings.Join(parts, ", "))
		}
	}
	return ""
}

// extractEntityAround finds the company name preceding a suffix inside
// `text` around its first occurrence. Returns the cleaned candidate, or ""
// if nothing reasonable was found. The candidate is post-filtered through
// cleanCandidateName, which rejects sentence-like text, html-entity
// remnants, and unbalanced-paren openers.
//
// Upstream also handles "prefix-form" jurisdictions (legal form written
// BEFORE the name — Cyrillic/CJK/Indonesian) via a right-side extraction
// branch; that branch is intentionally not ported here (see imprint_suffix.go
// — the trimmed suffixTable carries only name-then-suffix forms, so it is
// unreachable).
func extractEntityAround(text, suffix string) string {
	idx := strings.Index(text, suffix)
	if idx < 0 {
		return ""
	}
	startMax := idx
	startMin := idx - 80
	if startMin < 0 {
		startMin = 0
	}
	leftCut := startMin
	for i := startMax - 1; i >= startMin; i-- {
		c := text[i]
		if c == '\n' || c == '\r' || c == '|' {
			leftCut = i + 1
			break
		}
		// A full-stop followed by a space is a sentence boundary — stop
		// climbing so we don't pull the previous sentence into the name.
		if c == '.' && i+1 < len(text) && text[i+1] == ' ' && i+2 < len(text) {
			next := text[i+2]
			if next >= 'A' && next <= 'Z' {
				leftCut = i + 2
				break
			}
		}
	}
	right := idx + len(suffix)
	candidate := strings.TrimSpace(text[leftCut:right])
	candidate = stripCopyrightPrefix(candidate)
	candidate = stripCopyrightLeftover(candidate)
	// trimAtConjunction must come BEFORE stripLabelPrefix so a role-prefix
	// like "Managing Director of X and Y GmbH" exposes the conjunction to
	// the label strip in the right order.
	candidate = trimAtConjunction(candidate, suffix)
	candidate = stripPersonRolePrefix(candidate)
	candidate = stripAwardPrefix(candidate)
	candidate = stripLabelPrefix(candidate)
	candidate = trimAtConjunction(candidate, suffix)
	candidate = collapseSpaces(candidate)
	if !cleanCandidateName(candidate, suffix) {
		return ""
	}
	return candidate
}

// labeledNameRE matches an explicit trading-name label with no
// legal-form-suffix requirement — the fallback extractImprintText
// (imprint_jsonld.go) takes when the suffix-anchored scan (detectSuffix)
// finds nothing anywhere on the page, most commonly for a sole trader
// (Einzelunternehmen), who legitimately has no GmbH/AG/Ltd-style suffix to
// anchor on at all. Found via live verification against a real Austrian
// sole-trader Impressum (hotelrose.at), whose visible text reads
// "Firmenname: Aktivhotel Zur Rose" with no legal form anywhere on the
// page. Anchored to the START of the line (extractImprintText already
// works off stripTagsLines' one-block-per-line text) so a page that merely
// discusses "Firmenname" in running prose, with the label not immediately
// followed by ": <value>", does not false-positive — see
// TestExtractImprintTextLabeledNameFalsePositiveGuard.
var labeledNameRE = regexp.MustCompile(`(?i)^(?:Firmenname|Company\s*Name|Unternehmen|Inhaber(?:in)?)\s*:\s*(.*)$`)

// validLabeledName is the label-path's false-positive gate, parallel to
// cleanCandidateName's role for the suffix-anchored path but lighter:
// there is no glued suffix to strip here (labeledNameRE's capture group
// already isolated the value), so this only needs to reject junk values —
// empty, too long, HTML-entity residue, a bare domain, or sentence-shaped
// prose (e.g. a "Firmenname:" heading followed by an explanatory sentence
// rather than an actual name).
func validLabeledName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) < 2 || len(name) > 90 {
		return false
	}
	if containsNonPrintable(name) {
		return false
	}
	if looksLikeSentencePhrase(name) {
		return false
	}
	if looksLikeDomainString(name) {
		return false
	}
	for _, ent := range []string{"&quot;", "&amp;", "&nbsp;", "&copy;", "&#"} {
		if strings.Contains(name, ent) {
			return false
		}
	}
	firstAlpha := byte(0)
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			firstAlpha = c
			break
		}
	}
	if firstAlpha == 0 || (firstAlpha >= 'a' && firstAlpha <= 'z') {
		return false
	}
	low := " " + strings.ToLower(name) + " "
	hits := 0
	for _, w := range sentenceStopWords {
		if strings.Contains(low, " "+w+" ") {
			hits++
			if hits >= 2 {
				return false
			}
		}
	}
	return true
}

// cleanCandidateName rejects strings that look like sentence text,
// parenthetical asides, HTML-entity residue, address fragments, slash-tier
// labels, or weak-suffix non-names rather than a legal-entity name.
func cleanCandidateName(name, suffix string) bool {
	if len(name) < len(suffix)+2 || len(name) > 90 {
		return false
	}
	if containsNonPrintable(name) {
		return false
	}
	if looksLikeSentencePhrase(name) {
		return false
	}
	if looksLikeDomainString(name) {
		return false
	}
	if strings.HasPrefix(name, "©") || strings.HasPrefix(name, "™") ||
		strings.HasPrefix(name, "®") {
		return false
	}
	if strings.Contains(name, "&") && (strings.Contains(name, ";") || strings.Contains(name, "&#")) {
		for _, ent := range []string{"&quot;", "&amp;", "&nbsp;", "&copy;", "&apos;", "&#", "&lt;", "&gt;"} {
			if strings.Contains(name, ent) {
				return false
			}
		}
	}
	if strings.Count(name, "(") != strings.Count(name, ")") {
		return false
	}
	if strings.ContainsAny(name, "|\\") {
		return false
	}
	if strings.Contains(name, ";") {
		return false
	}
	if strings.Contains(name, "/") && !suffixHasSlash(suffix) {
		return false
	}
	if strings.Contains(name, "/") && suffixHasSlash(suffix) {
		if strings.Count(name, "/") > strings.Count(suffix, "/") {
			return false
		}
	}
	if name[0] >= '0' && name[0] <= '9' {
		return false
	}
	firstAlpha := byte(0)
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			firstAlpha = c
			break
		}
	}
	if firstAlpha >= 'a' && firstAlpha <= 'z' {
		return false
	}
	stem := strings.TrimSpace(strings.TrimSuffix(name, suffix))
	if strings.EqualFold(strings.TrimSuffix(stem, ":"), "Handelsregister") {
		return false
	}
	if cookieLikeToken(stem) {
		return false
	}
	if precededByPostalCode(stem) {
		return false
	}
	if looksLikeGeoAddressFragment(stem, suffix) {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(suffix), "Company") && !validCompanyBrand(stem) {
		return false
	}
	stemLow := " " + strings.ToLower(stem) + " "
	hits := 0
	for _, w := range sentenceStopWords {
		if strings.Contains(stemLow, " "+w+" ") {
			hits++
			if hits >= 2 {
				return false
			}
		}
	}
	return true
}

// containsNonPrintable returns true if s contains a control character or
// invalid UTF-8 sequence.
func containsNonPrintable(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return true
		}
	}
	return false
}

// looksLikeSentencePhrase returns true when the candidate name carries a
// finite verb / sentence-shape token typical of legal prose rather than a
// proper noun phrase.
func looksLikeSentencePhrase(name string) bool {
	low := strings.ToLower(name)
	for _, v := range []string{
		" has appointed ", " is operated by ", " is provided by ",
		" operates as ", " acts on behalf of ", " holds ",
		" has been ", " have been ", " was acquired by ",
	} {
		if strings.Contains(low, v) {
			return true
		}
	}
	return false
}

// looksLikeDomainString returns true for candidates that are just a bare
// registrable domain or host ("Walmart.com") — these come from
// og_site_name / json_ld brand aliases and are NOT legal-entity names.
func looksLikeDomainString(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\n\r") || !strings.Contains(s, ".") {
		return false
	}
	low := strings.ToLower(strings.TrimPrefix(strings.ToLower(s), "www."))
	labels := strings.Split(low, ".")
	if len(labels) < 2 {
		return false
	}
	for i, lab := range labels {
		if lab == "" {
			return false
		}
		for j := 0; j < len(lab); j++ {
			c := lab[j]
			isLetter := c >= 'a' && c <= 'z'
			isDigit := c >= '0' && c <= '9'
			if !(isLetter || isDigit || c == '-') {
				return false
			}
		}
		if i == len(labels)-1 {
			if len(lab) < 2 {
				return false
			}
			for j := 0; j < len(lab); j++ {
				if lab[j] < 'a' || lab[j] > 'z' {
					return false
				}
			}
		}
	}
	return true
}

// normalizeSpaces converts Unicode whitespace (non-breaking space U+00A0,
// narrow NBSP U+202F, thin space, en/em spaces, etc.) to a plain ASCII
// space. Imprint footers frequently use U+00A0 before a colon
// ("Label : Value"), which otherwise defeats an ASCII space-skip.
func normalizeSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if r != ' ' && isUnicodeSpace(r) {
			return ' '
		}
		return r
	}, s)
}

func isUnicodeSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\v', '\f', '\r', 0x85, 0xA0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005, 0x2006, 0x2007,
		0x2008, 0x2009, 0x200A, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000:
		return true
	}
	return false
}

// contactLabels are footer/contact-row labels that, in imprints, sit on the
// same physical line as a legal-form suffix and bleed into the extracted
// name — e.g. "Webmaster : SAS".
var contactLabels = []string{
	"webmaster", "kontakt", "contact", "email", "e-mail", "mail",
	"tel", "telephone", "téléphone", "telefon", "phone", "fax",
	"impressum", "imprint", "adresse", "address", "anschrift",
	"hébergeur", "hebergeur", "hosting", "host", "responsable",
	"editor", "éditeur", "editeur", "redaktion", "redaction", "rédaction",
}

// hostingProsePrefixes are leading "this site is hosted by …" disclaimers.
var hostingProsePrefixes = []string{
	"le site est hébergé par les ", "le site est hébergé par le ",
	"le site est hébergé par la ", "le site est hébergé par ",
	"site hébergé par les ", "site hébergé par le ",
	"site hébergé par la ", "site hébergé par ",
	"hébergé par les ", "hébergé par le ", "hébergé par la ", "hébergé par ",
	"this site is hosted by ", "the site is hosted by ",
	"website hosted by ", "site hosted by ", "hosted by ",
	"diese website wird gehostet von ", "gehostet von ", "gehostet bei ",
}

// trimAtConjunction handles "YOOX and Meta Platforms Ireland Limited" by
// keeping only the trailing entity (the one ending in the suffix).
func trimAtConjunction(s, suffix string) string {
	suffIdx := strings.LastIndex(s, suffix)
	if suffIdx < 0 {
		return s
	}
	for _, conj := range []string{" and ", " & ", " y ", " et ", " und ", " e "} {
		i := strings.LastIndex(s[:suffIdx], conj)
		if i > 0 {
			s = strings.TrimSpace(s[i+len(conj):])
			break
		}
	}
	return s
}

// cookieLikeToken returns true for shapes like "X-AB", "_GA" — cookie /
// header-tag tokens that the suffix detector mis-classifies as a real
// entity ending in "AB", "CV" or similar.
func cookieLikeToken(stem string) bool {
	stem = strings.TrimSpace(stem)
	if stem == "" {
		return false
	}
	if (stem[0] == 'X' || stem[0] == '_') && (len(stem) < 6) {
		dashes := strings.Count(stem, "-") + strings.Count(stem, "_")
		if dashes >= 1 {
			return true
		}
	}
	return false
}

// precededByPostalCode returns true when the token immediately preceding
// the suffix is a 4-digit number (a Dutch/German postal code prefix glued
// to a 2-letter postcode suffix like "AB"/"LP"/"CV"/"BV").
func precededByPostalCode(stem string) bool {
	parts := strings.Fields(stem)
	if len(parts) < 1 {
		return false
	}
	last := parts[len(parts)-1]
	if len(last) != 4 {
		return false
	}
	for i := 0; i < 4; i++ {
		if last[i] < '0' || last[i] > '9' {
			return false
		}
	}
	return true
}

// geoAmbiguousSuffixes are legal-form suffixes that double as common
// US-state or Canadian-province postal abbreviations ("Las Vegas, NV",
// "Edmonton, AB").
var geoAmbiguousSuffixes = map[string]bool{
	"NV": true, // Netherlands N.V. vs. Nevada
	"AB": true, // Sweden AB vs. Alberta
}

// looksLikeGeoAddressFragment reports whether name is a "City, ST" address
// fragment rather than a legal-entity name.
func looksLikeGeoAddressFragment(stem, suffix string) bool {
	if !geoAmbiguousSuffixes[suffix] {
		return false
	}
	return strings.HasSuffix(stem, ",")
}

// validCompanyBrand returns true when the brand token preceding "Company"
// is a real proper noun and not an article/adjective/possessive.
func validCompanyBrand(stem string) bool {
	stem = strings.TrimSpace(stem)
	parts := strings.Fields(stem)
	if len(parts) == 0 {
		return false
	}
	if len(parts) >= 3 {
		return true
	}
	brand := strings.ToLower(parts[0])
	for _, bad := range companyStopBrands {
		if brand == bad {
			return false
		}
	}
	return true
}

// companyStopBrands are words that are not real brand tokens when they
// appear immediately before "Company" as the entire brand.
var companyStopBrands = []string{
	"a", "an", "the", "our", "your", "their", "this", "that", "any", "every",
	"my", "his", "her", "its",
	"design", "fast", "free", "global", "local", "online", "digital", "tech",
	"smart", "best", "top", "leading", "world", "major", "minor", "future",
	"modern", "better", "right", "great", "small", "big", "large", "real",
	"new", "old", "open", "private", "public",
	"discord", "github", "stripe", "shopify", "google", "meta", "amazon",
}

// sentenceStopWords are common stop-words across EN/DE/FR/RO/IT/ES/NL that
// almost never appear inside a real legal-entity name but are dense in
// sentence prose. Two or more hits in a candidate's stem ⇒ reject as
// sentence text.
var sentenceStopWords = []string{
	// English
	"the", "and", "of", "to", "for", "is", "are", "was", "were", "we", "us", "our",
	"by", "on", "in", "with", "from", "or", "an", "as", "this", "that", "these",
	"those", "will", "shall", "may", "must", "be", "been", "being", "have", "has",
	"had", "do", "does", "did", "not", "no", "so", "if", "than", "then", "such",
	"please", "note", "refer", "means", "available", "supported", "prohibited",
	// German
	"der", "die", "das", "und", "oder", "mit", "ohne", "fur", "für", "von", "zu",
	"zur", "zum", "ist", "sind", "sie", "ihr", "auf", "bei", "bitte", "wird",
	"werden", "kontaktaufnahme", "elektronischen", "verwenden", "haftung",
	// French
	"les", "des", "une", "aux", "pour", "avec", "sans", "dans", "sur", "qui",
	"que", "est", "sont", "nous", "vous",
	// Romanian
	"acest", "aceste", "această", "aceasta", "sunt", "este", "între", "intre",
	"încheiate", "incheiate", "dumneavoastră", "dumneavoastra", "dvs",
	"conform", "prezentul", "prezenta", "prin", "precum",
	// Italian
	"questa", "questi", "della", "delle", "dello", "degli", "sono",
	// Spanish
	"este", "esta", "estos", "estas", "para", "por", "con", "como",
	// Dutch
	"deze", "dit", "voor", "ervan", "vanaf", "zoals", "naar",
}

func collapseSpaces(s string) string {
	out := make([]byte, 0, len(s))
	prevSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\t' || c == '\n' || c == '\r' {
			c = ' '
		}
		if c == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		out = append(out, c)
	}
	return strings.TrimSpace(string(out))
}

// stripLabelPrefix removes recognised page-section labels glued to a name,
// e.g. "Impressum SAP SE" → "SAP SE". Idempotent.
func stripLabelPrefix(s string) string {
	s = strings.TrimSpace(s)
	labels := []string{
		"Product-Specific Terms for ", "Specific Terms for ",
		"ATOL Holder: ", "ATOL Holder ",
		"Legal Disclosure - ", "Legal Disclosure: ", "Legal Disclosure ",
		"Legal Notice - ", "Legal Notice: ", "Legal Notice ",
		"Impressum: ", "Impressum - ", "Impressum ",
		"Imprint: ", "Imprint - ", "Imprint ",
		"Provider: ", "Provider - ", "Operator: ", "Controller: ",
		"Publisher: ", "Editor: ", "Authorised by ", "Authorized by ",
		"Bank: ", "Bancă: ", "Banque: ", "Banca: ",
		"About ", "Contact ", "Terms for ",
		"Managing Director of ", "Chief Executive Officer of ", "CEO of ",
		"CEO, ", "General Counsel, ", "General Counsel of ",
		"Board of Directors ", "Authorized recipient for ",
		"By Mail: ", "By Fax: ", "By Email: ", "By Phone: ",
		"Firma: ", "Disclaimer von ", "Impressum von ",
		"Terms of Use: ", "Terms of Service: ",
		"See the ", "See: ",
		"Best ",
		"Debt securities ", "Eligible deposits in ",
		"As far as ", "Subject to ", "Powered by ", "Provided by ",
		"Operated by ", "Powered By ", "Provided By ", "Operated By ",
		"News ", "NEWS ", "Help Center ", "Newsroom ", "Contractual terms of ",
	}
	for _, kw := range []string{"Section ", "Article ", "Annex ", "Schedule ",
		"Appendix ", "Exhibit ", "Chapter ", "Clause ", "Part "} {
		if strings.HasPrefix(s, kw) {
			rest := s[len(kw):]
			i := 0
			for i < len(rest) && ((rest[i] >= '0' && rest[i] <= '9') ||
				rest[i] == '.' || rest[i] == '-') {
				i++
			}
			if i > 0 && i < len(rest) {
				for i < len(rest) && (rest[i] == '.' || rest[i] == ':' || rest[i] == ' ') {
					i++
				}
				s = strings.TrimSpace(rest[i:])
				return s
			}
		}
	}
	for _, l := range labels {
		if strings.HasPrefix(s, l) {
			s = strings.TrimSpace(s[len(l):])
			break
		}
	}
	s = stripContactLabelPrefix(s)
	s = stripHostingProsePrefix(s)
	return s
}

// stripContactLabelPrefix removes leading footer/contact labels glued to a
// name via a ':' or '-' separator, e.g. "Webmaster : SAS" → "SAS". Loops so
// chained labels fully strip.
func stripContactLabelPrefix(s string) string {
	s = normalizeSpaces(s)
	for {
		t := strings.TrimSpace(s)
		matched := false
		low := strings.ToLower(t)
		for _, lbl := range contactLabels {
			if !strings.HasPrefix(low, lbl) {
				continue
			}
			rest := t[len(lbl):]
			j := 0
			for j < len(rest) && rest[j] == ' ' {
				j++
			}
			if j < len(rest) && (rest[j] == ':' || rest[j] == '-') {
				j++
				s = strings.TrimSpace(rest[j:])
				matched = true
				break
			}
		}
		if !matched {
			return t
		}
	}
}

// stripHostingProsePrefix removes a leading host-provider disclaimer
// sentence, returning the text after the "hosted by …" connective.
func stripHostingProsePrefix(s string) string {
	t := normalizeSpaces(strings.TrimSpace(s))
	low := strings.ToLower(t)
	for _, p := range hostingProsePrefixes {
		if strings.HasPrefix(low, p) {
			return strings.TrimSpace(t[len(p):])
		}
	}
	return t
}

// stripPersonRolePrefix removes "<Person Name>, <Title>(?: of)? " prefixes
// glued onto a real entity, e.g. "Oliver Bäte, CEO of Allianz SE" →
// "Allianz SE".
func stripPersonRolePrefix(s string) string {
	s = strings.TrimSpace(s)
	commaIdx := strings.Index(s, ",")
	if commaIdx < 3 {
		return s
	}
	pre := strings.Fields(s[:commaIdx])
	if len(pre) < 1 || len(pre) > 4 {
		return s
	}
	for _, w := range pre {
		if len(w) < 2 {
			return s
		}
		if w[0] < 'A' || w[0] > 'Z' {
			return s
		}
	}
	rest := strings.TrimSpace(s[commaIdx+1:])
	for _, t := range []string{"CEO ", "CFO ", "CTO ", "COO ", "CIO ", "CMO ",
		"CEO,", "CFO,", "CTO,", "COO,",
		"President ", "President,",
		"Chairman ", "Chair ", "Chairwoman ",
		"Managing Director ", "Managing Partner ",
		"General Counsel ", "General Counsel,",
		"Founder ", "Co-Founder ", "Co-founder ",
		"Director ", "Vice President ", "Senior Vice President ",
	} {
		if strings.HasPrefix(rest, t) {
			rest = strings.TrimSpace(rest[len(t):])
			rest = strings.TrimPrefix(rest, "of ")
			return strings.TrimSpace(rest)
		}
	}
	return s
}

// stripAwardPrefix turns "Best convertible bond: Asia Cement Corporation"
// into "Asia Cement Corporation".
func stripAwardPrefix(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "Best ") {
		return s
	}
	i := strings.Index(s, ": ")
	if i < 0 || i > 80 {
		return s
	}
	return strings.TrimSpace(s[i+2:])
}

// stripCopyrightLeftover removes leading "©", "™", "®", or curly-quote
// residue that survived stripCopyrightPrefix.
func stripCopyrightLeftover(s string) string {
	s = strings.TrimSpace(s)
	for {
		stripped := false
		for len(s) > 0 {
			r := s[0]
			if r == '(' || r == '"' || r == '\'' || r == '`' {
				s = strings.TrimSpace(s[1:])
				stripped = true
				continue
			}
			break
		}
		for _, p := range []string{"©", "™", "®", "“", "”", "„", "«", "»", "‘", "’"} {
			if strings.HasPrefix(s, p) {
				s = strings.TrimSpace(strings.TrimPrefix(s, p))
				stripped = true
			}
		}
		if len(s) >= 4 && s[0] >= '1' && s[0] <= '2' &&
			s[1] >= '0' && s[1] <= '9' &&
			s[2] >= '0' && s[2] <= '9' &&
			s[3] >= '0' && s[3] <= '9' {
			j := 4
			for j < len(s) && (s[j] == '-' || (s[j] >= '0' && s[j] <= '9')) {
				j++
			}
			s = strings.TrimSpace(s[j:])
			stripped = true
		}
		if !stripped {
			break
		}
	}
	return s
}

func stripCopyrightPrefix(s string) string {
	s = strings.TrimSpace(s)
	for _, pfx := range []string{"©", "(c)", "(C)", "Copyright", "copyright", "COPYRIGHT"} {
		if strings.HasPrefix(s, pfx) {
			s = strings.TrimSpace(strings.TrimPrefix(s, pfx))
		}
	}
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '-' ||
		(s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i > 0 && i < len(s) {
		s = strings.TrimSpace(s[i:])
	}
	if k := strings.LastIndex(s, "All rights reserved."); k >= 0 {
		s = strings.TrimSpace(s[k+len("All rights reserved."):])
	}
	return s
}

// formatRegister assembles a "<KIND> <VALUE>" string without duplicating the
// kind token if the regex already captured it (e.g. value="HRB 6089" + kind
// "HRB" would otherwise yield "HRB HRB 6089"). Also strips trailing colons
// and label residue ("No.", "Nummer", "Numer").
func formatRegister(kind, value string) string {
	v := strings.TrimSpace(value)
	lowKind := strings.ToLower(kind)
	for {
		lv := strings.ToLower(v)
		if strings.HasPrefix(lv, lowKind) {
			v = strings.TrimSpace(v[len(kind):])
			continue
		}
		break
	}
	v = strings.TrimLeft(v, " :.,#-")
	for _, lbl := range []string{"No. ", "No.", "No ", "Nummer ", "Numer ",
		"Number ", "Reg. ", "Reg ", "registered ", "Registered "} {
		if strings.HasPrefix(v, lbl) {
			v = strings.TrimSpace(v[len(lbl):])
		}
	}
	v = strings.TrimLeft(v, " :.,#-")
	if v == "" {
		return kind
	}
	return kind + " " + v
}
