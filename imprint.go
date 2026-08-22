package main

import (
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
const (
	RulesetEUBaseline   = "eu_baseline"
	RulesetDETMG        = "de_tmg"
	RulesetATECGMedienG = "at_ecg_medieng"
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

// hasImprintContact reports whether the page exposes a functional contact
// channel (email or a clearly-labelled phone number) — the eu_baseline
// "contact" checklist item.
func hasImprintContact(body string) bool {
	return imprintEmailRE.MatchString(body) || imprintPhoneRE.MatchString(body)
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
	case "hcard":
		return 2
	case "copyright_footer":
		return 3
	case "og_site_name":
		return 4
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

// bestImprintCandidate picks the highest-priority named candidate.
func bestImprintCandidate(cands []imprintCandidate) *imprintCandidate {
	var best *imprintCandidate
	bestScore := 1 << 30
	for i := range cands {
		c := &cands[i]
		if c.Name == "" {
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
	default:
		return base
	}
}

// rulesetFor selects the applicable ruleset for a country. Defaults to
// eu_baseline for anything not confidently DE/AT — deliberately: applying
// the stricter DE/AT checklist to a country whose imprint law we haven't
// modelled would over-claim requirements that don't actually apply there.
func rulesetFor(country string) string {
	switch country {
	case "DE":
		return RulesetDETMG
	case "AT":
		return RulesetATECGMedienG
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

	var vatValidation string
	if best != nil {
		im.LegalName = best.Name
		im.Suffix = best.Suffix
		im.Address = best.Address
		im.Register = best.Register
		im.VAT = best.VAT
		im.Country = best.Country
		for _, id := range best.Identifiers {
			if id.Kind == "VAT" && id.Value == best.VAT {
				vatValidation = id.Validation
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

	if im.Register != "" {
		if court := courtMention(text); court != "" {
			im.Register = im.Register + ", " + court
		}
	}

	scoreImprint(&im, hasImprintContact(body))
	return im
}
