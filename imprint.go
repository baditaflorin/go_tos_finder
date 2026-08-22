package main

import (
	"encoding/hex"
	"regexp"
	"strings"
	"unicode"
)

// This file is the go_tos_finder-native orchestration layer on top of the
// field-extraction pipeline vendored from go_legal_entity (imprint_vat.go,
// imprint_checksum.go, imprint_country.go, imprint_suffix.go, imprint_name.go,
// imprint_jsonld.go). It turns the bare `imprint_present` boolean this
// service used to emit (a `merged[DocImprint]` map-presence check) into
// structured, field-level compliance evidence: EU e-Commerce Directive
// Art. 5, Germany's TMG §5 (→ DDG §5), and Austria's ECG §5 + Mediengesetz
// §§24-25 all require a compliant imprint to disclose specific fields
// (legal entity name, address, register number, VAT ID, responsible
// person, contact) — a page merely containing the word "Impressum" in its
// footer says nothing about whether any of that is actually present.

// Ruleset names. See imprintFieldChecklist for what each requires.
//
// RulesetFRLCEN / RulesetITDLgs70 / RulesetESLSSICE / RulesetNLHandelsreg
// were added once round-1/round-2 EU-expansion real evidence confirmed
// register-number extraction is reliably working for these four countries
// (SIRET/SIREN, REA, Hoja-RM, KvK — see imprint_vat.go). Unlike
// de_tmg/at_ecg_medieng, none of these four require a named responsible
// person: LCEN Art. 6-III (France), D.Lgs. 70/2003 Art. 7 (Italy), LSSICE
// Art. 10 (Spain), and the Dutch Handelsregisterwet/BW Art. 3:15d are all
// entity-identification obligations (name, address, register number,
// contact) — not the separate DE/AT media-law individual-accountability
// requirement. France's LCEN Art. 6-III-2 does ALSO require naming a
// "directeur de la publication" — responsiblePersonLabelRE (below)
// already recognises the French phrasing ("responsable de la
// publication", "représentant légal") — but that requirement is
// deliberately NOT added to RulesetFRLCEN's checklist yet: no real
// evidence in this expansion actually exercised that extraction path
// against a live French page, and requiring a field this pass never
// verified works would overclaim validation that wasn't done.
//
// RulesetPLUsude (round 5) follows the same shape: Poland's Ustawa o
// świadczeniu usług drogą elektroniczną (Act on Providing Services by
// Electronic Means — Poland's transposition of the e-Commerce Directive)
// requires firm name, address, register entry (KRS), tax ID (NIP), and
// contact — entity identification only, no named individual.
const (
	RulesetEUBaseline   = "eu_baseline"
	RulesetDETMG        = "de_tmg"
	RulesetATECGMedienG = "at_ecg_medieng"
	RulesetFRLCEN       = "fr_lcen"
	RulesetITDLgs70     = "it_dlgs70"
	RulesetESLSSICE     = "es_lssice"
	RulesetNLHandelsreg = "nl_handelsregisterwet"
	RulesetPLUsude      = "pl_usude"
)

// Imprint is the structured, field-level result of analysing a verified
// imprint/Impressum page's body. ImprintPresent (Response, back-compat) is
// derived from Imprint.Present.
type Imprint struct {
	Present           bool     `json:"present"`
	URL               string   `json:"url,omitempty"`
	LegalName         string   `json:"legal_name,omitempty"`
	Suffix            string   `json:"suffix,omitempty"`
	Address           string   `json:"address,omitempty"`
	Country           string   `json:"country,omitempty"` // ISO-3166-1 alpha-2
	Register          string   `json:"register,omitempty"`
	VAT               string   `json:"vat,omitempty"`
	VATValidation     string   `json:"vat_validation,omitempty"` // checksum_valid | checksum_invalid | format_valid | unchecked
	ResponsiblePerson string   `json:"responsible_person,omitempty"`
	FieldsFound       []string `json:"fields_found,omitempty"`
	FieldsMissing     []string `json:"fields_missing,omitempty"`
	CompletenessScore int      `json:"completeness_score"` // 0-100, relative to the applicable ruleset
	Ruleset           string   `json:"ruleset"`
}

// imprintEmailRE is a pragmatic (not RFC-5322-complete) email matcher — good
// enough to prove a functional contact channel exists on the page, which is
// all the eu_baseline "contact" checklist item needs.
var imprintEmailRE = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

// imprintPhoneRE requires an explicit "Tel/Phone/Fax/Mobile" label before the
// digit run — an unlabelled loose digit-run matcher would false-positive on
// postal codes, register numbers, and VAT IDs elsewhere on the same page.
var imprintPhoneRE = regexp.MustCompile(`(?i)\b(?:tel(?:efon)?|phone|fax|mobile)\.?\s*:?\s*(\+?\d[\d\s()\-./]{5,20}\d)`)

// cfEmailProtectionRE matches Cloudflare's "Email Address Obfuscation"
// anti-scraping markup — a real, extremely common pattern (verified live
// on brucebetcasino-at.com, and widespread well beyond that one site):
// Cloudflare rewrites a real <a href="mailto:...">  into
// <a href="/cdn-cgi/l/email-protection#<hex>"> and/or adds a
// data-cfemail="<hex>" attribute on a <span>. <hex> is Cloudflare's public,
// well-documented, reversible single-byte-XOR encoding (decodeCFEmail
// below) — this is exactly what the small inline JS snippet Cloudflare
// itself injects into the page does to reconstruct the address for a real
// browser, not a protection being bypassed.
var cfEmailProtectionRE = regexp.MustCompile(`(?:data-cfemail="([0-9a-fA-F]+)"|/cdn-cgi/l/email-protection#([0-9a-fA-F]+))`)

// decodeCFEmail reverses Cloudflare's single-byte-XOR email-obfuscation
// encoding: the first hex byte-pair is the XOR key, and every subsequent
// byte-pair XORed against that key recovers the original ASCII email one
// byte at a time. Returns "" for a malformed/odd-length hex string.
func decodeCFEmail(hexStr string) string {
	if len(hexStr) < 4 || len(hexStr)%2 != 0 {
		return ""
	}
	raw, err := hex.DecodeString(hexStr)
	if err != nil || len(raw) < 2 {
		return ""
	}
	key := raw[0]
	out := make([]byte, 0, len(raw)-1)
	for _, b := range raw[1:] {
		out = append(out, b^key)
	}
	return string(out)
}

// hasCFObfuscatedEmail reports whether the page carries a Cloudflare
// email-obfuscation blob that decodes to a plausible email address — a
// functional contact channel Cloudflare has hidden from plain-text
// scraping (imprintEmailRE alone cannot see it) but that every real
// visitor's browser reconstructs via Cloudflare's own injected JS.
func hasCFObfuscatedEmail(body string) bool {
	for _, m := range cfEmailProtectionRE.FindAllStringSubmatch(body, -1) {
		hexStr := m[1]
		if hexStr == "" {
			hexStr = m[2]
		}
		if imprintEmailRE.MatchString(decodeCFEmail(hexStr)) {
			return true
		}
	}
	return false
}

// hasImprintContact reports whether the page exposes a functional contact
// channel (email, a clearly-labelled phone number, or a Cloudflare
// email-obfuscation blob that decodes to one) — the eu_baseline "contact"
// checklist item.
func hasImprintContact(body string) bool {
	return imprintEmailRE.MatchString(body) || imprintPhoneRE.MatchString(body) || hasCFObfuscatedEmail(body)
}

// responsiblePersonLabelRE matches the German (TMG §5 / RStV) and Austrian
// (ECG §5 / Mediengesetz §§24-25 "verantwortlich für den Inhalt") phrasing
// that names the natural person legally responsible for the page, plus
// English/French near-equivalents. Named per-language capture groups mirror
// this fleet's existing multi-language legal-term regex style (see e.g.
// go_gdpr_compliance's dpoMentionRE) — the LABEL is what these groups match;
// extractResponsiblePerson then windows forward from the label's end to pull
// the person's name that follows it.
var responsiblePersonLabelRE = regexp.MustCompile(`(?i)` +
	`(?P<de>Gesch[äa]ftsf[üu]hrer(?:in)?|vertretungsberechtigte?r?|verantwortlich\s+(?:im\s+Sinne\s+des?\s+§\s?\d+\S*\s+)?(?:für\s+den\s+Inhalt|gem[äa][ßs]\s+§)|inhaltlich\s+verantwortlich)` +
	`|(?P<en>managing\s+director|legal\s+representative|responsible\s+for\s+(?:the\s+)?content|represented\s+by)` +
	`|(?P<fr>repr[ée]sentant\s+l[ée]gal|responsable\s+de\s+la\s+publication|directeur\s+g[ée]n[ée]ral)`)

// extractResponsiblePerson finds the first responsiblePersonLabelRE hit and
// windows forward to the name that follows it. The label is often followed
// by a role qualifier before the actual name ("Geschäftsführerin: Maria
// Musterfrau") or a bare connector with no punctuation ("vertretungs-
// berechtigt durch Maria Musterfrau"): cutting at the LAST colon/dash on the
// line strips the common labelled case; the capitalised-token scan (skipping
// any leading lowercase connector word) handles the unlabelled case.
func extractResponsiblePerson(text string) string {
	loc := responsiblePersonLabelRE.FindStringIndex(text)
	if loc == nil {
		return ""
	}
	after := text[loc[1]:]
	cut := len(after)
	for i, r := range after {
		if r == '\n' || r == '\r' || r == '|' {
			cut = i
			break
		}
	}
	line := after[:cut]
	if i := strings.LastIndexAny(line, ":–-"); i >= 0 {
		line = line[i+1:]
	}
	line = strings.TrimSpace(line)
	fields := strings.Fields(line)
	var nameParts []string
	started := false
	for _, w := range fields {
		if len(nameParts) >= 4 {
			break
		}
		clean := strings.Trim(w, ".,;:")
		if clean == "" {
			continue
		}
		r := []rune(clean)
		if !unicode.IsUpper(r[0]) {
			if started {
				break
			}
			continue // skip a leading connector word (durch/by/von/par...)
		}
		started = true
		nameParts = append(nameParts, clean)
	}
	if len(nameParts) == 0 {
		return ""
	}
	return strings.Join(nameParts, " ")
}

// titleCaseNameRun scans s for the longest run of two-or-more consecutive
// Title-Case tokens (first rune upper, every other rune lower — rejecting
// ALL-CAPS acronyms) and returns it joined by spaces, or "" if no such run
// exists. Used as the Bug 3 fallback below: a sole trader's legal_name
// often already contains the responsible natural person's full name inline
// (e.g. real JSON-LD from hotelrose.at: "Aktivhotel zur Rose - Franz
// Holzmann" — "zur" breaks the run, leaving "Franz Holzmann"). This is a
// coarse heuristic, not a real name-entity recognizer — it is only invoked
// when no legal-form suffix was found at all (see extractImprintFields),
// which keeps it away from GmbH/AG-style names where a distinct
// managing-director line is the norm. Known limitation: when the legal_name
// itself is nothing BUT title-cased words with no lowercase connector to
// break the run (e.g. a bare plain-text label match with no JSON-LD's
// grammatically-correct lowercase preposition to disambiguate), this can
// mistake the whole trading name for a person's name rather than finding
// none. Accepted here since the real evidence this fix targets (a JSON-LD
// name field, which reliably carries natural lowercase connectors like
// "zur"/"von"/"and") does not hit that case.
func titleCaseNameRun(s string) string {
	isTitleCaseWord := func(w string) bool {
		r := []rune(w)
		if len(r) < 2 || !unicode.IsUpper(r[0]) {
			return false
		}
		for _, c := range r[1:] {
			if unicode.IsUpper(c) {
				return false
			}
		}
		return true
	}
	var run []string
	best := ""
	flush := func() {
		if len(run) >= 2 {
			if joined := strings.Join(run, " "); len(joined) > len(best) {
				best = joined
			}
		}
		run = nil
	}
	for _, w := range strings.Fields(s) {
		clean := strings.Trim(w, ".,;:-")
		if isTitleCaseWord(clean) {
			run = append(run, clean)
		} else {
			flush()
		}
	}
	flush()
	return best
}

// courtRE matches the register court named alongside a German Handelsregister
// or Austrian Firmenbuch entry — Amtsgericht (DE), Handelsgericht/
// Landesgericht (AT, acting as Firmenbuchgericht), or an explicit
// "Firmenbuchgericht"/"Registergericht" label. The Imprint struct has a
// single Register field (no separate "court" slot), so when found this is
// appended to Register rather than tracked separately — it's what lets the
// de_tmg/at_ecg_medieng "register + court" checklist item be satisfied by
// the one field the struct actually exposes.
var courtRE = regexp.MustCompile(`(?i)\b(?:Amtsgericht|Handelsgericht|Landesgericht|Firmenbuchgericht|Registergericht)\s+[\p{L}][\p{L}\.\- ]{1,40}`)

func courtMention(text string) string {
	return strings.TrimSpace(courtRE.FindString(text))
}

// candidateSourceRank / candidateConfRank implement go_legal_entity's
// candidate-selection precedence (handler_merge.go's rank/confRank):
// structured data (JSON-LD) outranks the suffix-anchored text scan, which
// outranks hCard, footer copyright lines, and finally the weak og:site_name
// fallback; ties broken by country-inference confidence.
func candidateSourceRank(source string) int {
	switch source {
	case "json_ld":
		return 0
	case "imprint_text":
		return 1
	case "imprint_label":
		// The sole-trader label fallback (Bug 2 — see extractImprintText's
		// doc comment): a bare "Firmenname:"-style label is a real but
		// weaker signal than an actual legal-form suffix match
		// ("imprint_text"), so it ranks just below it, still ahead of
		// hCard/copyright-footer/og:site_name.
		return 2
	case "hcard":
		return 3
	case "copyright_footer":
		return 4
	case "og_site_name":
		return 5
	}
	return 9
}

func candidateConfRank(c string) int {
	switch c {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	}
	return 9
}

// mergeImprintCandidates deduplicates same-page candidates that name the same
// entity (case-insensitive exact name match) and fills field gaps across
// them — e.g. a JSON-LD block naming the company + VAT, alongside the same
// company's Handelsregister number only present in the visible page text,
// are two different imprintCandidate values (different Source) for the
// SAME company. Without this merge, picking one source as "best" would
// silently drop whatever field only the other source captured. Adapted from
// go_legal_entity's handler_merge.go mergeCandidates, trimmed to what a
// single already-fetched page needs (no cross-page FoundOn provenance).
func mergeImprintCandidates(in []imprintCandidate) []imprintCandidate {
	idx := map[string]int{}
	var out []imprintCandidate
	for _, c := range in {
		if c.Name == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(c.Name))
		if i, ok := idx[key]; ok {
			cur := out[i]
			if cur.Register == "" {
				cur.Register = c.Register
			}
			if cur.VAT == "" {
				cur.VAT = c.VAT
			}
			if cur.Address == "" {
				cur.Address = c.Address
			}
			if cur.Country == "" {
				cur.Country = c.Country
				cur.Confidence = c.Confidence
			}
			if cur.Suffix == "" {
				cur.Suffix = c.Suffix
			}
			cur.Identifiers = mergeIdentifiers(append(cur.Identifiers, c.Identifiers...))
			// Keep the higher-priority source for tie-break purposes (the
			// merged record now carries the union of fields regardless).
			if candidateSourceRank(c.Source) < candidateSourceRank(cur.Source) {
				cur.Source = c.Source
			}
			out[i] = cur
			continue
		}
		idx[key] = len(out)
		out = append(out, c)
	}
	return out
}

// hasProximityCorroboration gates the two weak, plain-text-sourced candidate
// kinds (imprint_text — a bare legal-form-suffix match; imprint_label — a
// bare "Firmenname:"-style label match) behind a minimal corroboration
// requirement before either is trusted enough to WIN as bestImprintCandidate's
// pick: the candidate must carry at least one other imprint-shaped signal
// found near it on the page — a real postal address (Address, from
// extractAddressNearEntity's forward-line, contact-line-gated scan) or a
// VAT/register identifier (Register/VAT, from extractImprintText's
// 800-byte nearest-candidate proximity attachment). All three fields are
// already proximity-gated at the point they're populated, so simply
// requiring one of them to be non-empty is enough — no separate distance
// check needed here.
//
// Real evidence: stress-testing the shipped 1.5.2 extractor against
// atpkitz.at (a real Austrian gambling-comparison site) found it happily
// extracted "Winrolla GmbH" as legal_name from marketing prose reviewing
// third-party operators ("Winrolla GmbH ist ein weiterer Top-Anbieter mit
// MGA-Lizenz") — a real false positive: this kind of comparison/review page
// structurally MUST name the brands it's comparing, and a bare suffix match
// with nothing else nearby (no address, no register, no VAT) is exactly the
// shape of "brand mentioned in passing," not "this is who runs this site."
// See TestExtractImprintFieldsRejectsUncorroboratedThirdPartyBrandMention.
//
// A bare nearby contact email/phone was deliberately NOT added as a third
// qualifying signal, even though it reads as a natural candidate: this
// exact bug's own adversarial fixture has a real contact email only ~84
// bytes after the false-positive "Winrolla GmbH" match (a separate
// marketing paragraph's "Kontaktieren Sie uns: ..." call-to-action), which
// would have defeated this fix outright had "any contact info nearby" been
// accepted as sufficient corroboration. Contact info is near-ubiquitous on
// any commercial page and sits close to arbitrary text by page-layout
// coincidence, not entity ownership — it doesn't discriminate the way a
// genuine address or register/VAT number does. The page-wide "contact"
// checklist item (hasImprintContact, unrelated to this gate) is unaffected.
//
// Structured-data sources (json_ld, hcard) are never subject to this check
// — those are already high-confidence; this gate exists specifically to
// tighten the weakest source, plain-text suffix/label scanning, which is
// exactly where this false positive originated. copyright_footer and
// og_site_name are also left ungated: out of scope for this fix (see the
// bug report), and structurally narrower extractors (a single copyright
// line / a single og:site_name meta value, not a whole-page suffix scan).
func hasProximityCorroboration(c *imprintCandidate) bool {
	if c.Source != "imprint_text" && c.Source != "imprint_label" {
		return true
	}
	return c.Address != "" || c.Register != "" || c.VAT != ""
}

// bestImprintCandidate picks the highest-priority named candidate that also
// clears hasProximityCorroboration — a named-but-uncorroborated weak
// plain-text candidate is skipped entirely rather than won, so a page with
// nothing but such candidates correctly ends up with no legal_name at all
// (Present still stays true; see extractImprintFields) instead of
// hallucinating one from unrelated prose.
func bestImprintCandidate(cands []imprintCandidate) *imprintCandidate {
	var best *imprintCandidate
	bestScore := 1 << 30
	for i := range cands {
		c := &cands[i]
		if c.Name == "" {
			continue
		}
		if !hasProximityCorroboration(c) {
			continue
		}
		score := candidateSourceRank(c.Source)*10 + candidateConfRank(c.Confidence)
		if score < bestScore {
			bestScore = score
			best = c
		}
	}
	return best
}

// backfillWinnerIdentifiers fills Register/VAT gaps on the winning candidate
// (as picked by bestImprintCandidate) from any OTHER candidate in the same
// single-page candidate list that has them — regardless of whether the
// names match. This is deliberately looser than mergeImprintCandidates'
// exact-name-match merge: that function exists to combine genuinely-
// duplicate same-named candidates (e.g. JSON-LD + hCard both naming the
// identical company) and its strict matching is right-sized for
// go_legal_entity's original multi-page/multi-entity crawl context, where a
// name mismatch plausibly means two different real companies on two
// different pages. extractImprintFields only ever runs on a single
// already-fetched, already-verified imprint page (see its doc comment), so
// that risk doesn't apply here: an identifier (VAT/register) found ANYWHERE
// in this one page's own candidate list is attributable to the one entity
// the page is about — a same-page proximity/format-based identifier find
// (see extractImprintText) is already itself the evidence of entity
// association, independent of whether the extractor that found the name
// happened to capture it in exactly the same casing/wording as the winning
// candidate's name.
//
// Real evidence: hotelrose.at's real Impressum has JSON-LD naming "Aktivhotel
// zur Rose - Franz Holzmann" (no vatID field at all) winning as best over a
// plain-text "Firmenname: Aktivhotel Zur Rose" candidate that DID capture
// the page's visible "UID-Nr.: ATU43951103" via proximity attachment — two
// different strings under strings.ToLower, so mergeImprintCandidates never
// combines them and the JSON-LD winner's VAT silently stayed empty even
// though the VAT was genuinely present and found. Register is not usually
// affected by this same gap in practice, because courtMention (below, in
// extractImprintFields) already independently scans the whole page's text
// for a named register court outside of any candidate at all — but a
// candidate-attached register NUMBER (HRB/FN/etc., as opposed to a bare
// court name) can still hit this exact gap, so Register is backfilled here
// too for the same reason as VAT.
func backfillWinnerIdentifiers(best *imprintCandidate, cands []imprintCandidate) {
	if best == nil {
		return
	}
	find := func(has func(*imprintCandidate) bool) *imprintCandidate {
		var found *imprintCandidate
		foundScore := 1 << 30
		for i := range cands {
			c := &cands[i]
			if c == best || c.Name == "" || !has(c) {
				continue
			}
			score := candidateSourceRank(c.Source)*10 + candidateConfRank(c.Confidence)
			if score < foundScore {
				foundScore = score
				found = c
			}
		}
		return found
	}
	if best.Register == "" {
		if c := find(func(c *imprintCandidate) bool { return c.Register != "" }); c != nil {
			best.Register = c.Register
		}
	}
	if best.VAT == "" {
		if c := find(func(c *imprintCandidate) bool { return c.VAT != "" }); c != nil {
			best.VAT = c.VAT
			// isVATLikeIdentifierKind (imprint_checksum.go): PartitaIVA/CIF
			// count as VAT-equivalent here too, same reasoning as the
			// caller's own vatValidation lookup just above.
			for _, id := range c.Identifiers {
				if isVATLikeIdentifierKind(id.Kind) && cleanIdentifierValue(id.Kind, id.Value) == c.VAT {
					best.Identifiers = append(best.Identifiers, id)
				}
			}
		}
	}
}

// imprintFieldChecklist returns the ordered field keys the given ruleset
// requires unconditionally. VAT is handled separately by scoreImprint: it is
// never unconditionally mandatory under any of these rulesets (a sole
// proprietor below the VAT threshold legitimately has none), but when a VAT
// number IS present on the page it must actually check out.
func imprintFieldChecklist(ruleset string) []string {
	base := []string{"legal_name", "address", "contact"}
	switch ruleset {
	case RulesetDETMG, RulesetATECGMedienG:
		// Austria's ECG §5 (business-identity disclosure) and Mediengesetz
		// §§24-25 (media-content disclosure) are two independent legal
		// obligations that happen to want almost the same fields (name,
		// address, a responsible person; MedienG does not itself require a
		// VAT/register number). This pass merges them into one checklist
		// rather than modeling two separate completeness scores — simple
		// and correct for what's checked here, but a future pass should
		// split them if AT-specific completeness ever needs to distinguish
		// "ECG-compliant" from "MedienG-compliant" independently.
		return append(append([]string{}, base...), "register", "responsible_person")
	case RulesetFRLCEN, RulesetITDLgs70, RulesetESLSSICE, RulesetNLHandelsreg, RulesetPLUsude:
		// Entity-identification-only obligations (see the Ruleset consts'
		// doc comment above for the legal citations and why
		// responsible_person is deliberately excluded here).
		return append(append([]string{}, base...), "register")
	default:
		return base
	}
}

// rulesetFor selects the applicable ruleset for a country. Defaults to
// eu_baseline for anything not confidently modelled — deliberately:
// applying a stricter national checklist to a country whose imprint law we
// haven't modelled would over-claim requirements that don't actually apply
// there.
func rulesetFor(country string) string {
	switch country {
	case "DE":
		return RulesetDETMG
	case "AT":
		return RulesetATECGMedienG
	case "FR":
		return RulesetFRLCEN
	case "IT":
		return RulesetITDLgs70
	case "ES":
		return RulesetESLSSICE
	case "NL":
		return RulesetNLHandelsreg
	case "PL":
		return RulesetPLUsude
	default:
		return RulesetEUBaseline
	}
}

// scoreImprint fills FieldsFound / FieldsMissing / CompletenessScore against
// im.Ruleset's checklist, plus the VAT-if-present-then-must-validate check.
func scoreImprint(im *Imprint, hasContact bool) {
	checklist := imprintFieldChecklist(im.Ruleset)
	have := map[string]bool{
		"legal_name":         im.LegalName != "",
		"address":            im.Address != "",
		"contact":            hasContact,
		"register":           im.Register != "",
		"responsible_person": im.ResponsiblePerson != "",
	}
	found := make([]string, 0, len(checklist)+1)
	missing := make([]string, 0, len(checklist)+1)
	for _, f := range checklist {
		if have[f] {
			found = append(found, f)
		} else {
			missing = append(missing, f)
		}
	}
	total := len(checklist)
	if im.VAT != "" {
		total++
		if im.VATValidation == string(checksumValid) {
			found = append(found, "vat_valid")
		} else {
			missing = append(missing, "vat_valid")
		}
	}
	im.FieldsFound = found
	im.FieldsMissing = missing
	if total == 0 {
		im.CompletenessScore = 0
		return
	}
	im.CompletenessScore = (len(found) * 100) / total
}

// extractImprintFields turns a verified imprint page's already-fetched body
// into structured, field-level compliance evidence. Called once per request
// from handler_helpers.go, only when merged[DocImprint] exists (i.e. the
// existing footer-link/path-probe discovery plus verify.go's
// docTypeSignalRE[DocImprint] vocabulary gate already confirmed this is a
// real imprint page) — this function does not re-decide presence, only
// extracts fields from what discovery+verification already found.
func extractImprintFields(pageURL, body, hostname string) Imprint {
	im := Imprint{Present: true, URL: pageURL, Ruleset: RulesetEUBaseline}
	if strings.TrimSpace(body) == "" {
		// Discovery/verification confirmed the page is real, but we don't
		// have its body here (e.g. a footer link the verification phase
		// couldn't re-fetch — see hit.Evidence "link_unverified" in
		// handler_helpers.go). Degrade gracefully: Present stays true (the
		// back-compat imprint_present signal is still honest), but no
		// fields can be extracted from text we don't have.
		scoreImprint(&im, false)
		return im
	}

	text := stripTagsLines(body)
	candidates := mergeImprintCandidates(extractImprintCandidates(body, pageURL))
	best := bestImprintCandidate(candidates)
	backfillWinnerIdentifiers(best, candidates)

	var vatValidation string
	if best != nil {
		im.LegalName = best.Name
		im.Suffix = best.Suffix
		im.Address = best.Address
		im.Register = best.Register
		im.VAT = best.VAT
		im.Country = best.Country
		for _, id := range best.Identifiers {
			// isVATLikeIdentifierKind (imprint_jsonld.go) also covers
			// "PartitaIVA"/"CIF" — VAT-equivalent identifiers whose raw
			// regex match carries a label prefix, so cleanIdentifierValue
			// must be applied on BOTH sides before comparing (best.VAT was
			// already cleaned when it was first attached).
			if isVATLikeIdentifierKind(id.Kind) && cleanIdentifierValue(id.Kind, id.Value) == best.VAT {
				vatValidation = id.Validation
			}
			// A single-country identifier kind (PartitaIVA/REA only ever
			// exist in Italy, Hoja only in Spain, ...) is unambiguous
			// evidence of country — unlike a bare legal-form suffix, which
			// can genuinely collide across countries (ALL-CAPS "S.R.L." is
			// both a real Italian stylization AND Romania's native form;
			// imprint_suffix.go's suffixTable is case-sensitive precisely
			// to keep them apart, but an ALL-CAPS JSON-LD name only ever
			// matches Romania's entry). Real evidence: simanova.it's real
			// JSON-LD name field is styled "SIMANOVA S.R.L." (all caps),
			// which won as best candidate with the wrong Country="RO" from
			// that suffix match — but the SAME page's real REA/Partita IVA,
			// backfilled onto this candidate above, are only ever Italian.
			// Overriding here lets ground-truth government-identifier
			// evidence correct a merely-probabilistic suffix guess, without
			// touching suffixTable's matching rules at all (which would
			// risk regressing the OTHER real collisions in that table —
			// e.g. "S.A.S." is both France's and Italy's own native form).
			if cc := singleCountryIdentifierKind(id.Kind); cc != "" {
				im.Country = cc
			}
		}
	}
	if im.VAT != "" && vatValidation == "" {
		vatValidation = string(validateIdentifier("VAT", im.VAT, vatCountryHint(im.VAT)))
	}
	if im.VAT != "" {
		im.VATValidation = vatValidation
	}

	// Country precedence: an explicit VAT-prefix country is the most
	// specific signal available (it's issued BY that country's tax
	// authority), ahead of a suffix-inferred/structured-data country (a
	// bare "GmbH" is DE-or-AT ambiguous — see imprint_suffix.go), ahead of
	// the site's ccTLD, which is itself only a weak final fallback.
	country := ""
	if im.VAT != "" {
		country = normalizeCountry(vatCountryHint(im.VAT))
	}
	if country == "" && im.Country != "" {
		country = im.Country
	}
	if country == "" {
		country = tldCountryOf(hostname)
	}
	im.Country = country
	im.Ruleset = rulesetFor(country)

	im.ResponsiblePerson = extractResponsiblePerson(text)
	if im.ResponsiblePerson == "" && im.Suffix == "" && im.LegalName != "" {
		// Bug 3 fallback: a sole-trader-shaped legal name (no legal-form
		// suffix found at all — see imprint_suffix.go's detectSuffix) has
		// no SEPARATELY-labelled "Geschäftsführer"/"verantwortlich für den
		// Inhalt" line to find, because the trading name itself already IS
		// the natural person's disclosed identity — this is legitimate
		// under Austria's ECG §5 / Mediengesetz §§24-25, confirmed against
		// the real hotelrose.at Impressum, whose JSON-LD name
		// ("... - Franz Holzmann") already names the responsible person.
		// Additional satisfaction path alongside the label-based match
		// above, not a replacement: GmbH/AG-style entities (im.Suffix !=
		// "") still require the distinct managing-director line.
		//
		// Guard: only trust a run that is a proper SUBSET of LegalName —
		// i.e. some lowercase connector elsewhere in the name (like
		// hotelrose.at's "zur") actually isolated it from the rest. When
		// the run consumes the WHOLE LegalName, nothing was isolated at
		// all — every word just happened to be capitalised, which is
		// equally true of an ordinary multi-word TRADING name. Real
		// evidence: simpelbootverhuurutrecht.nl (a real Dutch eenmanszaak)
		// has LegalName "Simpel Bootverhuur Utrecht" — a business name with
		// no person embedded in it anywhere — and titleCaseNameRun happily
		// returned the whole string as a false "responsible person". This
		// is exactly the known limitation this function's own doc comment
		// already flagged as accepted-but-not-yet-hit; this real fixture is
		// the case that hits it.
		if p := titleCaseNameRun(im.LegalName); p != "" && p != im.LegalName {
			im.ResponsiblePerson = p
		}
	}

	if court := courtMention(text); court != "" {
		if im.Register != "" {
			im.Register = im.Register + ", " + court
		} else {
			// A named register court with no register NUMBER at all is
			// still a genuine (if partial) register-authority disclosure —
			// the real hotelrose.at Impressum names "Firmengericht:
			// Landesgericht Innsbruck" but cites no FN number at all (not
			// unusual for a sole trader, who may not be a Firmenbuch entry
			// at all). Treat a court-only mention as satisfying the
			// "register" checklist item rather than silently requiring a
			// number too.
			im.Register = court
		}
	}

	scoreImprint(&im, hasImprintContact(body))
	return im
}
