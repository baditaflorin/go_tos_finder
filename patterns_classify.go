package main

import (
	"strings"
)

// classifyLink returns the doc type matched by either the URL path or the
// surrounding link text, plus a flag for which one matched. Path matches are
// tried first (stronger signal) and the hub types (legal/policies) are tried
// last so a `/legal/privacy` link classifies as privacy, not as the hub.
func classifyLink(urlPath, linkText string) (DocType, string) {
	dt, how, _ := classifyLinkEvidence(urlPath, linkText)
	return dt, how
}

// classifyLinkEvidence is the richer form of classifyLink: it returns the
// matched doc type, which signal matched, and a *location-confidence floor*
// derived purely from the link itself (independent of any body fetch).
//
// The floor exists because in production the Phase-3 body-verification GET of a
// discovered footer link frequently times out behind the fleet fetch proxy,
// collapsing every footer link to "low". But a link is itself location
// evidence: when BOTH the URL path token AND the anchor text independently name
// the same canonical doc type (e.g. href=/legal/privacy text="Privacy Policy"),
// that corroboration is strong location evidence and should hold a `medium`
// floor even when the target body can't be fetched. A single-signal match holds
// only a `low` floor (a stray "/terms" path or a stray "Privacy" word can be a
// false friend on its own).
func classifyLinkEvidence(urlPath, linkText string) (DocType, string, string) {
	linkText = strings.TrimSpace(linkText)
	// Pass 1: specific (non-hub) path matches. Check whether the SAME doc
	// type is also corroborated by the anchor text → medium floor.
	for _, p := range patterns {
		if hubDocTypes[p.docType] {
			continue
		}
		if matchPathOnly(urlPath, p.pathPatterns) {
			floor := ConfLow
			if matchTextOrScript(p.textRegex, p.scriptRegex, linkText) {
				floor = ConfMedium // path AND text agree
			}
			return p.docType, "url_path", floor
		}
	}
	// Pass 2: specific (non-hub) link-text matches (path didn't match any type).
	// This is the arm that fires on CJK / Cyrillic footer links whose href is an
	// opaque ID (e.g. 服务协议 → /rule/202504020001): the script-aware text match
	// is the only available signal, so it classifies on anchor text alone.
	for _, p := range patterns {
		if hubDocTypes[p.docType] {
			continue
		}
		if p.docType == DocDisclaimer {
			continue // disclaimer text alone is not a legal-document location
		}
		if matchTextOrScript(p.textRegex, p.scriptRegex, linkText) {
			return p.docType, "link_text", ConfLow
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
			return p.docType, "url_path", ConfLow
		}
		if p.textRegex != nil && p.textRegex.MatchString(linkText) {
			return p.docType, "link_text", ConfLow
		}
	}
	return "", "", ConfNone
}
