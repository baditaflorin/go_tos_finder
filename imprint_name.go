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

// bareIntlPhoneRE matches a line that is NOTHING but an international
// phone number in "(+NN)NNNNNNNNN"-or-similar shape — see
// extractAddressNearEntity's doc comment for the real evidence.
var bareIntlPhoneRE = regexp.MustCompile(`^\(?\+\d{1,3}\)?[\d\s()\-./]{5,20}$`)

// aveWordRE matches the "Ave"/"Ave." US-street abbreviation as a whole
// word — see looksAddressLine's doc comment for why this can't be a plain
// substring check like its sibling markers.
var aveWordRE = regexp.MustCompile(`\bave\.?\b`)

// cuiWordRE matches Romania's "CUI"/"CIF" register-identifier label
// (imprint_vat.go's "CUI" vatPattern) as a whole word, case-insensitively —
// see extractAddressNearEntity's doc comment below for the real evidence.
// A plain substring check (like the DE/NL/FR markers just above it) is
// unsafe here: "cui" is also an ordinary standalone Romanian word ("to
// whom"/"nail"), so a bare Contains could break address collection on an
// unrelated sentence. Word-boundaries avoid that while still catching
// every real label form ("CUI:", "CUI 123", "CIF#123", ...).
var cuiWordRE = regexp.MustCompile(`(?i)\b(?:cui|cif)\b`)

// looksAddressLine is a loose heuristic for "this line is part of a postal
// address": either it carries a postal-code/house-number-shaped digit run
// (hasDigitRun — see its doc comment for why "any digit at all" was too
// loose) or a recognisable address-vocabulary marker.
func looksAddressLine(s string) bool {
	low := strings.ToLower(s)
	if hasDigitRun(s, 4) {
		return true
	}
	// "ave" (the "Ave."/"Ave" abbreviation for Avenue) is checked as its
	// own word-boundary-anchored regexp, NOT folded into the plain
	// substring markers loop below like the others — a bare
	// strings.Contains(low, "ave") false-positives on the ordinary English
	// verb "have" (and "gave"/"save"/"wave"/...). Real evidence: kims.dk's
	// real handelsbetingelser page has an unrelated age-disclaimer
	// sentence ("...eller have en forældre/værge tilladelse...") sitting
	// between the winning candidate's name line and its real address —
	// the substring match on "have" wrongly absorbed that whole sentence
	// into the address before this fix. "avenue" itself is unaffected: it
	// is matched separately, in full, by the marker loop below.
	if aveWordRE.MatchString(low) {
		return true
	}
	for _, marker := range []string{
		"street", "strasse", "straße", "road", "avenue", "boulevard", "suite", "floor",
		"germany", "united states", "romania", "france", "netherlands", "poland",
		// Real evidence: humresto.fr's real mentions-légales lists its
		// address as "20 rue Marcel Pagnol" — a two-digit house number plus
		// "rue" (French for "street") clears no digit-run threshold at all,
		// so without an explicit marker the line was silently dropped (see
		// extractAddressNearEntity's doc comment for the false-positive this
		// same page surfaced on the OTHER side of that gap).
		"rue",
		// Real evidence: neptun.orlen.pl's real "Dane kontaktowe i
		// rejestrowe" page lists its address as "Aleja Grunwaldzka 472,
		// 80-309 Gdańsk" — the house number (472) and the postal code
		// (80-309, split by a dash) both clear no consecutive-digit-run
		// threshold at all, so without an explicit marker the line was
		// silently dropped (same failure shape as the "rue" case above).
		"aleja",
		// Real evidence: kims.dk's real handelsbetingelser page lists its
		// street line as "Sømarksvej 31" — a two-digit house number glued
		// onto a "vej"-suffixed street name (Danish for "road"/"way", the
		// single most common Danish street-name ending) clears no
		// consecutive-digit-run threshold at all, so without this marker
		// only the following postal-code/city line ("5471 Søndersø")
		// survived extractAddressNearEntity's scan and the street itself
		// was silently dropped — same failure shape as the "rue"/"aleja"
		// cases above.
		"vej",
		// Real evidence: japanphoto.no's real Kjøpsvilkår
		// "Kontaktinformasjon" section lists its postal address as
		// "Postboks 4 Bjørndal" — a one-digit box number ("4") clears no
		// digit-run threshold at all, so without this marker only the
		// following postal-code/city line ("1214 Oslo") survived
		// extractAddressNearEntity's scan and the PO-box line itself was
		// silently dropped — same failure shape as the "rue"/"aleja"/"vej"
		// cases above.
		"postboks",
	} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// hasDigitRun reports whether s contains a run of at least n consecutive
// ASCII digits. Used in place of "contains any digit at all" (too loose —
// found via real-evidence stress-testing extractAddressNearEntity against a
// real Austrian gambling-comparison site's "Top 5" operator table: a rating
// column ("9.6") and a bonus-percentage line ("100% Bonus bis zu 500 Euro")
// both carry bare digits with no postal address anywhere near them, and got
// collected as this candidate's "address" — see
// hasProximityCorroboration's doc comment in imprint.go for how that then
// fed a second, worse bug: a false address made an uncorroborated
// third-party brand mention pass as corroborated). Real postal
// codes/house-number blocks in this codebase's target jurisdictions (DE
// 5-digit, AT/NL 4-digit postal codes) comfortably clear a 4-digit-run bar;
// a rating/percentage/price fragment in ordinary prose essentially never
// does.
func hasDigitRun(s string, n int) bool {
	run := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			run++
			if run >= n {
				return true
			}
		} else {
			run = 0
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
		// Real evidence: simpelbootverhuurutrecht.nl's real colofon lists
		// the KvK number and BTW-id BEFORE the street address (name,
		// Eenmanszaak, KvK, BTW-id, THEN the address) — the reverse of the
		// DE/AT fixtures this window was originally sized for. 20 raw lines
		// (rather than 10) gives enough room to reach the real address past
		// a couple of identifier lines on a page that uses two blank
		// separator lines between every block (common in modern
		// page-builder markup — stripTagsLines turns each <br>/block
		// boundary into its own blank line).
		for j := i + 1; j < len(lines) && j <= i+20 && len(parts) < 4; j++ {
			clean := collapseSpaces(strings.TrimSpace(lines[j]))
			if clean == "" {
				continue
			}
			low := strings.ToLower(clean)
			// KvK/BTW-id: real Dutch register/VAT labels. Skip (not break)
			// so scanning continues past them to the real address — unlike
			// the break-markers below, this page's real address comes AFTER
			// these lines, not before.
			if strings.Contains(low, "kvk") || strings.Contains(low, "btw-id") {
				continue
			}
			// CVR: Denmark's register/VAT label (see imprint_vat.go's "CVR"
			// vatPattern doc comment). Same shape as KvK/BTW-id above: real
			// evidence, kims.dk's real handelsbetingelser page puts its own
			// "CVR.nr. 15233877" line BEFORE the real street address
			// ("Sømarksvej 31, 5471 Søndersø") — its 8-digit run cleared
			// looksAddressLine's digit-run heuristic and got absorbed into
			// the address ahead of (and instead of losing) the real street
			// line without this skip.
			if strings.Contains(low, "cvr") {
				continue
			}
			// NIP/KRS/REGON: real Polish tax-ID/register/statistical
			// labels. Real evidence: neptun.orlen.pl's real registry-data
			// page lists these three right after the real street address —
			// all three (10, 10, and 9 digits respectively) cleared the
			// digit-run heuristic and got wrongly absorbed into the
			// address alongside (and beyond) the real "Aleja Grunwaldzka
			// 472, 80-309 Gdańsk" line.
			if strings.Contains(low, "nip") || strings.Contains(low, "krs") || strings.Contains(low, "regon") {
				continue
			}
			// A bare "(+NN)NNNNNNNNN"-shaped international phone number on
			// its own line, with no "tel"/"phone" label attached to THIS
			// line (hasImprintContact/imprintPhoneRE find it independently
			// elsewhere — see imprint.go). Real evidence:
			// eurostarshotels.com's real aviso legal puts "teléfono" and
			// its value in separate <a href="tel:..."> markup, so
			// stripTagsLines splits them onto different lines — the bare
			// value line then cleared looksAddressLine's digit-run
			// heuristic and got absorbed into the address as a false
			// positive.
			if bareIntlPhoneRE.MatchString(clean) {
				continue
			}
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
				strings.Contains(low, "fn ") ||
				// "Code APE"/"Code NAF": the French business-activity
				// classification code (SIRENE's NAF/APE nomenclature, e.g.
				// "5621Z"). Real evidence: humresto.fr's real
				// mentions-légales lists "Code APE : Service des traiteurs
				// 5621Z" right after its RCS/SIRET lines — the code's
				// 4-digit-plus-letter shape cleared looksAddressLine's
				// digit-run heuristic and got absorbed into the address as
				// a false positive. Not an address field at all, so this
				// must stop collection before looksAddressLine ever sees it,
				// same as the VAT/register markers above.
				strings.Contains(low, "code ape") || strings.Contains(low, "code naf") ||
				// "TVA" (French/Belgian for VAT) and "Nomenclature APE"
				// (an alternate real-world phrasing of the French
				// business-activity code, distinct from "Code APE" above).
				// Real evidence: factuo.be's real mentions légales lists
				// "TVA BE 0704742612" right after the real Belgian street
				// address, and separately "Nomenclature APE 6312Z" for a
				// different (French hosting) entity further down the same
				// page — both cleared looksAddressLine's digit-run
				// heuristic and got absorbed into the address, the second
				// one dragging in an entirely different company's address
				// from a different country in the process.
				strings.Contains(low, "tva") || strings.Contains(low, "nomenclature ape") ||
				// "RCS" (Registre de Commerce et des Sociétés — France AND
				// Luxembourg's shared company-register name). Real evidence:
				// menu.lu's real "Mentions légales" page puts its own "RCS
				// Luxembourg n&deg; B258641" register citation on its own
				// line, immediately after the real street address line —
				// the "L-1118 Luxembourg" postal-code-shaped digit run makes
				// the address line itself pass looksAddressLine, and without
				// this marker the following RCS line ALSO clears
				// looksAddressLine (its own digit run) and gets absorbed
				// into the address as if it were a continuation postal
				// line.
				strings.Contains(low, "rcs") ||
				// "CUI"/"CIF": Romania's own register-identifier label (see
				// cuiWordRE's doc comment above). Real evidence: emag.ro's
				// real Termeni si conditii footer line "© 2001-2026 Dante
				// International, CUI: 14399840, Reg. Com. J2002000372404"
				// sits a few lines below the real entity-name paragraph,
				// its "14399840"/"J2002000372404" digit runs clear
				// looksAddressLine, and without this marker the whole
				// unrelated copyright/register line got absorbed into the
				// Address (visible as a stray ", &copy; 2001-2026 Dante
				// International, CUI: ..." tail on im.Address) instead of
				// being left for the CUI identifier match
				// (extractImprintText/findIdentifiers) to own.
				cuiWordRE.MatchString(low) {
				break
			}
			if looksAddressLine(clean) {
				// Real evidence: humresto.fr's street line is itself
				// comma-terminated in the source markup ("20 rue Marcel
				// Pagnol,<br>69720 ..."), which otherwise joins into a
				// double comma ("Pagnol,, 69720"). Trim a trailing
				// separator before joining rather than after, so a
				// mid-line comma (a real part of the text) is untouched.
				parts = append(parts, strings.TrimRight(clean, ",;"))
			}
		}
		if len(parts) > 0 {
			return collapseSpaces(strings.Join(parts, ", "))
		}
	}
	return ""
}

// knownNonSentenceAbbreviations lists short legal-form-adjacent
// abbreviations that end in a period but do NOT end a sentence — see the
// real-evidence comment at extractEntityAround's sentence-boundary check
// for why this matters.
var knownNonSentenceAbbreviations = map[string]bool{
	"soc": true,
}

// precededByKnownAbbreviation reports whether the word ending immediately
// before text[periodIdx] (expected to be a '.') is one of
// knownNonSentenceAbbreviations.
func precededByKnownAbbreviation(text string, periodIdx int) bool {
	j := periodIdx
	for j > 0 {
		c := text[j-1]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			break
		}
		j--
	}
	return knownNonSentenceAbbreviations[strings.ToLower(text[j:periodIdx])]
}

// extractEntityAround finds the company name preceding a suffix inside
// `text` around its first occurrence. Returns the cleaned candidate, or ""
// if nothing reasonable was found. The candidate is post-filtered through
// cleanCandidateName, which rejects sentence-like text, html-entity
// remnants, and unbalanced-paren openers.
//
// Upstream also handles "prefix-form" jurisdictions (legal form written
// BEFORE the name — Cyrillic/CJK/Indonesian) via a right-side extraction
// branch; that branch was originally left unported here on the assumption
// that the trimmed suffixTable (imprint_suffix.go) only carries
// name-then-suffix Latin forms. Real evidence corrected that assumption:
// humresto.fr's real mentions-légales page opens its imprint paragraph with
// "SARL Hum!Resto" — casual French SARL/SAS usage routinely puts the legal
// form BEFORE the trading name, not just after it. extractEntityAfterSuffix
// below is the (narrower, Latin-only) fallback for that case — tried only
// when nothing precedes the suffix on the line, so it never second-guesses
// an already-successful name-then-suffix match.
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
		// Except a KNOWN abbreviation ("Soc." — see
		// knownNonSentenceAbbreviations): real evidence, urbana.com.pt's
		// real "Ficha técnica" page reads "MoonMedia Comunicação, Soc.
		// Unipessoal Lda" — "Soc." (Portuguese for "Sociedade") directly
		// precedes the legal-form suffix as part of the SAME entity
		// description, not a new sentence; without this exception the
		// heuristic truncated the candidate down to just "Unipessoal Lda"
		// (identical in length to the suffix itself, with nothing real
		// before it), which cleanCandidateName then correctly rejected as
		// too short — silently dropping the whole legal_name.
		if c == '.' && i+1 < len(text) && text[i+1] == ' ' && i+2 < len(text) && !precededByKnownAbbreviation(text, i) {
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
	if cleanCandidateName(candidate, suffix) {
		return candidate
	}
	// Prefix-form fallback (see the doc comment above): only tried when
	// nothing meaningful precedes the suffix on this line — a real
	// name-then-suffix match earlier on the same line always wins first.
	if strings.TrimSpace(text[startMin:idx]) == "" {
		if after := extractEntityAfterSuffix(text, suffix, idx); after != "" {
			return after
		}
	}
	return ""
}

// extractEntityAfterSuffix is extractEntityAround's prefix-form
// counterpart: the trading name is the text immediately AFTER the suffix
// rather than before it ("SARL Hum!Resto" — see extractEntityAround's doc
// comment for the real evidence). Stops at the first sentence-shaped
// punctuation or line break so a suffix mentioned mid-sentence ("SARL.
// Fondée en 2020...") doesn't vacuum up unrelated prose. Validated through
// cleanCandidateName via a synthetic "name suffix" string so the exact same
// false-positive gates (sentence-phrase, HTML-entity residue, cookie/geo/
// postal-code stems, ...) apply as the suffix-after-name path.
func extractEntityAfterSuffix(text, suffix string, idx int) string {
	after := text[idx+len(suffix):]
	cut := len(after)
	for i, r := range after {
		if r == '\n' || r == '\r' || r == '|' || r == '.' || r == ',' {
			cut = i
			break
		}
	}
	name := strings.TrimSpace(after[:cut])
	if name == "" {
		return ""
	}
	if !cleanCandidateName(name+" "+suffix, suffix) {
		return ""
	}
	return suffix + " " + name
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

// standaloneLegalFormRE matches a line that is NOTHING but a bare
// legal-form declaration — the Dutch "Eenmanszaak" (sole proprietorship)
// convention, where the form is stated on its own line immediately AFTER
// the trading name rather than glued onto the name as a suffix
// (GmbH/SARL-style) or introduced by an explicit "Label: value" (
// labeledNameRE, above). Real evidence:
// simpelbootverhuurutrecht.nl/colofon/, whose real colofon page reads
// "Simpel Bootverhuur Utrecht" then, alone on the very next non-blank line,
// "Eenmanszaak." — see standaloneLegalFormCandidate. The trailing
// "(?:&nbsp;)?" tolerates stripTagsLines leaving a literal "&nbsp;" entity
// un-decoded at the end of the line — the real page's markup is
// "Eenmanszaak.&nbsp;" (a non-breaking space before the next <span>).
var standaloneLegalFormRE = regexp.MustCompile(`(?i)^eenmanszaak\.?\s*(?:&nbsp;)?\s*$`)

// standaloneLegalFormCandidate is standaloneLegalFormRE's companion: once a
// bare legal-form line is found, the trading name is the nearest preceding
// NON-BLANK line. The lookback window is counted in raw lines, not
// non-blank ones — stripTagsLines commonly emits two blank lines between
// each block-level element (confirmed against the real
// simpelbootverhuurutrecht.nl fixture: "Simpel Bootverhuur Utrecht", two
// blank lines, "Eenmanszaak.&nbsp;"), so a naive "skip at most N blanks"
// counter can run out before reaching real content; 6 raw lines comfortably
// covers a few blank separators while still stopping well short of an
// unrelated preceding paragraph. validLabeledName (shared with the
// label-path above) gates out junk — empty, too long, HTML-entity residue,
// sentence-shaped prose. Tried only when neither the suffix-anchored scan
// nor the labelName fallback found anything at all (see
// extractImprintText) — a stronger signal always wins.
func standaloneLegalFormCandidate(lines []string) string {
	for i, raw := range lines {
		if !standaloneLegalFormRE.MatchString(strings.TrimSpace(raw)) {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-6; j-- {
			prev := strings.TrimSpace(lines[j])
			if prev == "" {
				continue
			}
			if validLabeledName(prev) {
				return prev
			}
			return ""
		}
	}
	return ""
}

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

// hostingProsePrefixes are leading "this site is hosted by …" / "the owner
// of this website is …" disclaimers.
var hostingProsePrefixes = []string{
	"le site est hébergé par les ", "le site est hébergé par le ",
	"le site est hébergé par la ", "le site est hébergé par ",
	"site hébergé par les ", "site hébergé par le ",
	"site hébergé par la ", "site hébergé par ",
	"hébergé par les ", "hébergé par le ", "hébergé par la ", "hébergé par ",
	"this site is hosted by ", "the site is hosted by ",
	"website hosted by ", "site hosted by ", "hosted by ",
	"diese website wird gehostet von ", "gehostet von ", "gehostet bei ",
	// Spanish "owner of this website is" — real evidence:
	// eurostarshotels.com's real aviso legal opens "El propietario de esta
	// web es: EUROSTARS HOTEL COMPANY SL". "web" and "sitio web" are pure
	// synonyms in this construction (not a separate unevidenced pattern).
	"el propietario de esta web es: ", "el propietario de este sitio web es: ",
}

// trimAtConjunction handles "YOOX and Meta Platforms Ireland Limited" by
// keeping only the trailing entity (the one ending in the suffix).
//
// " & " (a literal ampersand) is deliberately NOT in this conjunction list.
// It was originally included, unevidenced, alongside the real "and"/"y"/
// "et"/"und"/"e" word-for-word translations this function's own doc
// comment cites real evidence for (YOOX/Meta) — but an ampersand is far
// more often part of a SINGLE company's own legal name (H&M, Procter &
// Gamble, ...) than a joiner between two DIFFERENT entities. Real
// evidence: bqredovisning.se's real, live privacy-policy page names
// itself "BQ Redovisning & Rådgivning AB" — trimming at " & " truncated
// the extracted candidate down to just "Rådgivning AB", silently dropping
// "BQ Redovisning" from the legal_name. Removed rather than special-cased,
// since no real page has ever evidenced "&" actually joining two distinct
// entities the way "and" does in the YOOX/Meta fixture.
func trimAtConjunction(s, suffix string) string {
	suffIdx := strings.LastIndex(s, suffix)
	if suffIdx < 0 {
		return s
	}
	for _, conj := range []string{" and ", " y ", " et ", " und ", " e "} {
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
		// Real evidence: japanphoto.no's real Kjøpsvilkår "17.
		// Kontaktinformasjon" section labels its postal-contact line
		// "Brev: CEWE Norge AS" (Norwegian for "Letter:") — the Norwegian
		// analogue of the existing "By Mail: " label above, which this
		// English-only list had no equivalent for at all.
		"Brev: ",
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
