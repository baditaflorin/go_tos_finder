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
	out := tagBoundaryPunctRE.ReplaceAllString(b.String(), "$1$2")
	return labelColonDigitBoundaryRE.ReplaceAllString(out, ": $1")
}

// labelColonDigitBoundaryRE collapses a tag-boundary newline sitting right
// after a label's own colon when a digit follows (skipping at most one
// literal space) — the "<strong>Label:</strong> 123456789" shape, distinct
// from tagBoundaryPunctRE just above (that one fixes a letter immediately
// before the break; this one fixes a colon immediately before it, with a
// DIGIT — not general prose — immediately after). Real evidence:
// thikishop.gr's real Όροι Χρήσης page writes "<strong>ΑΦΜ:</strong>
// 800617296" and "<strong>Αρ. Γ.Ε.ΜΗ.:</strong> 132389607000" — both labels
// sit entirely inside their own <strong>, so (unlike round 16's Czech
// "s.r.o." case) the LABEL text itself was never split — but the VALUE
// landing on its own line, with no label of its own on that same line,
// meant extractAddressNearEntity's per-line stop-marker checks (which only
// ever inspect the CURRENT line) had nothing to recognise and wrongly
// absorbed the orphaned value line into the address ahead of the real
// address further down the page. Restricted to "colon directly followed by
// a digit" (not any character) to avoid the broader, riskier scope of
// merging every colon-terminated line break unconditionally — a genuine
// section heading followed by non-numeric prose on the next line is left
// untouched.
var labelColonDigitBoundaryRE = regexp.MustCompile(`:\n ?(\d)`)

// tagBoundaryPunctRE collapses a newline that stripTagsLines just inserted
// at a tag boundary when the very next character is a bare "." or "," —
// i.e. undoes the line-break specifically where it would otherwise strand a
// trailing punctuation mark that belongs to the word before it, onto its
// own line. Real evidence: onlineshop.cz's real Obchodní podmínky page
// writes its entity name as "<strong>SHOP TRADING, s.r.o</strong>., se
// sídlem ..." — the closing "." (and the rest of the sentence) sits OUTSIDE
// the <strong> tag, a common real authoring pattern (bold entity name,
// plain trailing punctuation). Before this fix, stripTagsLines' own
// tag-boundary newline landed BETWEEN "s.r.o" and its own closing ".",
// turning what should read as the single contiguous string "s.r.o." into
// two separate lines ("SHOP TRADING, s.r.o" / "., se sídlem ...") — so
// suffixTable's literal "s.r.o." (dot-terminated, like most CZ/PL/civil-law
// abbreviation suffixes) never matched at all, and the ENTIRE
// suffix-anchored scan silently found nothing on the page (same
// total-extraction-failure shape as round 5/9/12/13/14's Polish/Irish/
// Danish/Finnish/Norwegian gaps). Restricted to only "."/"," (not e.g. ")"
// or ":") since those are the only two real punctuation marks confirmed to
// appear split this way so far.
//
// Gated on the char immediately BEFORE the newline being a Unicode LETTER
// (\p{L}), not just "any char" — an earlier version of this fix collapsed
// every "\n."/"\n," unconditionally and broke a real, existing regression:
// eurostarshotels.com's real aviso legal has a bare phone number in its own
// <a href="tel:...">932681010</a> tag, immediately followed by ". Para más
// formas de contacto..." outside the tag — the SAME tag-boundary shape as
// the CZ suffix case, but here the line break is a genuine sentence
// boundary, and bareIntlPhoneRE (this file) depends on the phone number
// staying isolated on its own line to be recognised and excluded from the
// address. Unconditional collapsing merged the two, defeating that
// isolation and leaking the phone number into the address. Requiring a
// LETTER (not a digit) immediately before the break correctly distinguishes
// "mid-abbreviation split" (CZ: "...s.r.o" ends in a letter) from
// "end-of-sentence-after-a-number split" (ES: "...932681010" ends in a
// digit) without needing to special-case phone numbers specifically.
var tagBoundaryPunctRE = regexp.MustCompile(`(\p{L})\n([.,])`)

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

// inlineOfAddressRE matches the UK/Malta-style company-law drafting
// convention "<Name> (<company number>) of <address> ("<nickname>")" —
// see extractAddressNearEntity's doc comment below for the real evidence.
// Anchored to the START of the text immediately following the entity name
// (optionally through one short parenthetical, e.g. a company-number
// citation) so it only fires on that specific adjacent-clause shape, not
// on any later "of" appearing deeper in the same line/sentence.
var inlineOfAddressRE = regexp.MustCompile(`^\s*(?:\([^()]{0,40}\)\s*)?of\s+`)

// registeredOfficeAddressRE matches the UK company-law disclosure phrasing
// "Registered Office is <address>" / "Registered Office: <address>" — a
// same-line inline-address shape distinct from inlineOfAddressRE's Malta
// "(<number>) of <address>" convention: here the address-introducing clause
// isn't adjacent to the entity name at all (it follows an intervening
// "is registered in <jurisdiction> under company number <n>, " clause), so
// it's searched for anywhere in the text following the name rather than
// anchored to the start of it. See extractAddressNearEntity's doc comment
// below for the real evidence (swetenhams.co.uk). Whitespace class mirrors
// stripTradingNamePrefix's tradingNamePrefixRE: stripTagsLines never
// decodes NBSP-shaped entities, and real UK pages use them freely between
// words in exactly this kind of disclosure sentence.
var registeredOfficeAddressRE = regexp.MustCompile(`(?i)registered(?:\s|&#160;|&nbsp;)+office(?:\s|&#160;|&nbsp;)+(?:is|:)(?:\s|&#160;|&nbsp;)+`)

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
		// Same-line inline address: UK/Malta-style company-law drafting
		// states "<Name> (<company number>) of <address> ("<nickname>")
		// is a ..." — the whole clause sits on ONE line (no <br> splits
		// name from address), so the forward per-line scan below (which
		// only ever looks at lines AFTER the one naming `name`) never
		// sees it; the line containing `name` is itself skipped for
		// address purposes everywhere else in this function. Captured as
		// the text between the adjacent "of " token (inlineOfAddressRE,
		// anchored right after the name/company-number clause) and the
		// next "(" on the same line, since that paren always opens the
		// defined-term nickname clause in this drafting style. Real
		// evidence: artemisialtd.com's real Terms of Sale page:
		// "Artemisia Fine Arts & Antiques Limited (C 71943) of 'Ridge
		// View', Triq is-Sagra Familja, Bidnija, Mosta MST 5012, Malta
		// ("Artemisia"; "we"; "us"; or "our") is a specialist vendor...".
		if nameIdx := strings.Index(line, name); nameIdx >= 0 {
			rest := line[nameIdx+len(name):]
			if loc := inlineOfAddressRE.FindStringIndex(rest); loc != nil {
				seg := rest[loc[1]:]
				if pIdx := strings.IndexByte(seg, '('); pIdx >= 0 {
					seg = seg[:pIdx]
				}
				seg = collapseSpaces(strings.TrimSpace(seg))
				seg = strings.Trim(seg, " ,;")
				if seg != "" && looksAddressLine(seg) {
					parts = append(parts, seg)
				}
			}
			// Same-line inline address, alternate UK phrasing: "<Name>
			// is registered in <jurisdiction> under company number <n>,
			// Registered Office is <address>." -- the address-
			// introducing clause isn't adjacent to the name (unlike the
			// Malta shape above), so it's searched for anywhere in
			// `rest` rather than anchored to its start. Only tried when
			// the Malta shape didn't already claim this line's address.
			// Cut at the next "(" (a possible trailing defined-term
			// clause) or sentence-ending "." (a UK postal address never
			// itself contains a period), whichever comes first. Real
			// evidence: swetenhams.co.uk's real legal-notices page:
			// "Swetenhams is a trading name of Sequence (UK) Limited is
			// registered in England and Wales under company number
			// 4268443, Registered Office is Cumbria House, 16-20
			// Hockliffe Street, Leighton Buzzard, Bedfordshire, LU7
			// 1GN.  VAT Registration Number is 500 2481 05.".
			if len(parts) == 0 {
				if loc := registeredOfficeAddressRE.FindStringIndex(rest); loc != nil {
					seg := rest[loc[1]:]
					if pIdx := strings.IndexByte(seg, '('); pIdx >= 0 {
						seg = seg[:pIdx]
					}
					if dIdx := strings.IndexByte(seg, '.'); dIdx >= 0 {
						seg = seg[:dIdx]
					}
					seg = collapseSpaces(strings.TrimSpace(seg))
					seg = strings.Trim(seg, " ,;")
					if seg != "" && looksAddressLine(seg) {
						parts = append(parts, seg)
					}
				}
			}
		}
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
			// A line that itself repeats the entity's own `name` marks a
			// SEPARATE mention of the entity (e.g. a footer signature
			// block restating "epic ltd | 87 Kennedy Avenue | ..." further
			// down the same page), not a continuation of the CURRENT
			// occurrence's address — stop here rather than absorbing that
			// whole separate block (which would otherwise re-include the
			// entity name itself, along with whatever it lists, as
			// "address" content). Real evidence: epic.com.cy's real
			// eStore Terms & Conditions page mentions "epic ltd" once
			// mid-paragraph (whose own candidate-name extraction fails —
			// a separate, honestly-documented gap) and again, cleanly, in
			// its real page footer much further down; without this stop,
			// the mid-paragraph occurrence's forward scan reached all the
			// way down to that footer line and absorbed it whole.
			if strings.Contains(low, strings.ToLower(name)) {
				break
			}
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
			// "cégjegyzékszám"/"adószám": Hungary's own register/tax-number
			// labels (see the "Cégjegyzékszám"/"Adószám" vatPatterns
			// entries in imprint_vat.go). Same shape as NIP/KRS/REGON just
			// above, not the RCS/TVA break-markers further below: real
			// evidence, szatmari-izek.shop.hu's real Impresszum page lists
			// both BEFORE the real "Székhely:" (registered-seat) address
			// line ("Név: ...<br />Cégjegyzékszám: ...<br />Adószám:
			// ...<br />Székhely: ...") — skip (not break) so scanning
			// continues past them to the real address that follows. Long,
			// specific compound words — safe as plain substring checks,
			// unlike "cui" (an ordinary short Romanian word) which needed
			// cuiWordRE's word-boundary regex further below.
			if strings.Contains(low, "cégjegyzékszám") || strings.Contains(low, "adószám") {
				continue
			}
			// "αφμ"/"γ.ε.μη.": Greece's own tax/register-number labels (see
			// the "ΑΦΜ"/"Γ.Ε.ΜΗ." vatPatterns entries in imprint_vat.go).
			// Same shape as the Hungarian markers just above: real
			// evidence, thikishop.gr's real Όροι Χρήσης page lists both
			// BEFORE the real "Έδρα/Διεύθυνση:" (registered-seat) address
			// line — skip (not break) so scanning continues past them.
			// "γ.ε.μη." (with its periods) is distinctive enough for a
			// plain substring check; "αφμ" is short like "cui" was
			// (round 15), but Go's RE2 \b is ASCII-only and can never
			// match adjacent to a Greek letter (see the "ΑΦΜ" vatPattern's
			// doc comment) — cuiWordRE's word-boundary-regex approach isn't
			// available here, so this stays a plain substring check. Real
			// evidence found no unrelated Greek word containing "αφμ" as a
			// substring; accepted as the same low residual risk this
			// codebase already tolerates for "rcs"/"tva"/"nip" below.
			if strings.Contains(low, "αφμ") || strings.Contains(low, "γ.ε.μη.") {
				continue
			}
			// "еик": Bulgaria's own register/tax-number label (see the
			// "ЕИК" vatPatterns entry in imprint_vat.go). Same shape as
			// the Hungarian/Greek markers above: real evidence, cressi.bg's
			// real Общи условия page lists "Наименование Кеме ЕООД, ЕИК
			// 201845795;" BEFORE the real "Седалище и адрес на управление"
			// address line — skip (not break) so scanning continues past
			// it. Same residual-risk acceptance as Greek "αφμ" above (a
			// short, specific Cyrillic abbreviation, plain substring check
			// since RE2's \b can't help here either).
			if strings.Contains(low, "еик") {
				continue
			}
			// "ičo:"/"dič:"/"ič dph"/"iban": Czech/Slovak IČO's own domestic
			// tax-registration labels (round 16 added the IČO identifier
			// itself but never an address stop-marker for it, since that
			// round's own fixture didn't need one — this round's real
			// Slovak evidence does). Includes the colon in "ičo:"/"dič:"
			// deliberately, unlike the bare-word markers elsewhere in this
			// function: "IČO"/"DIČ" without it would risk false-positiving
			// on ordinary Czech/Slovak words that happen to contain the
			// same short letter run (e.g. "dedičom", "dič" mid-word) —
			// the label is essentially always colon-terminated in real
			// imprint text, so requiring it keeps the match specific.
			// "ič dph" (Slovak "IČ DPH" = VAT number label, two words) and
			// "iban" are both distinctive enough on their own. Real
			// evidence: elektro-siete.sk's real Obchodné podmienky page
			// lists "IČO: 46775994<br />DIČ: 2023571528<br />IČ DPH:
			// SK2023571528" then "IBAN: SK67 ..." as four separate lines,
			// each with a long digit run that cleared looksAddressLine and
			// got wrongly absorbed into Address ahead of (and instead of)
			// the real "Sídlo spoločnosti" address a few lines earlier.
			if strings.Contains(low, "ičo:") || strings.Contains(low, "dič:") ||
				strings.Contains(low, "ič dph") || strings.Contains(low, "iban") {
				continue
			}
			// "číslo účtu" (Slovak "account number") and "obchodnom
			// registri" (Slovak "Commercial Register", the phrase this
			// page's own court-register citation line uses) — same real
			// page, same failure shape as the IČO/DIČ/IBAN block just
			// above: both lines have their own long digit runs (a bank
			// account number; the register's own insert/vložka number)
			// that cleared looksAddressLine and got wrongly absorbed once
			// the IČO/DIČ/IBAN lines ahead of them were excluded.
			if strings.Contains(low, "číslo účtu") || strings.Contains(low, "obchodnom registri") {
				continue
			}
			// "matična številka" (Slovenian company-registration-number
			// label) and "davčna številka" (Slovenian tax-number label) —
			// same failure shape as the Czech/Slovak block just above:
			// real evidence, sgermobil.si's real Splošni pogoji page lists
			// both right after the real "2000 Maribor" postal-code/city
			// line, each with its own long digit run that cleared
			// looksAddressLine and got wrongly absorbed alongside it.
			if strings.Contains(low, "matična številka") || strings.Contains(low, "davčna številka") {
				continue
			}
			// "vložno številko" (Slovenian "insert number" — the court
			// commercial-register citation, e.g. "registrirana pri
			// Okrožnem sodišču v Mariboru ... pod vložno številko
			// 1/12603/00"). Same failure shape as round 10's Luxembourg
			// "RCS Luxembourg n°..." fix: the citation's own digit run
			// cleared looksAddressLine and got wrongly absorbed into the
			// address on the same real sgermobil.si page.
			if strings.Contains(low, "vložno številko") {
				continue
			}
			// "reģistrācijas numurs" (Latvian registration-number label)
			// and "pvn maksātāja numurs" (Latvian VAT-payer-number label)
			// — same shape as the markers above: real evidence,
			// gatavosana.lv's real Juridiskā informācija page lists both
			// right BEFORE the real "Juridiskā adrese:" (legal address)
			// line, each with its own long digit run that cleared
			// looksAddressLine and got wrongly absorbed ahead of it.
			if strings.Contains(low, "reģistrācijas numurs") || strings.Contains(low, "pvn maksātāja numurs") {
				continue
			}
			// "kmkr"/"registrikood": Estonian VAT-number and registry-code
			// labels (see the "registrikood" vatPatterns entry in
			// imprint_vat.go). Same shape as the markers above: real
			// evidence, voluaed.ee's real Müügitingimused page has "KMKR
			// nr: EE101490959" on its own line, whose own digit run
			// cleared looksAddressLine and got wrongly absorbed. "kmkr" is
			// a 4-letter acronym, not an ordinary word in any language
			// this codebase covers — safe as a plain substring check.
			if strings.Contains(low, "kmkr") || strings.Contains(low, "registrikood") {
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
		// A literal "•" (U+2022 BULLET) list-marker character is the same
		// kind of line/list-item boundary as '\n' above, just rendered as
		// plain text instead of a tag — real evidence: mako.hr's real Opći
		// uvjeti page writes each defined term as its own bullet line
		// ("<br />\n• Mako znači Mako d.o.o., sa sjedištem..."), and
		// without this stop the backward scan climbed straight through the
		// bullet into the PRECEDING sentence, capturing "• Mako znači Mako
		// d.o.o." (the bullet plus the defining clause's own subject and
		// verb) as the candidate name instead of just "Mako d.o.o.".
		// Checked via HasPrefix (not a plain byte comparison) since "•" is
		// a 3-byte UTF-8 sequence and this loop walks the string one byte
		// at a time; HasPrefix still correctly fires only when i lands
		// exactly on the bullet's own leading byte.
		if strings.HasPrefix(text[i:], "•") {
			leftCut = i + len("•")
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
	candidate = stripTradingNamePrefix(candidate)
	candidate = stripAwardPrefix(candidate)
	candidate = stripLabelPrefix(candidate)
	candidate = trimAtConjunction(candidate, suffix)
	candidate = collapseSpaces(candidate)
	if cleanCandidateName(candidate, suffix) {
		return candidate
	}
	// Prefix-form fallback (see the doc comment above): tried when nothing
	// meaningful precedes the suffix on this line — a real name-then-suffix
	// match earlier on the same line always wins first — OR when the
	// preceding text is just a role/label ending in a dash-style separator
	// ("Pardavėjas – UAB „...\"", "Seller — Ltd ...").
	// Real evidence: grupinispirkimas.lt's real Taisyklės page writes
	// "Pardavėjas &#8211; UAB „GRUPINIS PIRKIMAS"" — the pure-blank check
	// alone never tried the prefix-form path at all here, since "1.2.
	// Pardavėjas &#8211;" precedes "UAB" and is not blank, even though it
	// is unambiguously just a role label, not a failed name-then-suffix
	// attempt. Recognising the dash separator narrows this without
	// broadening the fallback to arbitrary non-blank prefixes (which
	// would risk masking genuine name-then-suffix rejections elsewhere).
	// The dash itself is a literal, un-decoded numeric HTML entity on
	// this real page (stripTagsLines never decodes "&#8211;" — same
	// deliberate non-decoding as "&nbsp;"/"&quot;"/"&copy;" elsewhere in
	// this codebase), so both the real Unicode dash characters AND their
	// common un-decoded entity spellings are checked.
	preceding := strings.TrimSpace(text[startMin:idx])
	hasDashSuffix := strings.HasSuffix(preceding, "–") || strings.HasSuffix(preceding, "—") ||
		strings.HasSuffix(preceding, "-") || strings.HasSuffix(preceding, "&#8211;") ||
		strings.HasSuffix(preceding, "&#8212;") || strings.HasSuffix(preceding, "&ndash;") ||
		strings.HasSuffix(preceding, "&mdash;")
	if preceding == "" || hasDashSuffix {
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
		if r == '\n' || r == '\r' || r == '|' || r == ',' {
			cut = i
			break
		}
		if r == '.' {
			// A period immediately followed by a short run of uppercase
			// letters, then a non-letter (or end of string), reads as a
			// domain-style suffix ("SOLLER.LV") rather than a sentence
			// boundary — real evidence, gatavosana.lv's real Juridiskā
			// informācija page names its own entity "SIA SOLLER.LV"; the
			// unconditional "stop at the first period" rule truncated the
			// real trading name to just "SOLLER", silently dropping its
			// own genuine ".LV" suffix. Only recognised for this specific
			// shape (not periods in general) so a genuine sentence
			// boundary right after the suffix ("SARL. Fondée en 2020...")
			// still stops the scan exactly as before.
			if looksLikeDomainSuffixPeriod(after, i) {
				continue
			}
			cut = i
			break
		}
	}
	name := strings.TrimSpace(after[:cut])
	name = stripQuoteDelimiters(name)
	if name == "" {
		return ""
	}
	if !cleanCandidateName(name+" "+suffix, suffix) {
		return ""
	}
	return suffix + " " + name
}

// looksLikeDomainSuffixPeriod reports whether the '.' at byte offset
// periodIdx in s is immediately followed by 2-4 uppercase ASCII letters
// and then either the end of the string or a non-letter character — the
// shape of a domain-style suffix (".LV", ".COM", ".INFO", ...) rather than
// a sentence-ending full stop.
func looksLikeDomainSuffixPeriod(s string, periodIdx int) bool {
	rest := s[periodIdx+1:]
	n := 0
	for n < len(rest) && rest[n] >= 'A' && rest[n] <= 'Z' {
		n++
	}
	if n < 2 || n > 4 {
		return false
	}
	if n == len(rest) {
		return true
	}
	next := rest[n]
	return !((next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z'))
}

// stripQuoteDelimiters trims a leading/trailing quote-style delimiter pair
// wrapping a prefix-form trading name (e.g. "UAB „GRUPINIS PIRKIMAS"" —
// the low-9-quote convention several European languages, including
// Lithuanian, share). Real evidence: grupinispirkimas.lt's real Taisyklės
// page writes this literally as un-decoded numeric entities
// ("&#8222;GRUPINIS PIRKIMAS&#8221;") — stripTagsLines never decodes them
// (same deliberate non-decoding as "&nbsp;"/"&quot;" elsewhere), so
// cleanCandidateName's entity-corruption filter (which correctly rejects
// genuinely broken "&#..." residue) was rejecting this real, well-formed
// quoted name outright. The delimiters are pure punctuation around the
// name, not part of it, so they're stripped here — BEFORE the corruption
// check ever sees them — rather than loosening that filter itself. Real
// Unicode quote characters are handled too, for a page that has already
// decoded them.
func stripQuoteDelimiters(s string) string {
	for _, open := range []string{"&#8222;", "„"} {
		if strings.HasPrefix(s, open) {
			s = strings.TrimSpace(s[len(open):])
			break
		}
	}
	for _, closer := range []string{"&#8221;", "”"} {
		if strings.HasSuffix(s, closer) {
			s = strings.TrimSpace(s[:len(s)-len(closer)])
			break
		}
	}
	return s
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
		// A lowercase-initial candidate is usually a mid-sentence fragment
		// ("...through the eStore of epic ltd." captured from too far
		// back) — looksLikeSentencePhrase above already rejects most of
		// those via stop-word density, but this check exists as a second,
		// blunter net for the ones that slip through. Real evidence,
		// round 23: epic.com.cy's real footer line "epic ltd | 87 Kennedy
		// Avenue | ..." names its OWN entity as deliberately all-lowercase
		// "epic ltd" (a real brand-styling choice, not a sentence
		// fragment) — an unconditional reject here discarded the one
		// clean candidate this real page actually has. Narrowed to word
		// count: a genuine sentence fragment needs several words
		// (articles/prepositions/verbs) to be grammatical; a brand-name
		// stem is virtually always 1-2 words. Only reject 3-plus-word
		// stems — a stronger, more specific signal than "starts lowercase"
		// alone, and it doesn't touch the (already-uppercase, unaffected)
		// vast majority of real candidates this codebase has verified.
		if stemWords := strings.Fields(strings.TrimSpace(strings.TrimSuffix(name, suffix))); len(stemWords) > 2 {
			return false
		}
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
	// " znači " (Croatian "means") isn't grammatically a conjunction like
	// the others below, but plays the same mechanical role for this
	// function's purpose: real evidence, mako.hr's real Opći uvjeti page
	// defines its entity as "Mako znači Mako d.o.o., sa sjedištem..." — a
	// repeated-name defining clause ("[Term] means [Full legal name]").
	// Without trimming at "znači", the backward-scanned candidate captured
	// "Mako znači Mako d.o.o." (the defining clause's own subject) instead
	// of just the real name "Mako d.o.o." that follows it.
	for _, conj := range []string{" and ", " y ", " et ", " und ", " e ", " znači "} {
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
		// Real evidence: szatmari-izek.shop.hu's real Impresszum page
		// labels its entity-name line "Név: Szatmári Ízek Kft." (Hungarian
		// for "Name:") — same shape as "Brev: " above, no Hungarian
		// analogue existed in this list before.
		"Név: ",
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

// stripTradingNamePrefix strips a UK/Irish "trading name" disclosure
// preamble ("X is a trading name of Y Limited", "X trading as Y Ltd") down
// to just the actual legal entity name that follows. Real evidence:
// swetenhams.co.uk's real legal-notices page reads "Swetenhams is a
// trading name of Sequence (UK) Limited is registered in England and
// Wales..." — the stem "Swetenhams is a trading name of Sequence (UK)"
// contains two sentenceStopWords hits ("is", "of") and was rejected by
// cleanCandidateName as sentence-like before any strip*Prefix function
// ever ran, even though "Sequence (UK) Limited" is unambiguously the real
// legal entity name — the same class of gap stripPersonRolePrefix and
// stripAwardPrefix already close for their own preamble shapes, just for
// this one. "trading style of" and "trading as" are the same standard UK
// company-branding phrasing, not independently evidenced this round but
// structurally identical (strip through the marker, keep what follows).
var tradingNamePrefixRE = regexp.MustCompile(`(?i)is(?:\s|&#160;|&nbsp;)+a(?:\s|&#160;|&nbsp;)+trading(?:\s|&#160;|&nbsp;)+(?:name|style)(?:\s|&#160;|&nbsp;)+of(?:\s|&#160;|&nbsp;)+|trading(?:\s|&#160;|&nbsp;)+as(?:\s|&#160;|&nbsp;)+`)

func stripTradingNamePrefix(s string) string {
	// Word-by-word (?:\s|&#160;|&nbsp;)+ rather than a plain string search:
	// real evidence, swetenhams.co.uk's actual raw markup has a literal,
	// un-decoded "&#160;" (numeric NBSP entity) between "of" and
	// "Sequence" ("...trading name of&#160;Sequence (UK) Limited...") —
	// stripTagsLines never decodes NBSP-shaped entities (same deliberate
	// non-decoding as "&nbsp;"/"&quot;"/"&copy;" elsewhere in this
	// codebase), so a plain-space marker string silently never matched
	// this real page at all despite looking identical when printed.
	loc := tradingNamePrefixRE.FindStringIndex(s)
	if loc == nil {
		return s
	}
	return strings.TrimSpace(s[loc[1]:])
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
//
// collapseSpaces first: findIdentifiers (imprint_vat.go) deliberately keeps
// `value` as the RAW regex match, byte-identical to the source text, so
// extractImprintText's proximity-attachment step can look it back up via
// strings.Index — but a pattern whose label and digits sit on opposite
// sides of an HTML tag boundary (stripTagsLines inserts a real "\n" there)
// can match ACROSS that boundary, leaking a literal newline into this
// function's output otherwise. Real evidence: thikishop.gr's real
// "<strong>Αρ. Γ.Ε.ΜΗ.:</strong> 132389607000" markup, round 18.
func formatRegister(kind, value string) string {
	v := strings.TrimSpace(collapseSpaces(value))
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
	// A literal, un-decoded "&nbsp;" (see trimLeadingTrailingNBSP's doc
	// comment for why stripTagsLines never decodes it) commonly sits right
	// between a label's colon and its value — real evidence,
	// grupinispirkimas.lt's real "Įmonės kodas:&nbsp;302983374" left a
	// stray "&nbsp;" in this function's output otherwise, since it isn't
	// whitespace and the "#-." cutset above doesn't remove multi-character
	// substrings. Stripped here (and re-stripped below, for a second
	// "&nbsp;" or leftover punctuation immediately behind it) rather than
	// changing stripTagsLines' deliberate non-decoding globally.
	for strings.HasPrefix(v, "&nbsp;") {
		v = strings.TrimSpace(v[len("&nbsp;"):])
	}
	for _, lbl := range []string{"No. ", "No.", "No ", "Nummer ", "Numer ",
		"Number ", "Reg. ", "Reg ", "registered ", "Registered ",
		// Malta's "Company Registration Number IS C 71943" pattern
		// deliberately keeps the optional "is" inside its regex match (see
		// that Kind's vatPatterns doc comment — findIdentifiers keeps the
		// RAW match), so it needs stripping here too, the same way "No."/
		// "Reg."/"registered" already are. Real evidence: artemisialtd.com's
		// real Terms of Sale page: "Our company registration number is
		// C 71943".
		"is ", "Is "} {
		if strings.HasPrefix(v, lbl) {
			v = strings.TrimSpace(v[len(lbl):])
		}
	}
	v = strings.TrimLeft(v, " :.,#-")
	for strings.HasPrefix(v, "&nbsp;") {
		v = strings.TrimSpace(v[len("&nbsp;"):])
	}
	if v == "" {
		return kind
	}
	return kind + " " + v
}
