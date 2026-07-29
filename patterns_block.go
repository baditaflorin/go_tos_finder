package main

import (
	"regexp"
	"strings"
)

// homepageBlockRE matches the small, closed set of bot-block / WAF-challenge
// page signatures observed on REAL production domains during a live 1000-host
// sample run (2026-07-29): Cloudflare's "Just a moment... / Enable JavaScript
// and cookies to continue" JS challenge (herogames.com, bibra-information.co.uk,
// pinoytech.ph, insani24.de, ...), the stock Apache/Nginx "403 Forbidden / you
// don't have permission to access this resource" stub (photovoltaik-info.eu,
// gcvcc.org, caseycavanagh.tech, ...), and the common Incapsula/PerimeterX/
// generic-WAF "Access Denied / Request Rejected / verify you are human"
// wording. All of these return HTTP 200/403 (never 5xx, so fetchPage's
// existing >=500 guard never catches them) with a real, non-empty body — the
// homepage genuinely fetched, but the content is a bot/WAF interstitial, not
// the site. Before this fix the handler fed that interstitial straight into
// linkScan, which — correctly — found zero legal-document links in it, and
// the site was recorded as verdict:"none" / documents_found:0: an
// indistinguishable false negative next to a domain that really has no legal
// pages. 15/202 (~7%) of a real "no_data" production sample matched this
// signature on live re-fetch.
//
// Deliberately narrow and phrase-based (not size-based): a genuinely tiny
// real homepage must never be misclassified, so this only fires on the
// specific, well-known interstitial vocabulary these WAFs/CDNs emit, mirroring
// the existing softNotFoundRE precision approach.
var homepageBlockRE = regexp.MustCompile(`(?i)(?:` +
	`\b403\s+forbidden\b|` +
	`you\s+don't\s+have\s+permission\s+to\s+access|` +
	`\baccess\s+denied\b|` +
	`\brequest\s+rejected\b|` +
	`attention\s+required[^<]{0,40}cloudflare|` +
	`checking\s+your\s+browser\s+before\s+accessing|` +
	`enable\s+javascript\s+and\s+cookies\s+to\s+continue|` +
	`please\s+verify\s+you\s+are\s+a?\s*human|` +
	`verify\s+you\s+are\s+human|` +
	`unusual\s+traffic\s+from\s+your\s+computer\s+network|` +
	`incapsula\s+incident|` +
	`cf-browser-verification|` +
	`ddos\s+protection\s+by|` +
	`sorry,\s+you\s+have\s+been\s+blocked|` +
	`your\s+ip\s+(?:address\s+)?has\s+been\s+(?:blocked|banned|flagged)|` +
	`bot\s+detection|` +
	`pardon\s+our\s+interruption|` +
	`\bjust\s+a\s+moment\.\.\.` +
	`)`)

// minHomepageBytes: a 2xx/4xx homepage body shorter than this with no
// recognisable HTML content is treated the same as a block page — most real
// homepages (even minimal ones) are at least a few hundred bytes once
// boilerplate <head>/<html> markup is included. This catches bare rate-limit
// stubs ("Request failed", empty 200s) that carry no block vocabulary but are
// equally unusable as a real homepage fetch. Threshold chosen well below
// minRealDocBytes (600) used for canonical-probe documents, since some
// legitimate single-page sites ARE genuinely small; empirically the observed
// stubs (esc-computer.ch len=0, marcone.com len=41, mongolia-trips.com len=13)
// are all under 50 bytes.
const minHomepageBytes = 50

// isHomepageBlocked reports whether a successfully-fetched (HTTP < 500)
// homepage body is actually a bot-block / WAF-challenge interstitial (or an
// unusably empty stub) rather than real site content, plus a short
// machine-readable reason token for the evidence trail.
func isHomepageBlocked(body string, httpStatus int) (bool, string) {
	if len(strings.TrimSpace(body)) < minHomepageBytes {
		return true, "empty_or_stub_body"
	}
	if homepageBlockRE.MatchString(body) {
		return true, "bot_or_waf_challenge"
	}
	return false, ""
}
