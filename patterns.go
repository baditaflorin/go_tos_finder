package main

import (
	"regexp"
	"strings"
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
			"termini-e-condizioni", "termini-e-condizioni-generali", "condizioni-generali",
			"terminos-y-condiciones", "condiciones-de-uso", "termos-e-condicoes", "termos-de-uso",
			"algemene-voorwaarden", "termeni-si-conditii", "termeni-si-condiții",
			"warunki-korzystania", "ogolne-warunki", "polzovatelskoe-soglashenie", "usloviya-ispolzovaniya",
		),
		textRegex:  regexp.MustCompile(`(?i)\b(?:terms\s+(?:of\s+)?(?:service|use|conditions)?|terms\s*&\s*conditions|terms\s+and\s+conditions|conditions\s+of\s+use|user\s+agreement|EULA|nutzungsbedingungen|allgemeine\s+gesch[äa]ftsbedingungen|AGB|conditions\s+d['e]\s*utilisation|conditions\s+g[ée]n[ée]rales|termini\s+(?:di\s+servizio|e\s+condizioni)|t[ée]rminos\s+(?:y\s+condiciones|de\s+servicio)|condiciones\s+(?:de\s+uso|del\s+servicio)|termos\s+(?:de\s+(?:uso|servi[çc]o)|e\s+condi[çc][õo]es)|gebruiksvoorwaarden|algemene\s+voorwaarden|regulamin|termeni\s+[șs]i\s+condi[țt]ii)\b`),
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
		textRegex: regexp.MustCompile(`(?i)\b(?:privacy(?:\s+(?:policy|notice|statement|choices|center|centre))?|data\s+privacy|datenschutz(?:erkl[äa]rung|hinweise)?|politique\s+de\s+confidentialit[ée]|informativa\s+(?:sulla\s+)?privacy|pol[íi]tica\s+de\s+privacidad|pol[íi]tica\s+de\s+privacidade|privacyverklaring|privacybeleid|polityka\s+prywatno[śs]ci|integritetspolicy)\b`),
		// datenschutz(?:erkl[äa]rung|hinweise)? — NOT bare "datenschutz": the
		// standalone word is rare as a page <title>, while "Datenschutzerklärung"
		// (the compound word) is the standard, near-universal title for a German
		// privacy policy page. A bare `\bdatenschutz\b` cannot match inside that
		// compound (no word boundary between "...schutz" and "erklärung..."), so
		// the single most common real German privacy-policy title was silently
		// falling through to medium/low confidence instead of the high-confidence
		// title match every other language gets. Found via the charset fix's own
		// test fixture (a real German title, decoded correctly, still failed to
		// classify as high-confidence) — a distinct, charset-independent bug.
		titleRegex: regexp.MustCompile(`(?i)\b(?:privacy\s+(?:policy|notice|statement)|datenschutz(?:erkl[äa]rung|hinweise)?|politique\s+de\s+confidentialit[ée]|informativa\s+(?:sulla\s+)?privacy|pol[íi]tica\s+de\s+privacid|privacyverklaring|polityka\s+prywatno[śs]ci)\b`),
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
			"legal-information", "legal-info", "company-information", "corporate-information",
			"about-company", "company-details", "datos-legales", "informacion-legal",
			"informazioni-legali", "informazioni-societarie", "informations-legales",
			"informations-juridiques", "informații-legale", "informatii-legale",
			"bedrijfsgegevens", "bedrijfsinformatie", "dane-firmy", "informacje-prawne", "company-profile",
		),
		textRegex:  regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|note\s+legali|aviso\s+legal|colof[oó]n|informazioni\s+(?:legali|societarie)|información\s+legal|datos\s+legales|informa[çc][õo]es\s+legais|dados\s+da\s+empresa|informations\s+l[ée]gales|bedrijfsgegevens|wettelijke\s+informatie|informa[țt]ii\s+legale|dane\s+firmy|informacje\s+prawne|juridische\s+informatie|hukuki\s+bilgiler)\b`),
		titleRegex: regexp.MustCompile(`(?i)\b(?:imprint|impressum|legal\s+notice|mentions\s+l[ée]gales|aviso\s+legal|informazioni\s+(?:legali|societarie)|información\s+legal|informations\s+l[ée]gales|bedrijfsgegevens|informa[țt]ii\s+legale)\b`),
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
			"Informații legale", "Informatii legale", "Informazioni legali", "Informazioni societarie",
			"Información legal", "Datos legales", "Informações legais", "Dados da empresa",
			"Informations légales", "Bedrijfsgegevens", "Dane firmy", "Informacje prawne",
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
