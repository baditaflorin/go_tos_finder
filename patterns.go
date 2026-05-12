package main

import (
	"regexp"
	"strings"
)

// DocType is a canonical legal-document category identifier.
type DocType string

const (
	DocTermsOfService       DocType = "terms_of_service"
	DocPrivacyPolicy        DocType = "privacy_policy"
	DocCookiePolicy         DocType = "cookie_policy"
	DocAcceptableUse        DocType = "acceptable_use"
	DocDMCA                 DocType = "dmca"
	DocGDPRDPA              DocType = "gdpr_dpa"
	DocSubprocessors        DocType = "subprocessors"
	DocSLA                  DocType = "sla"
	DocRefundPolicy         DocType = "refund_policy"
	DocShippingPolicy       DocType = "shipping_policy"
	DocImprint              DocType = "imprint"
	DocCommunityGuidelines  DocType = "community_guidelines"
	DocCopyrightPolicy      DocType = "copyright_policy"
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

// pattern describes how to recognise a document type, in three ways:
//   - pathPatterns:  case-insensitive path segments (must appear as a path token).
//   - textPatterns:  case-insensitive substrings to match link text.
//   - canonicalPaths: paths probed when no link was discovered.
type pattern struct {
	docType        DocType
	pathPatterns   []*regexp.Regexp
	textRegex      *regexp.Regexp
	canonicalPaths []string
}

// Helper for compiling path-segment regexes. A path matches if any segment
// equals one of the listed tokens (case-insensitive), or the full path ends
// with that token. We compile them as a single anchored regex to keep scanning
// cheap.
func pathRE(tokens ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(tokens))
	for _, t := range tokens {
		// Match `/token`, `/token/`, `/token.html`, `/foo/token`, etc.
		// We deliberately accept the token anywhere as a path segment.
		re := regexp.MustCompile(`(?i)(^|/)` + regexp.QuoteMeta(t) + `(/|\.html?|$)`)
		out = append(out, re)
	}
	return out
}

func textRE(words ...string) *regexp.Regexp {
	// `\b(word1|word2|...)\b` — match a whole-word occurrence.
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
			"terms", "terms-of-service", "terms-of-use", "tos", "conditions",
			"user-agreement", "eula", "agb", "legal-terms", "termsofuse",
			"termsofservice", "terms_of_service", "terms_of_use",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:terms\s+(?:of\s+)?(?:service|use|conditions)?|terms\s*&\s*conditions|conditions\s+of\s+use|user\s+agreement|EULA|nutzungsbedingungen|allgemeine\s+gesch[äa]ftsbedingungen|AGB|conditions\s+d['e]\s*utilisation|conditions\s+g[ée]n[ée]rales)\b`),
		canonicalPaths: []string{
			"/terms", "/terms-of-service", "/terms-of-use", "/tos",
			"/legal/terms", "/conditions", "/user-agreement", "/eula", "/agb",
		},
	},
	{
		docType: DocPrivacyPolicy,
		pathPatterns: pathRE(
			"privacy", "privacy-policy", "privacy-notice", "privacy-statement",
			"datenschutz", "datenschutzerklaerung", "datenschutzerklärung",
			"politique-confidentialite", "politique-de-confidentialite",
			"confidentialite", "informativa-privacy", "politica-de-privacidad",
			"privacy_policy", "privacypolicy",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:privacy(?:\s+(?:policy|notice|statement))?|datenschutz(?:erkl[äa]rung)?|politique\s+de\s+confidentialit[ée]|informativa\s+sulla\s+privacy|pol[íi]tica\s+de\s+privacidad)\b`),
		canonicalPaths: []string{
			"/privacy", "/privacy-policy", "/privacy-notice", "/legal/privacy",
			"/datenschutz", "/politique-confidentialite",
		},
	},
	{
		docType: DocCookiePolicy,
		pathPatterns: pathRE(
			"cookies", "cookie-policy", "cookie-notice", "cookie-statement",
			"use-of-cookies", "cookie-richtlinie", "politique-cookies",
			"cookies-policy", "cookie",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:cookie(?:s)?(?:\s+(?:policy|notice|statement))?|use\s+of\s+cookies|cookie[-\s]?richtlinie|politique\s+(?:des\s+)?cookies)\b`),
		canonicalPaths: []string{
			"/cookies", "/cookie-policy", "/use-of-cookies", "/legal/cookies",
		},
	},
	{
		docType: DocAcceptableUse,
		pathPatterns: pathRE(
			"aup", "acceptable-use", "acceptable-use-policy", "use-policy",
			"abuse-policy",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:acceptable\s+use(?:\s+policy)?|AUP|abuse\s+policy)\b`),
		canonicalPaths: []string{
			"/aup", "/acceptable-use", "/acceptable-use-policy", "/legal/aup",
		},
	},
	{
		docType: DocCommunityGuidelines,
		pathPatterns: pathRE(
			"community-guidelines", "code-of-conduct", "conduct", "community",
			"guidelines",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:community\s+guidelines|code\s+of\s+conduct|house\s+rules)\b`),
		canonicalPaths: []string{
			"/community-guidelines", "/code-of-conduct", "/conduct",
		},
	},
	{
		docType: DocDMCA,
		pathPatterns: pathRE("dmca", "dmca-policy", "dmca-notice"),
		textRegex: regexp.MustCompile(`(?i)\b(?:DMCA(?:\s+(?:policy|notice))?)\b`),
		canonicalPaths: []string{"/dmca", "/dmca-policy", "/legal/dmca"},
	},
	{
		docType: DocCopyrightPolicy,
		pathPatterns: pathRE(
			"copyright", "copyright-policy", "ip-policy",
			"intellectual-property",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:copyright(?:\s+policy)?|intellectual\s+property(?:\s+policy)?|IP\s+policy)\b`),
		canonicalPaths: []string{
			"/copyright", "/copyright-policy", "/legal/copyright", "/ip-policy",
		},
	},
	{
		docType: DocGDPRDPA,
		pathPatterns: pathRE(
			"gdpr", "dpa", "data-processing-agreement",
			"data-processing-addendum", "dpa-policy",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:GDPR(?:\s+(?:policy|notice))?|data\s+processing\s+(?:agreement|addendum)|DPA)\b`),
		canonicalPaths: []string{
			"/gdpr", "/dpa", "/data-processing-agreement",
			"/legal/dpa", "/legal/gdpr",
		},
	},
	{
		docType: DocSubprocessors,
		pathPatterns: pathRE(
			"subprocessors", "sub-processors", "subprocessor-list", "vendors",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:sub[-\s]?processors?|subprocessor\s+list|vendor\s+list)\b`),
		canonicalPaths: []string{
			"/subprocessors", "/sub-processors", "/legal/subprocessors",
			"/vendors",
		},
	},
	{
		docType: DocSLA,
		pathPatterns: pathRE("sla", "service-level-agreement"),
		textRegex: regexp.MustCompile(`(?i)\b(?:SLA|service\s+level\s+agreement)\b`),
		canonicalPaths: []string{
			"/sla", "/service-level-agreement", "/legal/sla",
		},
	},
	{
		docType: DocRefundPolicy,
		pathPatterns: pathRE(
			"refunds", "returns", "refund-policy", "return-policy",
			"refunds-and-returns",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:refund(?:\s+policy)?|return(?:s)?\s+(?:and\s+refunds?\s+)?policy|return\s+policy)\b`),
		canonicalPaths: []string{
			"/refunds", "/returns", "/refund-policy", "/return-policy",
		},
	},
	{
		docType: DocShippingPolicy,
		pathPatterns: pathRE("shipping", "shipping-policy", "delivery"),
		textRegex: regexp.MustCompile(`(?i)\b(?:shipping\s+policy|delivery\s+(?:policy|information))\b`),
		canonicalPaths: []string{
			"/shipping-policy", "/shipping", "/delivery",
		},
	},
	{
		docType: DocImprint,
		pathPatterns: pathRE(
			"imprint", "impressum", "legal-notice", "mentions-legales",
		),
		textRegex: regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|colof[oó]n)\b`),
		canonicalPaths: []string{
			"/imprint", "/impressum", "/legal-notice", "/mentions-legales",
		},
	},
}

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
// surrounding link text, plus a flag for which one matched.
func classifyLink(urlPath, linkText string) (DocType, string) {
	for _, p := range patterns {
		if matchPathOnly(urlPath, p.pathPatterns) {
			return p.docType, "url_path"
		}
	}
	for _, p := range patterns {
		if p.textRegex != nil && p.textRegex.MatchString(linkText) {
			return p.docType, "link_text"
		}
	}
	return "", ""
}

// canonicalPathsFor returns the probe paths for a doc type.
func canonicalPathsFor(t DocType) []string {
	for _, p := range patterns {
		if p.docType == t {
			return p.canonicalPaths
		}
	}
	return nil
}

// allCanonicalProbes returns (docType, path) pairs to probe, capped at maxN
// total to keep request budget bounded.
func allCanonicalProbes(missing []DocType, maxN int) []probeCandidate {
	out := make([]probeCandidate, 0, maxN)
	for _, dt := range missing {
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
