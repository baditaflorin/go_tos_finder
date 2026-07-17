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
var softNotFoundRE = regexp.MustCompile(`(?i)(?:\b(?:404|page\s+not\s+found|not\s+found|seite\s+nicht\s+gefunden|page\s+introuvable|p[áa]gina\s+no\s+encontrada|no\s+longer\s+available|coming\s+soon|under\s+construction|domain\s+(?:is\s+)?(?:for\s+sale|parked)|buy\s+this\s+domain|account\s+suspended|default\s+web\s+page)\b|(?:ページが見つかりません|お探しのページは見つかりません|页面不存在|頁面不存在|找不到页面|找不到網頁|페이지를 찾을 수 없습니다|страница не найдена|Страница не найдена))`)

// legalVocabRE matches generic legal-document vocabulary used as a weak
// corroborating signal when the title/h1 doesn't directly name the doc type.
var legalVocabRE = regexp.MustCompile(`(?i)\b(?:last\s+(?:updated|modified|revised)|effective\s+date|these\s+terms|this\s+(?:policy|agreement|notice)|we\s+(?:collect|process|use)\s+(?:your\s+)?(?:personal\s+)?(?:data|information)|governed\s+by|liability|indemnif|warrant|jurisdiction|arbitration|gerichtsstand|haftung|responsabilit[ée]|datos\s+personales|dati\s+personali)\b`)

// docTypeSignalRE maps each canonical doc type to a regex of body vocabulary
// that is *specific* to that type. It is the false-positive guard for
// canonical probes: a server that returns a generic 200 page (or a wildcard
// catch-all that happens to clear the soft-404 gate by being large) for
// /shipping or /refunds on a site that ships nothing produces a "low,
// page_exists_unconfirmed" finding that is almost always noise. Requiring a
// probed path's body to carry at least a type-specific term (or the generic
// legalVocab term, or a title match) before it counts removes that long tail
// of probe false positives seen on the 100 real rows. Link-discovered docs are
// exempt — the explicit link is itself the location evidence.
var docTypeSignalRE = map[DocType]*regexp.Regexp{
	DocTermsOfService:      regexp.MustCompile(`(?i)\b(?:terms\s+(?:of\s+)?(?:service|use)|terms\s+and\s+conditions|user\s+agreement|acceptance\s+of\s+these\s+terms|by\s+(?:using|accessing))\b`),
	DocPrivacyPolicy:       regexp.MustCompile(`(?i)\b(?:privacy|personal\s+(?:data|information)|datenschutz|we\s+collect|how\s+we\s+(?:use|process)|data\s+we\s+collect|cookies?\s+and\s+similar)\b`),
	DocCookiePolicy:        regexp.MustCompile(`(?i)\b(?:cookies?|tracking\s+technolog|local\s+storage|web\s+beacons?)\b`),
	DocAcceptableUse:       regexp.MustCompile(`(?i)\b(?:acceptable\s+use|prohibited\s+(?:uses?|activities|conduct)|may\s+not\s+use|abuse|fair\s+use)\b`),
	DocCommunityGuidelines: regexp.MustCompile(`(?i)\b(?:community\s+(?:guidelines|standards)|code\s+of\s+conduct|behaviou?r|respectful|harassment)\b`),
	DocDMCA:                regexp.MustCompile(`(?i)\b(?:DMCA|copyright\s+(?:infringement|owner|agent)|takedown|notice\s+and\s+takedown|17\s+U\.?S\.?C)\b`),
	DocCopyrightPolicy:     regexp.MustCompile(`(?i)\b(?:copyright|intellectual\s+property|trademark|all\s+rights\s+reserved|infringement)\b`),
	DocGDPRDPA:             regexp.MustCompile(`(?i)\b(?:GDPR|data\s+processing|data\s+controller|data\s+processor|sub[-\s]?processor|article\s+\d+|personal\s+data)\b`),
	DocSubprocessors:       regexp.MustCompile(`(?i)\b(?:sub[-\s]?processors?|vendors?|third[-\s]party\s+(?:service|provider)|hosting\s+provider)\b`),
	DocSLA:                 regexp.MustCompile(`(?i)\b(?:service\s+level|uptime|availability|credit|downtime|99\.\d+%)\b`),
	DocRefundPolicy:        regexp.MustCompile(`(?i)\b(?:refund|return|cancellation|money[-\s]back|exchange|reimburse)\b`),
	DocShippingPolicy:      regexp.MustCompile(`(?i)\b(?:shipping|delivery|dispatch|carrier|tracking\s+number|order\s+(?:will|is)\s+(?:ship|deliver))\b`),
	DocImprint:             regexp.MustCompile(`(?i)\b(?:impressum|imprint|registered\s+(?:office|address)|company\s+(?:number|registration)|handelsregister|umsatzsteuer|vat\s+(?:id|number)|gesch[äa]ftsf[üu]hrer|mentions\s+l[ée]gales)\b`),
	DocDisclaimer:          regexp.MustCompile(`(?i)\b(?:disclaimer|no\s+warrant|as\s+is|not\s+(?:liable|responsible)|haftungsausschluss)\b`),
}

// scriptLegalVocabRE matches non-Latin-script generic legal-document vocabulary
// (effective date / "we collect" / last-updated / governing-law phrasing) used
// to corroborate a CJK/Cyrillic/Greek/etc. legal page whose body has no Latin
// legalVocabRE hit. Distinctive multi-char phrases → substring match is
// high-precision.
var scriptLegalVocabRE = regexp.MustCompile(`(?i)(?:` + strings.Join([]string{
	// Japanese
	"個人情報", "本規約", "本ポリシー", "制定日", "改定日", "最終更新", "プライバシー", "利用規約",
	// Chinese (simplified + traditional)
	"个人信息", "本协议", "本政策", "隐私", "隱私", "更新日期", "生效日期", "我们收集", "適用",
	// Korean
	"개인정보", "본 약관", "본 정책", "시행일", "수집", "이용약관",
	// Russian / Ukrainian
	"персональных данных", "настоящ", "конфиденциальност", "обработк", "соглашени",
	// Greek / Turkish / Arabic
	"προσωπικών δεδομένων", "kişisel veri", "gizlilik", "البيانات الشخصية", "الخصوصية",
}, "|") + `)`)

// bodyHasTypeSignal reports whether a body carries type-specific vocabulary for
// `want` (Latin docTypeSignalRE or the type's non-Latin scriptRegex), or generic
// legal vocabulary in any script. Used as the false-positive guard on
// canonical-probe hits.
func bodyHasTypeSignal(body string, want DocType) bool {
	if re, ok := docTypeSignalRE[want]; ok && re.MatchString(body) {
		return true
	}
	if sr := scriptRegexFor(want); sr != nil && sr.MatchString(body) {
		return true
	}
	return legalVocabRE.MatchString(body) || scriptLegalVocabRE.MatchString(body)
}

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
	// Error responses can reuse legal templates or CDN challenge text; they
	// are not evidence that the requested document exists.
	if httpStatus < 200 || httpStatus >= 400 {
		res.Evidence = append(res.Evidence, "rejected:http_status")
		return res
	}
	trimmed := strings.TrimSpace(body)
	res.Title = strings.TrimSpace(stripTags(firstGroup(titleRE, body)))

	// Gate 1: redirect-stub parking pages (the 114-byte production pattern).
	smallBody := len(trimmed) < minRealDocBytes
	if smallBody && (jsRedirectRE.MatchString(body) || metaRefreshRE.MatchString(body)) {
		res.Evidence = append(res.Evidence, "rejected:redirect_stub")
		return res
	}

	// Gate 2: soft-404 / parked / placeholder text is disqualifying even on
	// a large branded error page.  The old small-body-only guard let CDN 200
	// error templates (often several KB of navigation) become legal docs.
	hay := res.Title + " " + stripTags(body)
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

	// Confidence scoring. The title/h1 confirmation is script-aware: a Latin
	// titleRegex OR a CJK/Cyrillic scriptRegex match is equally strong evidence
	// the page is the expected type (e.g. <title>利用規約</title>,
	// <h1>개인정보처리방침</h1>, <title>Политика конфиденциальности</title>).
	tr := titleRegexFor(want)
	sr := scriptRegexFor(want)
	if tr != nil || sr != nil {
		h1 := strings.TrimSpace(stripTags(firstGroup(h1RE, body)))
		if matchTextOrScript(tr, sr, res.Title) || matchTextOrScript(tr, sr, h1) {
			res.Confidence = ConfHigh
			res.Evidence = append(res.Evidence, "title_matches_type")
			return res
		}
	}
	if legalVocabRE.MatchString(body) || scriptLegalVocabRE.MatchString(body) {
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
