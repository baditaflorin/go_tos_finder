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

	// scriptRegex matches CJK / Cyrillic / Greek / Arabic / Thai / Hindi
	// legal-document phrases in link text, titles, and bodies. These scripts
	// have no ASCII word boundaries (Go's regexp `\b` is ASCII-only), so the
	// ASCII-anchored textRegex/titleRegex never fire on a Korean/Chinese/
	// Japanese/Russian footer link such as "服务协议" / "이용약관" /
	// "利用規約" / "Пользовательское соглашение". scriptRegex is matched in
	// addition to (not instead of) textRegex/titleRegex everywhere they are
	// consulted, closing the non-Latin-script geo gap without weakening any
	// existing Latin-script precision.
	scriptRegex *regexp.Regexp
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

// scriptRE compiles a case-insensitive alternation of literal non-Latin-script
// phrases with NO ASCII word boundaries. CJK / Cyrillic / Greek / Arabic /
// Devanagari / Thai have no `\b` notion (and Go's `\b` is ASCII-only), so these
// phrases must be matched as bare substrings. The phrases listed are distinctive
// multi-character legal terms (e.g. 利用規約, 개인정보처리방침, Пользовательское
// соглашение) that do not collide with ordinary prose, so a substring match is
// high-precision. Returns nil when no phrases are supplied.
func scriptRE(phrases ...string) *regexp.Regexp {
	if len(phrases) == 0 {
		return nil
	}
	escaped := make([]string, len(phrases))
	for i, p := range phrases {
		escaped[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(?i)(?:` + strings.Join(escaped, "|") + `)`)
}

// matchTextOrScript reports whether s matches either the ASCII-anchored
// textRegex/titleRegex (re) OR the non-Latin scriptRegex (sr). Either being nil
// is treated as "no match for that arm". This is the single place that fuses
// Latin and non-Latin link/title/body matching.
func matchTextOrScript(re, sr *regexp.Regexp, s string) bool {
	if re != nil && re.MatchString(s) {
		return true
	}
	if sr != nil && sr.MatchString(s) {
		return true
	}
	return false
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
		scriptRegex: scriptRE(
			// Japanese: terms of use / member agreement
			"利用規約", "利用条件", "ご利用規約", "会員規約", "サービス利用規約",
			// Chinese (simplified + traditional): service terms / agreement
			"服务条款", "服務條款", "服务协议", "服務協議", "使用条款", "使用條款", "用户协议", "用戶協議", "用户协定",
			// Korean: terms of use / terms of service
			"이용약관", "이용 약관", "서비스 이용약관", "이용규약",
			// Russian / Ukrainian: user agreement / terms of use
			"Пользовательское соглашение", "Условия использования", "Условия использова",
			"Умови використання", "Угода користувача",
			// Greek: terms of use
			"Όροι χρήσης", "Όροι Χρήσης",
			// Turkish: terms of use / membership agreement
			"Kullanım Koşulları", "Kullanım Şartları", "Üyelik Sözleşmesi", "Hizmet Şartları",
			// Arabic: terms of use / terms and conditions
			"شروط الاستخدام", "الشروط والأحكام", "اتفاقية الاستخدام",
			// Thai / Hindi / Indonesian-Malay / Vietnamese
			"ข้อกำหนดการใช้งาน", "उपयोग की शर्तें", "सेवा की शर्तें",
			"Syarat Penggunaan", "Ketentuan Layanan", "Điều khoản sử dụng", "Điều khoản dịch vụ",
		),
		canonicalPaths: []string{
			"/terms", "/terms-of-service", "/terms-of-use", "/terms-and-conditions",
			"/tos", "/legal/terms", "/conditions", "/user-agreement", "/eula", "/agb",
			// non-Latin canonical paths seen in the wild (CJK sites frequently
			// expose romanised or kanji policy paths)
			"/kiyaku", "/riyou-kiyaku", "/利用規約", "/terms-of-use.html",
			"/agreement", "/useragreement", "/法律声明", "/服务协议",
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
		scriptRegex: scriptRE(
			// Japanese: privacy policy / handling of personal information
			"プライバシーポリシー", "プライバシー", "個人情報保護方針", "個人情報の取り扱い", "個人情報保護",
			// Chinese (simplified + traditional): privacy policy
			"隐私政策", "隱私政策", "隱私權政策", "隐私权政策", "隐私声明", "隱私聲明", "个人信息保护", "個人資料",
			// Korean: privacy policy / handling of personal data
			"개인정보처리방침", "개인정보 처리방침", "개인정보보호정책", "개인정보보호", "개인정보취급방침",
			// Russian / Ukrainian: privacy policy
			"Политика конфиденциальности", "Конфиденциальность", "Политика конфиденц",
			"Політика конфіденційності",
			// Greek / Turkish / Arabic
			"Πολιτική απορρήτου", "Πολιτική Απορρήτου",
			"Gizlilik Politikası", "Gizlilik İlkesi", "Kişisel Verilerin Korunması",
			"سياسة الخصوصية", "الخصوصية",
			// Thai / Hindi / Indonesian-Malay / Vietnamese
			"นโยบายความเป็นส่วนตัว", "गोपनीयता नीति",
			"Kebijakan Privasi", "Dasar Privasi", "Chính sách bảo mật", "Chính sách quyền riêng tư",
		),
		canonicalPaths: []string{
			"/privacy", "/privacy-policy", "/privacy-notice", "/legal/privacy",
			"/datenschutz", "/politique-confidentialite",
			// non-Latin / romanised privacy paths seen in the wild
			"/privacypolicy", "/privacy.html", "/personal-information",
			"/kojinjoho", "/隐私政策", "/隱私權政策", "/개인정보처리방침",
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
		scriptRegex: scriptRE(
			// Japanese / Chinese / Korean / Russian / Greek / Turkish / Arabic cookie policy
			"クッキーポリシー", "クッキーの使用について", "Cookieポリシー",
			"Cookie政策", "Cookie 政策", "Cookie使用政策", "Cookie聲明",
			"쿠키 정책", "쿠키정책",
			"Политика использования файлов cookie", "Политика cookie", "файлов cookie",
			"Πολιτική cookies", "Çerez Politikası", "سياسة ملفات تعريف الارتباط",
		),
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
		docType:      DocRefundPolicy,
		pathPatterns: pathRE("refunds", "returns", "refund-policy", "return-policy", "refunds-and-returns", "returns-policy", "cancellation-policy"),
		textRegex:    regexp.MustCompile(`(?i)\b(?:refund(?:\s+policy)?|returns?\s+(?:and\s+refunds?\s+)?policy|return\s+policy|cancellation\s+policy)\b`),
		titleRegex:   regexp.MustCompile(`(?i)\b(?:refund\s+policy|returns?\s+policy|cancellation\s+policy)\b`),
		scriptRegex: scriptRE(
			// Japanese / Chinese / Korean / Russian refund & return policy
			"返品・交換", "返品ポリシー", "返金ポリシー", "返品について", "キャンセルポリシー",
			"退款政策", "退货政策", "退換貨政策", "退貨政策", "退款退货",
			"환불정책", "환불 정책", "교환/환불", "반품정책",
			"Политика возврата", "Возврат товара",
		),
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
		textRegex:  regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|note\s+legali|aviso\s+legal|colof[oó]n)\b`),
		titleRegex: regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|aviso\s+legal)\b`),
		scriptRegex: scriptRE(
			// Japanese: Specified Commercial Transactions Act disclosure (the
			// JP-mandated seller/company legal-info page — the imprint analogue),
			// plus company-profile / legal-notice
			"特定商取引法に基づく表記", "特定商取引法", "特定商取引法に基づく表示", "会社概要", "運営会社", "法的事項",
			// Chinese: legal notice / business licence / about-company
			"法律声明", "法律聲明", "营业执照", "營業執照", "公司信息", "公司資訊",
			// Korean: business operator information / company info
			"사업자정보", "사업자 정보", "회사소개", "사업자등록번호",
			// Russian / Greek / Turkish / Arabic legal-notice / company-info
			"Правовая информация", "Реквизиты", "Юридическая информация",
			"Νομική σημείωση", "Yasal Uyarı", "Şirket Bilgileri", "إشعار قانوني",
		),
		canonicalPaths: []string{
			"/imprint", "/impressum", "/legal-notice", "/mentions-legales",
			"/tokushoho", "/law", "/company", "/about-us", "/特定商取引法",
		},
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

// scriptRegexFor returns the non-Latin-script confirmation regex for a doc
// type, if any. Used by content verification to confirm a CJK/Cyrillic title or
// body (where the ASCII-anchored titleRegex can never fire).
func scriptRegexFor(t DocType) *regexp.Regexp {
	if p, ok := patternByType[t]; ok {
		return p.scriptRegex
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
