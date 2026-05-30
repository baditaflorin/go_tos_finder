package main

import (
	"regexp"
	"strings"
)

// DocType is a canonical legal-document category identifier.
type DocType string

const (
	DocTermsOfService      DocType = "terms_of_service"
	DocPrivacyPolicy       DocType = "privacy_policy"
	DocCookiePolicy        DocType = "cookie_policy"
	DocAcceptableUse       DocType = "acceptable_use"
	DocDMCA                DocType = "dmca"
	DocGDPRDPA             DocType = "gdpr_dpa"
	DocSubprocessors       DocType = "subprocessors"
	DocSLA                 DocType = "sla"
	DocRefundPolicy        DocType = "refund_policy"
	DocShippingPolicy      DocType = "shipping_policy"
	DocImprint             DocType = "imprint"
	DocCommunityGuidelines DocType = "community_guidelines"
	DocCopyrightPolicy     DocType = "copyright_policy"
	DocTrustCenter         DocType = "trust_center"
	DocLegalHub            DocType = "legal_hub"
	DocDisclaimer          DocType = "disclaimer"
)

// documentTypeOrder controls output ordering and the "expected" set used for
// the documents_missing computation. The most important / most universal
// document types come first.
var documentTypeOrder = []DocType{
	DocTermsOfService,
	DocPrivacyPolicy,
	DocCookiePolicy,
	DocAcceptableUse,
	DocCommunityGuidelines,
	DocDMCA,
	DocCopyrightPolicy,
	DocGDPRDPA,
	DocSubprocessors,
	DocSLA,
	DocRefundPolicy,
	DocShippingPolicy,
	DocImprint,
	DocDisclaimer,
	DocTrustCenter,
	DocLegalHub,
}

// expectedDocTypes are the document types considered "expected" for a
// reasonably comprehensive site. Niche types like SLA / sub-processors /
// refunds are excluded so e-commerce-only docs don't penalise SaaS, etc.
var expectedDocTypes = []DocType{
	DocTermsOfService,
	DocPrivacyPolicy,
	DocCookiePolicy,
	DocAcceptableUse,
	DocDMCA,
}

// hubDocTypes are "directory" pages that point at the real legal docs rather
// than being a single policy themselves. They are never probed canonically
// (we only follow them when they appear as an explicit link) and are excluded
// from the soft-404-prone probe budget. They DO count as evidence the site
// has a legal surface.
var hubDocTypes = map[DocType]bool{
	DocTrustCenter: true,
	DocLegalHub:    true,
}

// pattern describes how to recognise a document type, in three ways:
//   - pathPatterns:  case-insensitive path segments (must appear as a path token).
//   - textRegex:     case-insensitive whole-word/phrase match against link text.
//   - canonicalPaths: paths probed when no link was discovered.
//   - titleRegex:    used by content verification — a fetched page whose
//     <title>/<h1> matches is strong confirmation of the doc type.
type pattern struct {
	docType        DocType
	pathPatterns   []*regexp.Regexp
	textRegex      *regexp.Regexp
	titleRegex     *regexp.Regexp
	canonicalPaths []string
}

// pathRE compiles a set of path-segment regexes. A path matches if a listed
// token appears as a path segment (case-insensitive): `/token`, `/token/`,
// `/token.html`, `/foo/token`, `/de/token`, etc. Locale-prefixed paths
// (`/en/privacy`, `/de/datenschutz`) match naturally because the token is a
// segment.
func pathRE(tokens ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tokens))
	for _, t := range tokens {
		re := regexp.MustCompile(`(?i)(^|/)` + regexp.QuoteMeta(t) + `(/|\.html?|\.php|\.aspx?|$)`)
		out = append(out, re)
	}
	return out
}

// pathREContains compiles regexes that match if the token appears anywhere
// inside a path segment (not only as a whole segment). Use sparingly for
// tokens distinctive enough that a substring match is still high-precision —
// "cookie"/"cookies" qualify (almost no non-cookie page embeds the word), but
// "terms" or "legal" do NOT (they collide with thermometer, paralegal, …).
func pathREContains(tokens ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tokens))
	for _, t := range tokens {
		re := regexp.MustCompile(`(?i)(^|/)[^/]*` + regexp.QuoteMeta(t) + `[^/]*(/|$)`)
		out = append(out, re)
	}
	return out
}

func textRE(words ...string) *regexp.Regexp {
	escaped := make([]string, len(words))
	for i, w := range words {
		escaped[i] = regexp.QuoteMeta(w)
	}
	return regexp.MustCompile(`(?i)\b(?:` + strings.Join(escaped, "|") + `)\b`)
}

// patterns is the master table consulted by the discovery pass.
var patterns = []pattern{
	{
		docType: DocTermsOfService,
		pathPatterns: pathRE(
			"terms", "terms-of-service", "terms-of-use", "terms-and-conditions",
			"tos", "tou", "conditions", "conditions-of-use", "user-agreement",
			"eula", "agb", "legal-terms", "termsofuse", "termsofservice",
			"terms_of_service", "terms_of_use", "terms-conditions", "tnc",
			// multilingual
			"nutzungsbedingungen", "geschaeftsbedingungen", "cgu", "cgv",
			"conditions-generales", "conditions-utilisation", "termini",
			"termini-di-servizio", "condiciones", "terminos", "termos",
			"voorwaarden", "algemene-voorwaarden", "regulamin", "warunki",
			"vilkar", "anvandarvillkor", "kayttoehdot",
		),
		textRegex:  regexp.MustCompile(`(?i)\b(?:terms\s+(?:of\s+)?(?:service|use|conditions)?|terms\s*&\s*conditions|terms\s+and\s+conditions|conditions\s+of\s+use|user\s+agreement|EULA|nutzungsbedingungen|allgemeine\s+gesch[äa]ftsbedingungen|AGB|conditions\s+d['e]\s*utilisation|conditions\s+g[ée]n[ée]rales|termini\s+di\s+servizio|t[ée]rminos\s+(?:y\s+condiciones|de\s+servicio)|condiciones\s+(?:de\s+uso|del\s+servicio)|termos\s+de\s+(?:uso|servi[çc]o)|gebruiksvoorwaarden|algemene\s+voorwaarden|regulamin)\b`),
		titleRegex: regexp.MustCompile(`(?i)\b(?:terms\s+(?:of\s+)?(?:service|use|conditions)|terms\s+and\s+conditions|terms\s*&\s*conditions|user\s+agreement|end[-\s]user\s+license|EULA|nutzungsbedingungen|gesch[äa]ftsbedingungen|conditions\s+g[ée]n[ée]rales|termini\s+di\s+servizio|t[ée]rminos|condiciones|termos\s+de)\b`),
		canonicalPaths: []string{
			"/terms", "/terms-of-service", "/terms-of-use", "/terms-and-conditions",
			"/tos", "/legal/terms", "/conditions", "/user-agreement", "/eula", "/agb",
		},
	},
	{
		docType: DocPrivacyPolicy,
		pathPatterns: pathRE(
			"privacy", "privacy-policy", "privacy-notice", "privacy-statement",
			"privacy-center", "data-privacy", "privacypolicy", "privacy_policy",
			"datenschutz", "datenschutzerklaerung", "datenschutzerklärung",
			"datenschutzhinweise", "politique-confidentialite",
			"politique-de-confidentialite", "confidentialite",
			"informativa-privacy", "informativa-sulla-privacy", "privacy-italiano",
			"politica-de-privacidad", "politica-privacidade", "privacidad",
			"privacidade", "privacyverklaring", "privacybeleid",
			"polityka-prywatnosci", "prywatnosc", "personvern", "integritetspolicy",
			"tietosuoja",
		),
		textRegex:  regexp.MustCompile(`(?i)\b(?:privacy(?:\s+(?:policy|notice|statement|choices|center|centre))?|data\s+privacy|datenschutz(?:erkl[äa]rung|hinweise)?|politique\s+de\s+confidentialit[ée]|informativa\s+(?:sulla\s+)?privacy|pol[íi]tica\s+de\s+privacidad|pol[íi]tica\s+de\s+privacidade|privacyverklaring|privacybeleid|polityka\s+prywatno[śs]ci|integritetspolicy)\b`),
		titleRegex: regexp.MustCompile(`(?i)\b(?:privacy\s+(?:policy|notice|statement)|datenschutz|politique\s+de\s+confidentialit[ée]|informativa\s+(?:sulla\s+)?privacy|pol[íi]tica\s+de\s+privacid|privacyverklaring|polityka\s+prywatno[śs]ci)\b`),
		canonicalPaths: []string{
			"/privacy", "/privacy-policy", "/privacy-notice", "/legal/privacy",
			"/datenschutz", "/politique-confidentialite",
		},
	},
	{
		docType: DocCookiePolicy,
		pathPatterns: append(
			// "cookie"/"cookies" anywhere in a segment — distinctive enough that
			// a substring match stays high-precision (catches /cookie-settings,
			// /blog/cookies-are-tasty, /cookiebeleid, …).
			pathREContains("cookie"),
			pathRE(
				"politique-cookies", "politica-cookies", "politica-de-cookies",
				"informativa-cookie",
			)...,
		),
		textRegex:  regexp.MustCompile(`(?i)\b(?:cookie(?:s)?(?:\s+(?:policy|notice|statement|settings|preferences))?|use\s+of\s+cookies|cookie[-\s]?richtlinie|politique\s+(?:des\s+)?cookies|pol[íi]tica\s+de\s+cookies|cookiebeleid)\b`),
		titleRegex: regexp.MustCompile(`(?i)\b(?:cookie\s+(?:policy|notice|statement)|cookie[-\s]?richtlinie|politique\s+(?:des\s+)?cookies|pol[íi]tica\s+de\s+cookies)\b`),
		canonicalPaths: []string{
			"/cookies", "/cookie-policy", "/use-of-cookies", "/legal/cookies",
		},
	},
	{
		docType: DocAcceptableUse,
		pathPatterns: pathRE(
			"aup", "acceptable-use", "acceptable-use-policy", "use-policy",
			"abuse-policy", "fair-use", "prohibited-uses", "restricted-uses",
			"restricted-businesses", "prohibited-businesses",
		),
		textRegex:      regexp.MustCompile(`(?i)\b(?:acceptable\s+use(?:\s+policy)?|AUP|abuse\s+policy|fair\s+use\s+policy|prohibited\s+uses?|restricted\s+businesses)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:acceptable\s+use|fair\s+use\s+policy|prohibited\s+(?:uses?|businesses)|abuse\s+policy)\b`),
		canonicalPaths: []string{"/aup", "/acceptable-use", "/acceptable-use-policy", "/legal/aup"},
	},
	{
		docType: DocCommunityGuidelines,
		pathPatterns: pathRE(
			"community-guidelines", "community-guidelines-policy",
			"code-of-conduct", "conduct", "community-standards",
			"participation-guidelines", "house-rules",
		),
		textRegex:      regexp.MustCompile(`(?i)\b(?:community\s+(?:guidelines|standards)|code\s+of\s+conduct|participation\s+guidelines|house\s+rules)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:community\s+(?:guidelines|standards)|code\s+of\s+conduct|participation\s+guidelines)\b`),
		canonicalPaths: []string{"/community-guidelines", "/code-of-conduct"},
	},
	{
		docType:        DocDMCA,
		pathPatterns:   pathRE("dmca", "dmca-policy", "dmca-notice", "copyright-infringement", "takedown", "takedown-policy"),
		textRegex:      regexp.MustCompile(`(?i)\b(?:DMCA(?:\s+(?:policy|notice))?|takedown\s+(?:policy|notice|request)|copyright\s+infringement(?:\s+(?:policy|notice))?)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:DMCA|takedown|copyright\s+infringement)\b`),
		canonicalPaths: []string{"/dmca", "/dmca-policy", "/legal/dmca"},
	},
	{
		docType: DocCopyrightPolicy,
		pathPatterns: pathRE(
			"copyright", "copyright-policy", "ip-policy",
			"intellectual-property", "intellectual-property-policy",
			"trademark", "trademark-policy",
		),
		textRegex:      regexp.MustCompile(`(?i)\b(?:copyright(?:\s+policy)?|intellectual\s+property(?:\s+policy)?|IP\s+policy|trademark(?:\s+policy)?)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:copyright\s+policy|intellectual\s+property|trademark\s+policy)\b`),
		canonicalPaths: []string{"/copyright", "/copyright-policy", "/legal/copyright", "/ip-policy"},
	},
	{
		docType: DocGDPRDPA,
		pathPatterns: pathRE(
			"gdpr", "dpa", "data-processing-agreement",
			"data-processing-addendum", "dpa-policy", "data-protection",
		),
		textRegex:      regexp.MustCompile(`(?i)\b(?:GDPR(?:\s+(?:policy|notice|compliance))?|data\s+processing\s+(?:agreement|addendum)|data\s+protection\s+(?:agreement|addendum)|DPA)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:GDPR|data\s+processing\s+(?:agreement|addendum)|data\s+protection\s+addendum|DPA)\b`),
		canonicalPaths: []string{"/gdpr", "/dpa", "/data-processing-agreement", "/legal/dpa", "/legal/gdpr"},
	},
	{
		docType:        DocSubprocessors,
		pathPatterns:   pathRE("subprocessors", "sub-processors", "subprocessor-list", "sub-processor-list", "vendors", "vendor-list"),
		textRegex:      regexp.MustCompile(`(?i)\b(?:sub[-\s]?processors?|subprocessor\s+list|vendor\s+list)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:sub[-\s]?processors?|subprocessor\s+list)\b`),
		canonicalPaths: []string{"/subprocessors", "/sub-processors", "/legal/subprocessors"},
	},
	{
		docType:        DocSLA,
		pathPatterns:   pathRE("sla", "service-level-agreement", "service-level"),
		textRegex:      regexp.MustCompile(`(?i)\b(?:SLA|service[-\s]level\s+agreement)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:SLA|service[-\s]level\s+agreement)\b`),
		canonicalPaths: []string{"/sla", "/service-level-agreement", "/legal/sla"},
	},
	{
		docType:        DocRefundPolicy,
		pathPatterns:   pathRE("refunds", "returns", "refund-policy", "return-policy", "refunds-and-returns", "returns-policy", "cancellation-policy"),
		textRegex:      regexp.MustCompile(`(?i)\b(?:refund(?:\s+policy)?|returns?\s+(?:and\s+refunds?\s+)?policy|return\s+policy|cancellation\s+policy)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:refund\s+policy|returns?\s+policy|cancellation\s+policy)\b`),
		canonicalPaths: []string{"/refunds", "/returns", "/refund-policy", "/return-policy"},
	},
	{
		docType:        DocShippingPolicy,
		pathPatterns:   pathRE("shipping", "shipping-policy", "delivery", "delivery-policy", "shipping-and-delivery"),
		textRegex:      regexp.MustCompile(`(?i)\b(?:shipping\s+policy|delivery\s+(?:policy|information)|shipping\s+(?:and|&)\s+delivery)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:shipping\s+policy|delivery\s+policy)\b`),
		canonicalPaths: []string{"/shipping-policy", "/shipping", "/delivery"},
	},
	{
		docType: DocImprint,
		pathPatterns: pathRE(
			"imprint", "impressum", "legal-notice", "legal-notices",
			"mentions-legales", "note-legali", "aviso-legal", "colofon",
		),
		textRegex:      regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|note\s+legali|aviso\s+legal|colof[oó]n)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|aviso\s+legal)\b`),
		canonicalPaths: []string{"/imprint", "/impressum", "/legal-notice", "/mentions-legales"},
	},
	{
		docType:        DocDisclaimer,
		pathPatterns:   pathRE("disclaimer", "disclaimers", "haftungsausschluss"),
		textRegex:      regexp.MustCompile(`(?i)\b(?:disclaimer|haftungsausschluss)\b`),
		titleRegex:     regexp.MustCompile(`(?i)\b(?:disclaimer|haftungsausschluss)\b`),
		canonicalPaths: []string{"/disclaimer"},
	},
	{
		docType: DocTrustCenter,
		pathPatterns: pathRE(
			"trust-center", "trust-hub", "trust", "trustcenter",
			"trust-and-safety", "trust-safety", "security-center",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:trust\s+(?:center|centre|hub)|trust\s*(?:&|and)\s*safety|security\s+center|security\s+centre)\b`),
		// titleRegex left nil — trust hubs are landing pages with varied titles;
		// link-text evidence is sufficient and we don't canonical-probe them.
		canonicalPaths: nil,
	},
	{
		docType: DocLegalHub,
		pathPatterns: pathRE(
			"legal", "policies", "legal-center", "legal-hub", "rechtliches",
		),
		textRegex:      regexp.MustCompile(`(?i)^\s*(?:legal|policies|rechtliches|legal\s+(?:center|centre|hub|information)|legal\s+&\s+compliance)\s*$`),
		canonicalPaths: nil,
	},
}

// patternByType is a lookup built once at init.
var patternByType = func() map[DocType]pattern {
	m := make(map[DocType]pattern, len(patterns))
	for _, p := range patterns {
		m[p.docType] = p
	}
	return m
}()

// matchPathOnly tests whether `urlPath` (case-folded) appears to match any of
// `regs`.
func matchPathOnly(urlPath string, regs []*regexp.Regexp) bool {
	for _, re := range regs {
		if re.MatchString(urlPath) {
			return true
		}
	}
	return false
}

// classifyLink returns the doc type matched by either the URL path or the
// surrounding link text, plus a flag for which one matched. Path matches are
// tried first (stronger signal) and the hub types (legal/policies) are tried
// last so a `/legal/privacy` link classifies as privacy, not as the hub.
func classifyLink(urlPath, linkText string) (DocType, string) {
	// Pass 1: specific (non-hub) path matches.
	for _, p := range patterns {
		if hubDocTypes[p.docType] {
			continue
		}
		if matchPathOnly(urlPath, p.pathPatterns) {
			return p.docType, "url_path"
		}
	}
	// Pass 2: specific (non-hub) link-text matches.
	for _, p := range patterns {
		if hubDocTypes[p.docType] {
			continue
		}
		if p.textRegex != nil && p.textRegex.MatchString(linkText) {
			return p.docType, "link_text"
		}
	}
	// Pass 3: hub types last (legal / policies / trust). For the legal hub
	// the path must END in /legal or /policies (a bare hub), not merely
	// contain it, so /legal/privacy doesn't get swallowed here.
	for _, p := range patterns {
		if !hubDocTypes[p.docType] {
			continue
		}
		if matchPathOnly(urlPath, p.pathPatterns) {
			if p.docType == DocLegalHub && !isBareHubPath(urlPath) {
				continue
			}
			return p.docType, "url_path"
		}
		if p.textRegex != nil && p.textRegex.MatchString(strings.TrimSpace(linkText)) {
			return p.docType, "link_text"
		}
	}
	return "", ""
}

// isBareHubPath reports whether the path is a bare /legal or /policies hub
// (optionally locale-prefixed and trailing-slashed) rather than a sub-page
// like /legal/privacy.
func isBareHubPath(urlPath string) bool {
	p := strings.Trim(strings.ToLower(urlPath), "/")
	segs := strings.Split(p, "/")
	if len(segs) == 0 {
		return false
	}
	last := segs[len(segs)-1]
	switch last {
	case "legal", "policies", "legal-center", "legal-hub", "rechtliches":
		return true
	}
	return false
}

// canonicalPathsFor returns the probe paths for a doc type.
func canonicalPathsFor(t DocType) []string {
	if p, ok := patternByType[t]; ok {
		return p.canonicalPaths
	}
	return nil
}

// titleRegexFor returns the title/h1 confirmation regex for a doc type, if any.
func titleRegexFor(t DocType) *regexp.Regexp {
	if p, ok := patternByType[t]; ok {
		return p.titleRegex
	}
	return nil
}

// allCanonicalProbes returns (docType, path) pairs to probe, capped at maxN
// total to keep request budget bounded. Hub types are never canonical-probed.
func allCanonicalProbes(missing []DocType, maxN int) []probeCandidate {
	out := make([]probeCandidate, 0, maxN)
	for _, dt := range missing {
		if hubDocTypes[dt] {
			continue
		}
		for _, p := range canonicalPathsFor(dt) {
			if len(out) >= maxN {
				return out
			}
			out = append(out, probeCandidate{DocType: dt, Path: p})
		}
	}
	return out
}

type probeCandidate struct {
	DocType DocType
	Path    string
}
