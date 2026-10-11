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
	DocTermsOfService: regexp.MustCompile(`(?i)\b(?:terms\s+(?:of\s+)?(?:service|use)|terms\s+and\s+conditions|user\s+agreement|acceptance\s+of\s+these\s+terms|by\s+(?:using|accessing)|nutzungsbedingungen|gesch[äa]ftsbedingungen|algemene\s+voorwaarden|gebruiksvoorwaarden|voorwaarden|regulamin|warunki(?:\s+(?:korzystania|użytkowania))?|termeni\s+[șs]i\s+condi[țt]ii|obchodn[íi]\s+podm[íi]nky|k[äa]ytt[öo]ehdot|anv[äa]ndarvillkor|handelsvillkor|vilk[åa]r)\b`),
	// datenschutz(?:erkl[äa]rung|hinweise)? — see the matching titleRegex fix
	// in patterns.go: a bare `\bdatenschutz\b` cannot match inside the standard
	// German compound "Datenschutzerklärung", so a canonical-probed German-only
	// privacy page (title AND body using only the compound form, no English
	// "privacy"/"we collect" vocabulary) could fail this false-positive guard
	// entirely and be silently rejected as noise, not merely under-scored.
	DocPrivacyPolicy:       regexp.MustCompile(`(?i)\b(?:privacy|personal\s+(?:data|information)|datenschutz(?:erkl[äa]rung|hinweise)?|we\s+collect|how\s+we\s+(?:use|process)|data\s+we\s+collect|cookies?\s+and\s+similar)\b`),
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

// Fast (prefiltered) twins of the body-level vocabulary regexes. The plain
// regexes stay the single source of truth for the accepted language (and are
// still used on short strings: titles, link text); these wrappers add a sound
// necessary-literal prefilter so a non-matching 24 KB body costs a handful of
// strings.Contains passes instead of a full NFA scan. See fastre.go.
var (
	softNotFoundFast     = newFastRE(softNotFoundRE)
	legalVocabFast       = newFastRE(legalVocabRE)
	scriptLegalVocabFast = newFastRE(scriptLegalVocabRE)
	homepageBlockFast    = newFastRE(homepageBlockRE)
	docTypeSignalFast    = func() map[DocType]*fastRE {
		m := make(map[DocType]*fastRE, len(docTypeSignalRE))
		for t, re := range docTypeSignalRE {
			m[t] = newFastRE(re)
		}
		return m
	}()
	scriptSignalFast = func() map[DocType]*fastRE {
		m := make(map[DocType]*fastRE)
		for _, p := range patterns {
			if p.scriptRegex != nil {
				m[p.docType] = newFastRE(p.scriptRegex)
			}
		}
		return m
	}()
)

// maxVerifyText bounds the visible text a verification looks at. A real legal
// document announces itself (title, h1, "these terms", "we collect", "last
// updated") in its opening paragraphs; scanning more only burns CPU.
const maxVerifyText = 24 * 1024

// softTextWindow / softShortText implement the soft-404 text rule: a branded
// error template is SHORT, so the body-text phrase check (not found, 404, coming
// soon, ...) only applies to pages with little visible text. Long legal
// documents routinely contain "not found" / "404" in ordinary prose or phone
// numbers and must not be rejected for it.
const softShortText = 6 * 1024

// bodyView is the one-time, linear-time analysis of a fetched body from which
// every verification decision is derived. Building it once per distinct body
// (the handler memoises on a body hash) is what keeps a site that answers all 30
// canonical probes with the same SPA shell from costing 30 analyses.
type bodyView struct {
	trimmedLen int
	small      bool
	pdf        bool
	title      string
	h1         string
	text       string // visible text, <= maxVerifyText
	lower      string // strings.ToLower(text)
	redirect   bool   // tiny body that JS/meta-redirects away
	soft404    bool
	legalVocab bool
}

func analyzeBody(body string) *bodyView {
	v := &bodyView{}
	trimmed := strings.TrimSpace(body)
	v.trimmedLen = len(trimmed)
	v.small = v.trimmedLen < minRealDocBytes
	if strings.HasPrefix(trimmed, "%PDF-") {
		v.pdf = true
		return v
	}
	v.title = cleanText(tagInner(body, "title"))
	v.h1 = cleanText(tagInner(body, "h1"))
	if v.small {
		v.redirect = jsRedirectRE.MatchString(body) || metaRefreshRE.MatchString(body)
	}
	v.text = visibleText(body, maxVerifyText)
	v.lower = strings.ToLower(v.text)
	// Soft-404 / parked / placeholder: title or h1 naming it is disqualifying on
	// any page; the same phrases inside the body text only count on short pages.
	switch {
	case softNotFoundFast.MatchString(v.title), softNotFoundFast.MatchString(v.h1):
		v.soft404 = true
	case len(v.text) < softShortText && softNotFoundFast.Match(v.text, v.lower):
		v.soft404 = true
	}
	v.legalVocab = legalVocabFast.Match(v.text, v.lower) || scriptLegalVocabFast.Match(v.text, v.lower)
	return v
}

// hasTypeSignal reports whether the view carries type-specific vocabulary for
// want (Latin docTypeSignalRE or the type's non-Latin scriptRegex), or generic
// legal vocabulary in any script. Used as the false-positive guard on
// canonical-probe hits.
func (v *bodyView) hasTypeSignal(want DocType) bool {
	if re, ok := docTypeSignalFast[want]; ok && re.Match(v.text, v.lower) {
		return true
	}
	if sr, ok := scriptSignalFast[want]; ok && sr.Match(v.text, v.lower) {
		return true
	}
	return v.legalVocab
}

// bodyHasTypeSignal is the single-body convenience form of (*bodyView).hasTypeSignal.
func bodyHasTypeSignal(body string, want DocType) bool {
	return analyzeBody(body).hasTypeSignal(want)
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
// status of the GET.
//
// Decision logic (precision-first — this is the fix for the ~528k soft-404
// false positives in production):
//   - A 2xx with a tiny body that JS/meta-redirects away  -> NOT real.
//   - A title/h1 (or, on short pages, body text) that screams "404 / parked /
//     for sale"                                            -> NOT real.
//   - A body whose <title>/<h1> matches the doc type's titleRegex -> high.
//   - A body whose title names a DIFFERENT legal document type    -> NOT real
//     for this type (a /terms-of-use URL that serves the privacy policy).
//   - A body that's a substantial page with legal vocabulary      -> medium.
//   - Anything else that's a real page but unconfirmed            -> low.
func classifyBody(body string, httpStatus int, want DocType) verifyResult {
	return classifyView(analyzeBody(body), httpStatus, want)
}

// classifyView is classifyBody over a pre-computed bodyView.
func classifyView(v *bodyView, httpStatus int, want DocType) verifyResult {
	res := verifyResult{Confidence: ConfNone}
	// Error responses can reuse legal templates or CDN challenge text; they
	// are not evidence that the requested document exists.
	if httpStatus < 200 || httpStatus >= 400 {
		res.Evidence = append(res.Evidence, "rejected:http_status")
		return res
	}
	if v.pdf {
		// A PDF is a real document; its type can only be vouched for by the
		// link (path/anchor text) that led here, which the caller applies as
		// the confidence floor.
		res.IsReal = true
		res.Confidence = ConfLow
		res.Evidence = append(res.Evidence, "pdf_document")
		return res
	}
	res.Title = v.title

	// Gate 1: redirect-stub parking pages (the 114-byte production pattern).
	if v.small && v.redirect {
		res.Evidence = append(res.Evidence, "rejected:redirect_stub")
		return res
	}

	// Gate 2: soft-404 / parked / placeholder text is disqualifying even on a
	// large branded error page — see analyzeBody for how title/h1/short-text
	// are weighed (script text no longer counts).
	if v.soft404 {
		res.Evidence = append(res.Evidence, "rejected:soft_404")
		return res
	}

	// Gate 3: empty/near-empty body with no useful title is not a document.
	if v.small && res.Title == "" {
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
		if matchTextOrScript(tr, sr, res.Title) || matchTextOrScript(tr, sr, v.h1) {
			res.Confidence = ConfHigh
			res.Evidence = append(res.Evidence, "title_matches_type")
			return res
		}
	}
	// Cross-type guard: the page announces itself (title or h1) as a DIFFERENT
	// legal document than the one we are looking for — e.g. /terms-of-use that
	// serves the Privacy Policy — so it is no evidence for `want`.
	if other := otherTitleType(want, res.Title, v.h1); other != "" {
		res.IsReal = false
		res.Confidence = ConfNone
		res.Evidence = append(res.Evidence, "rejected:title_is_"+string(other))
		return res
	}
	if v.legalVocab {
		res.Confidence = ConfMedium
		res.Evidence = append(res.Evidence, "legal_vocabulary")
		return res
	}
	res.Confidence = ConfLow
	res.Evidence = append(res.Evidence, "page_exists_unconfirmed")
	return res
}

// otherTitleType returns the first non-hub doc type OTHER than want whose
// title/script regex matches the page's title or h1, or "" when none does. Only
// the unambiguous core types are consulted: ToS, privacy, cookie, imprint —
// the four a mis-served page is realistically confused with — to keep the guard
// narrow (no "legal" vs "terms" hair-splitting).
func otherTitleType(want DocType, title, h1 string) DocType {
	for _, t := range []DocType{DocTermsOfService, DocPrivacyPolicy, DocCookiePolicy, DocImprint} {
		if t == want {
			continue
		}
		p := patternByType[t]
		if matchTextOrScript(p.titleRegex, p.scriptRegex, title) || matchTextOrScript(p.titleRegex, p.scriptRegex, h1) {
			return t
		}
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
