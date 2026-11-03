package main

import (
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Candidate normalisation, applied once to every candidate extractImprintCandidates
// returns (whatever source produced it). Added after a 2026-11 audit that replayed
// the extractor over 720 real imprint pages and had an independent model judge
// each extracted legal_name against the page text (see CHANGELOG 1.9.11): about
// half of the names were not the operator's name verbatim. The failures that
// recurred were mechanical: a field label or a copyright line glued to the name
// ("Website Owner: Manfredo Music SL", "Alle Rechte vorbehalten VIER PFOTEN - Stiftung"),
// a connective phrase before it ("Ingenieros.es pertenece a ... SL"), prose after
// the legal form ("SA CONFORT DECOR au capital de 2 903 000 €"), an unclosed
// bracket in a Japanese company name, a field label or a legal-form description
// taken as the name ("Company ID: 04788290", "Private Limited Company (Ltd)"), and the
// hosting provider or web agency named in a credit line ("Hébergeur : OVH SAS").
// Each rule below is anchored on a real page of that audit.

// leadingBoilerplate are lower-case prefixes that are never part of a legal name.
var leadingBoilerplate = []string{
	"tm &", "©", "(c)", "copyright", "all rights reserved", "alle rechte vorbehalten",
	"alle rettigheter", "tous droits réservés", "todos los derechos reservados",
	"all rights", "@",
}

var leadingYearRE = regexp.MustCompile(`^(?:19|20)\d\d(?:\s*[-–—]\s*(?:19|20)?\d\d)?\b`)

// connectivePhrases introduce the operator's name inside a sentence; the name is what follows.
var connectivePhrases = []string{
	" est édité par ", " est edité par ", " édité par ", " pertenece a ", " es propiedad de ",
	" é propriedade de ", " is een uitgave van ", " is operated by ", " is published by ",
	" wird betrieben von ", " ist ein angebot der ", " ist ein angebot von ",
	" publicado por ", " publicada por ", " frente a ", " propiedad de ", " tochtergesellschaft ",
}

// leadingConnectives are the same phrases at the start of the string.
var leadingConnectives = []string{
	"propiedad de ", "propriedade de ", "eigentümer: ", "su denominación social es: ",
	"the exploitation rights belong to: ", "está publicado por ", "esta publicado por ",
	"zweigniederlassung der ", "niederlassung der ", "unternehmensinformationen der ",
	"la société française ", "la société ", "l'entreprise ", "la empresa ", "la sociedad ",
}

// leadingFunctionWords are lower-case articles and prepositions: a candidate that starts with
// one of them has read a word too far to the left ("der Selfio SE", "by SmartTube Co., Ltd.").
var leadingFunctionWords = []string{"der ", "die ", "das ", "den ", "dem ", "the ", "by ", "von ", "la ", "le ", "el ", "los ", "las ", "het ", "een "}

// leadingPageLabels are page or section titles glued to the name ("Aviso legal - Datan S.R.L.",
// "Legal NoticeVisualsofjulius Oy"): matched case-insensitively, only before a non-lowercase character.
var leadingPageLabels = []string{
	"aviso legal", "legal notice", "avis légal", "mentions légales", "impressum", "disclaimer",
	"privacy policy", "politica de privacidad", "política de privacidad", "datenschutzerklärung",
	"terms and conditions", "condiciones generales", "datapolitik handelsbetingelser",
}

// trailingProseMarkers start text that describes the company, not names it.
var trailingProseMarkers = []string{
	" au capital", " capital social", " mit sitz ", " mit dem sitz ", " with registered office",
	" registered in ", " con domicilio ", " con sede ", " inscrite au ", " immatriculée ",
	" is a registered ", " est une société ",
}

// genericStemWords: a "name" made only of these is a heading or a legal-form description.
var genericStemWords = map[string]bool{
	"company": true, "information": true, "links": true, "profile": true, "overview": true,
	"history": true, "about": true, "us": true, "group": true, "corporate": true, "private": true,
	"limited": true, "public": true, "by": true, "shares": true, "besloten": true, "vennootschap": true,
	"name": true, "details": true, "legal": true, "notice": true, "imprint": true, "id": true,
	"number": true, "license": true, "licence": true, "registration": true, "registered": true,
	"the": true, "our": true, "of": true, "and": true, "&": true, "(ltd)": true, "(bv)": true,
}

// serviceProviders are hosting, platform and web-service companies that imprints name as
// the host or a third-party service, almost never as the site's operator.
var serviceProviders = []string{
	"webflow", "wix.com", "squarespace", "ovh", "automattic", "amazon web services", "cloudflare",
	"godaddy", "ionos", "1&1", "hetzner", "strato", "shopify", "google llc", "google inc",
	"google ireland", "facebook", "meta platforms", "microsoft", "hostinger", "bluehost",
	"siteground", "wordpress", "twitter", "linkedin ireland", "apple inc", "paypal",
}

// creditContextRE matches the words before a name on its own line that mark it as a credit
// (host, web agency, photographer), not the site's operator.
var creditContextRE = regexp.MustCompile(`(?i)(hébergeur|hebergeur|hébergé|hebergé|hébergement|hosting|hosted (?:by|at|on)|gehostet|webhosting|` +
	`webdesign|web design|web development|website by|site by|site realisation|created by|designed by|design by|` +
	`développé par|developpé par|réalisé par|réalisation du site|conception du site|gestaltung|umsetzung|` +
	`programmierung|realisiert (?:von|durch)|entwickelt von|powered by|diseño web|desarrollado por|` +
	`alojado|alojamiento|ontwikkeld door|gehost|fotos?:|photos?:|bildnachweis|image credits?|crédits? photos?)\W*$`)

// normalizeCandidates cleans every candidate and drops those that are not a legal name.
func normalizeCandidates(in []imprintCandidate, body, pageURL string) []imprintCandidate {
	if len(in) == 0 {
		return in
	}
	host := ""
	if u, err := url.Parse(pageURL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	var text string // lazily built: only the text-sourced candidates need the page context
	out := in[:0:0]
	for _, c := range in {
		switch c.Source {
		case "imprint_text", "imprint_label", "copyright_footer", "json_ld", "hcard", "og_site_name":
		default:
			out = append(out, c)
			continue
		}
		name, suffix, ok := normalizeLegalName(c.Name, c.Suffix)
		if !ok {
			if c.Register != "" || c.VAT != "" || len(c.Identifiers) > 0 {
				c.IdentifierOnly = true
				out = append(out, c)
			}
			continue
		}
		if isServiceProvider(name, host) {
			continue
		}
		if c.Source == "imprint_text" || c.Source == "imprint_label" {
			if text == "" {
				text = stripTagsLines(body)
			}
			if inCreditContext(text, name) {
				continue
			}
		}
		c.Name, c.Suffix = name, suffix
		out = append(out, c)
	}
	return out
}

// normalizeLegalName returns the cleaned name, its (possibly kept) suffix, and false when the
// string is not a legal name at all. A name without a suffix is only cleaned, never judged by it.
func normalizeLegalName(name, suffix string) (string, string, bool) {
	s := strings.TrimSpace(normalizeSpaces(name))
	if s == "" {
		return "", suffix, false
	}
	s = stripLeadingBoilerplate(s)
	for _, p := range leadingConnectives {
		if len(s) > len(p) && strings.EqualFold(s[:len(p)], p) {
			s = strings.TrimSpace(s[len(p):])
		}
	}
	if i := strings.Index(s, "©"); i > 0 {
		// "Vlasnik autorskih prava © avaz-roto press d.o.o.": the name follows the copyright sign.
		s = stripLeadingBoilerplate(s[i:])
	}
	for changed := true; changed; {
		changed = false
		for _, w := range leadingFunctionWords {
			if len(s) > len(w) && strings.HasPrefix(s, w) {
				s, changed = strings.TrimSpace(s[len(w):]), true
			}
		}
		low := strings.ToLower(s)
		for _, l := range leadingPageLabels {
			if len(low) > len(l) && strings.HasPrefix(low, l) && len(low) == len(s) {
				r, _ := utf8.DecodeRuneInString(s[len(l):])
				if !unicode.IsLower(r) {
					s, changed = strings.TrimLeft(s[len(l):], " \t:-–—"), true
				}
			}
		}
	}
	{
		// Offsets from the lower-cased copy are only valid when lower-casing kept the byte length.
		low := strings.ToLower(s)
		if len(low) == len(s) {
			for _, p := range connectivePhrases {
				if i := strings.Index(low, p); i > 0 {
					if rest := strings.TrimSpace(s[i+len(p):]); rest != "" && hasLegalSuffix(rest, suffix) {
						s = rest
						break
					}
				}
			}
			low = strings.ToLower(s)
			for _, p := range trailingProseMarkers {
				if i := strings.Index(low, p); i > 3 {
					s = strings.TrimSpace(s[:i])
					break
				}
			}
		}
	}
	// "Label: Name": the part after the last colon is the name when it ends in the legal form.
	if i, w := lastColon(s); i > 0 {
		before, after := strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+w:])
		if len(strings.Fields(before)) <= 6 {
			if after == "" || !hasLetter(after) {
				return "", suffix, false
			}
			s = after
		}
	}
	s = stripLeadingBoilerplate(s)
	s = cutAfterSuffixSeparator(s, suffix)
	s = closeOrCutBrackets(s)
	s = strings.TrimSpace(strings.Trim(s, " \t,;:-–—|·"))
	if s == "" {
		return "", suffix, false
	}
	if suffix != "" && !strings.Contains(s, suffix) {
		return "", suffix, false
	}
	stem := strings.TrimSpace(strings.TrimSuffix(s, suffix))
	if stem == "" && suffix != "" {
		return "", suffix, false
	}
	if isGenericStem(stem) || digitsOutnumberLetters(stem) {
		return "", suffix, false
	}
	return s, suffix, true
}

func stripLeadingBoilerplate(s string) string {
	for {
		t := strings.TrimLeft(s, " \t©®™@&.,:;-–—​️")
		low := strings.ToLower(t)
		cut := 0
		for _, p := range leadingBoilerplate {
			if strings.HasPrefix(low, p) && len(t) > len(p) {
				cut = len(p)
				break
			}
		}
		if cut == 0 {
			if m := leadingYearRE.FindString(t); m != "" && len(t) > len(m) {
				cut = len(m)
			}
		}
		if cut == 0 {
			return strings.TrimSpace(t)
		}
		s = t[cut:]
	}
}

func lastColon(s string) (int, int) {
	a, b := strings.LastIndex(s, ":"), strings.LastIndex(s, "：")
	if b > a {
		return b, len("：")
	}
	return a, 1
}

func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

func hasLegalSuffix(s, suffix string) bool {
	if suffix == "" {
		return true
	}
	return strings.HasSuffix(strings.TrimRight(s, " .,"), strings.TrimRight(suffix, " .,"))
}

// cutAfterSuffixSeparator drops a tagline after the legal form: "IRTECH GmbH – App-Entwicklung".
func cutAfterSuffixSeparator(s, suffix string) string {
	if suffix == "" {
		return s
	}
	i := strings.LastIndex(s, suffix)
	if i < 0 {
		return s
	}
	tail := s[i+len(suffix):]
	t := strings.TrimLeft(tail, " ")
	if len(tail) > len(t) && (strings.HasPrefix(t, "– ") || strings.HasPrefix(t, "— ") || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "| ")) {
		return strings.TrimSpace(s[:i+len(suffix)])
	}
	return s
}

// closeOrCutBrackets removes an unclosed bracket and what follows it: the source cut the
// parenthetical alias ("ラッコ株式会社（Rakko, Inc.") at the legal form.
func closeOrCutBrackets(s string) string {
	for _, pair := range [][2]rune{{'(', ')'}, {'（', '）'}, {'[', ']'}, {'［', '］'}} {
		open, closeR := strings.Count(s, string(pair[0])), strings.Count(s, string(pair[1]))
		if open > closeR {
			if i := strings.LastIndex(s, string(pair[0])); i > 0 {
				s = strings.TrimSpace(s[:i])
			}
		}
	}
	return s
}

func isGenericStem(stem string) bool {
	fields := strings.Fields(strings.ToLower(stem))
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		f = strings.Trim(f, ".,:;")
		if f != "" && !genericStemWords[f] {
			return false
		}
	}
	return true
}

func digitsOutnumberLetters(stem string) bool {
	letters, digits := 0, 0
	for _, r := range stem {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
			digits++
		}
	}
	return digits >= 5 && digits > letters
}

func isServiceProvider(name, host string) bool {
	low := strings.ToLower(name)
	for _, p := range serviceProviders {
		if strings.Contains(low, p) && !strings.Contains(host, strings.Fields(p)[0]) {
			return true
		}
	}
	return false
}

// inCreditContext reports whether name's first occurrence on its line follows host / design /
// photo-credit wording. Only the same line counts: a heading on the line above is another notice.
func inCreditContext(text, name string) bool {
	i := strings.Index(text, name)
	if i < 0 {
		return false
	}
	lineStart := strings.LastIndex(text[:i], "\n") + 1
	from := i - 100
	if from < lineStart {
		from = lineStart
	}
	return creditContextRE.MatchString(text[from:i])
}
