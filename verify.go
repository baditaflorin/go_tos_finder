package main

import (
	"regexp"
	"strings"
)

// Confidence bands for a discovered/verified document.
const (
	ConfHigh   = "high"
	ConfMedium = "medium"
	ConfLow    = "low"
	ConfNone   = "none"
)

// minRealDocBytes is the smallest plausible size for a real legal document
// page once tags are accounted for. Parking-page / soft-404 JS-redirect stubs
// observed in production are ~114 bytes; legitimate policy pages are KBs.
const minRealDocBytes = 600

// titleRE / h1RE pull the first <title> and first <h1> text from a body.
var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
var h1RE = regexp.MustCompile(`(?is)<h1[^>]*>(.*?)</h1>`)

// metaRefreshRE detects <meta http-equiv="refresh" content="0;url=...">.
var metaRefreshRE = regexp.MustCompile(`(?is)<meta[^>]+http-equiv\s*=\s*["']?refresh["']?[^>]*>`)

// jsRedirectRE detects a body whose <head> immediately bounces the browser
// elsewhere — the classic domain-parking soft-404 (window.location / replace).
var jsRedirectRE = regexp.MustCompile(`(?is)(?:window\.)?location(?:\.href)?\s*=\s*["']|location\.(?:replace|assign)\s*\(`)

// softNotFoundRE matches body/title text that signals a soft 404 / placeholder
// even though the HTTP status was 2xx.
var softNotFoundRE = regexp.MustCompile(`(?i)\b(?:404|page\s+not\s+found|not\s+found|seite\s+nicht\s+gefunden|page\s+introuvable|p[áa]gina\s+no\s+encontrada|no\s+longer\s+available|coming\s+soon|under\s+construction|domain\s+(?:is\s+)?(?:for\s+sale|parked)|buy\s+this\s+domain|account\s+suspended|default\s+web\s+page)\b`)

// legalVocabRE matches generic legal-document vocabulary used as a weak
// corroborating signal when the title/h1 doesn't directly name the doc type.
var legalVocabRE = regexp.MustCompile(`(?i)\b(?:last\s+(?:updated|modified|revised)|effective\s+date|these\s+terms|this\s+(?:policy|agreement|notice)|we\s+(?:collect|process|use)\s+(?:your\s+)?(?:personal\s+)?(?:data|information)|governed\s+by|liability|indemnif|warrant|jurisdiction|arbitration|gerichtsstand|haftung|responsabilit[ée]|datos\s+personales|dati\s+personali)\b`)

// verifyResult is the outcome of fetching a candidate URL's body and checking
// whether it is a genuine legal document of the expected type.
type verifyResult struct {
	IsReal     bool     // passes the soft-404 / placeholder gate
	Confidence string   // high|medium|low|none
	Title      string   // extracted <title>, trimmed
	Evidence   []string // human-readable signals that produced the verdict
}

// classifyBody inspects a fetched body and returns whether it is a genuine
// document and how confident we are it matches `want`. `httpStatus` is the
// status of the GET; `bodyLen` is the number of bytes read (the body itself
// may be longer if truncated, so pass the on-the-wire length when known).
//
// Decision logic (precision-first — this is the fix for the ~528k soft-404
// false positives in production):
//   - A 2xx with a tiny body that JS/meta-redirects away  -> NOT real.
//   - A body whose title/text screams "404 / parked / for sale" -> NOT real.
//   - A body whose <title>/<h1> matches the doc type's titleRegex -> high.
//   - A body that's a substantial page with legal vocabulary    -> medium.
//   - Anything else that's a real page but unconfirmed          -> low.
func classifyBody(body string, httpStatus int, want DocType) verifyResult {
	res := verifyResult{Confidence: ConfNone}
	trimmed := strings.TrimSpace(body)
	res.Title = strings.TrimSpace(stripTags(firstGroup(titleRE, body)))

	// Gate 1: redirect-stub parking pages (the 114-byte production pattern).
	smallBody := len(trimmed) < minRealDocBytes
	if smallBody && (jsRedirectRE.MatchString(body) || metaRefreshRE.MatchString(body)) {
		res.Evidence = append(res.Evidence, "rejected:redirect_stub")
		return res
	}

	// Gate 2: soft-404 / parked / placeholder text in the title or a small body.
	hay := res.Title
	if smallBody {
		hay = hay + " " + stripTags(body)
	}
	if softNotFoundRE.MatchString(hay) {
		res.Evidence = append(res.Evidence, "rejected:soft_404")
		return res
	}

	// Gate 3: empty/near-empty body with no useful title is not a document.
	if smallBody && res.Title == "" {
		res.Evidence = append(res.Evidence, "rejected:empty_body")
		return res
	}

	// Passed the rejection gates: it is a real page.
	res.IsReal = true

	// Confidence scoring.
	if tr := titleRegexFor(want); tr != nil {
		h1 := strings.TrimSpace(stripTags(firstGroup(h1RE, body)))
		if tr.MatchString(res.Title) || tr.MatchString(h1) {
			res.Confidence = ConfHigh
			res.Evidence = append(res.Evidence, "title_matches_type")
			return res
		}
	}
	if legalVocabRE.MatchString(body) {
		res.Confidence = ConfMedium
		res.Evidence = append(res.Evidence, "legal_vocabulary")
		return res
	}
	res.Confidence = ConfLow
	res.Evidence = append(res.Evidence, "page_exists_unconfirmed")
	return res
}

// firstGroup returns the first capture group of the first match, or "".
func firstGroup(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

// confidenceRank orders confidence bands so we can prefer the stronger of two
// findings for the same doc type.
func confidenceRank(c string) int {
	switch c {
	case ConfHigh:
		return 3
	case ConfMedium:
		return 2
	case ConfLow:
		return 1
	default:
		return 0
	}
}
