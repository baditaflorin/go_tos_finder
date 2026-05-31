package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestClassifyScriptLinkText verifies that CJK / Cyrillic / Greek / Turkish /
// Arabic legal anchor text classifies to the right doc type even when the href
// path is an opaque ID (the common non-Western footer-link shape, e.g.
// qq.com 服务协议 -> /rule/202504020001). These are REAL anchor phrases scraped
// from naver.com / qq.com / tencent.com / vk.com / rakuten.co.jp footers.
func TestClassifyScriptLinkText(t *testing.T) {
	cases := []struct {
		name     string
		path     string // opaque/ID path with no Latin token
		text     string
		wantType DocType
	}{
		// Chinese (qq.com / tencent.com real footer)
		{"zh service agreement", "/rule/202504020001", "服务协议", DocTermsOfService},
		{"zh privacy policy", "/mb/policy/tencent-privacypolicy", "隐私政策", DocPrivacyPolicy},
		{"zh-tw terms", "/p/abc", "服務條款", DocTermsOfService},
		{"zh-tw privacy", "/p/xyz", "隱私權政策", DocPrivacyPolicy},
		{"zh legal notice", "/p/legal", "法律声明", DocImprint},
		// Japanese (rakuten / common JP footer)
		{"ja terms of use", "/info/12345", "利用規約", DocTermsOfService},
		{"ja privacy policy", "/info/privacy", "プライバシーポリシー", DocPrivacyPolicy},
		{"ja tokushoho imprint", "/info/law", "特定商取引法に基づく表記", DocImprint},
		{"ja cookie", "/info/cookie", "クッキーポリシー", DocCookiePolicy},
		// Korean (naver / coupang footer)
		{"ko terms", "/policy/service", "이용약관", DocTermsOfService},
		{"ko privacy", "/policy/privacy", "개인정보처리방침", DocPrivacyPolicy},
		{"ko business info", "/info/seller", "사업자정보", DocImprint},
		// Russian / Ukrainian (vk / yandex footer)
		{"ru user agreement", "/terms", "Пользовательское соглашение", DocTermsOfService},
		{"ru privacy", "/privacy", "Политика конфиденциальности", DocPrivacyPolicy},
		// Greek / Turkish / Arabic / Thai / Vietnamese
		{"el terms", "/oroi", "Όροι χρήσης", DocTermsOfService},
		{"tr privacy", "/g", "Gizlilik Politikası", DocPrivacyPolicy},
		{"ar terms", "/t", "شروط الاستخدام", DocTermsOfService},
		{"th privacy", "/p", "นโยบายความเป็นส่วนตัว", DocPrivacyPolicy},
		{"vi terms", "/dk", "Điều khoản sử dụng", DocTermsOfService},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dt, how := classifyLink(c.path, c.text)
			if dt != c.wantType {
				t.Errorf("classifyLink(%q,%q) type=%q want %q (how=%q)", c.path, c.text, dt, c.wantType, how)
			}
		})
	}
}

// TestScriptLinkTextDoesNotFalsePositive: ordinary non-Latin prose / navigation
// must NOT classify as a legal document. Guards against the script gazetteer
// over-firing.
func TestScriptLinkTextDoesNotFalsePositive(t *testing.T) {
	cases := []struct {
		path, text string
	}{
		{"/news", "新闻中心"},        // zh "News center"
		{"/shop", "商品一覧"},        // ja "Product list"
		{"/login", "로그인"},        // ko "Login"
		{"/about", "О компании"}, // ru "About the company"
		{"/home", "Ana Sayfa"},   // tr "Home page"
	}
	for _, c := range cases {
		t.Run(c.text, func(t *testing.T) {
			if dt, _ := classifyLink(c.path, c.text); dt != "" {
				t.Errorf("classifyLink(%q,%q) should not classify, got %q", c.path, c.text, dt)
			}
		})
	}
}

// TestClassifyBodyScriptTitle: a fetched page whose <title>/<h1> is a CJK or
// Cyrillic legal-doc name is high-confidence confirmation of the type.
func TestClassifyBodyScriptTitle(t *testing.T) {
	body := func(title, h1 string) string {
		return `<html><head><title>` + title + `</title></head><body><h1>` + h1 + `</h1>` +
			strings.Repeat("本規約に同意の上ご利用ください。", 30) + `</body></html>`
	}
	cases := []struct {
		name  string
		title string
		h1    string
		want  DocType
	}{
		{"ja terms title", "利用規約 | Example", "利用規約", DocTermsOfService},
		{"zh privacy title", "隐私政策", "隐私政策", DocPrivacyPolicy},
		{"ko privacy h1", "Example", "개인정보처리방침", DocPrivacyPolicy},
		{"ru terms title", "Пользовательское соглашение", "Условия", DocTermsOfService},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vr := classifyBody(body(c.title, c.h1), 200, c.want)
			if !vr.IsReal {
				t.Fatalf("script legal page should be real: %+v", vr)
			}
			if vr.Confidence != ConfHigh {
				t.Errorf("script title/h1 match should be high, got %q (ev=%v)", vr.Confidence, vr.Evidence)
			}
		})
	}
}

// TestScriptSoftNotFound: a CJK / Cyrillic soft-404 page must still be rejected
// (geo FP-hardening — a Japanese/Chinese/Korean/Russian "page not found" that
// 200s should not count as a found document).
func TestScriptSoftNotFound(t *testing.T) {
	for _, msg := range []string{
		"<title>ページが見つかりません</title>",
		"<title>页面不存在</title>",
		"<title>페이지를 찾을 수 없습니다</title>",
		"<title>Страница не найдена</title>",
	} {
		body := `<html><head>` + msg + `</head><body>` + msg + `</body></html>`
		vr := classifyBody(body, 200, DocTermsOfService)
		if vr.IsReal {
			t.Errorf("script soft-404 should be rejected: %q -> %+v", msg, vr)
		}
	}
}

// TestBodyHasTypeSignalScript: the canonical-probe FP guard accepts a CJK /
// Cyrillic legal body (via the type scriptRegex or script legal vocab) and
// rejects unrelated non-Latin content.
func TestBodyHasTypeSignalScript(t *testing.T) {
	cases := []struct {
		name string
		body string
		want DocType
		ok   bool
	}{
		{"ja privacy body", "本ポリシーは個人情報の取り扱いについて定めます。", DocPrivacyPolicy, true},
		{"zh terms body", "本协议适用于您使用本服务的行为。服务条款如下。", DocTermsOfService, true},
		{"ko privacy body", "본 개인정보처리방침은 회원의 개인정보를 보호합니다.", DocPrivacyPolicy, true},
		{"ru terms body", "Настоящее соглашение регулирует использование сервиса.", DocTermsOfService, true},
		// Unrelated CJK marketing copy must NOT pass for a niche probe type.
		{"zh marketing not shipping", "欢迎来到我们的视频平台，观看精彩内容。", DocShippingPolicy, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bodyHasTypeSignal(c.body, c.want); got != c.ok {
				t.Errorf("bodyHasTypeSignal(%s)=%v want %v", c.want, got, c.ok)
			}
		})
	}
}

// TestE2EChineseFooterDetection: end-to-end reproduction of the qq.com / Tencent
// case the deployed 1.2.1 returned `none` for. A homepage whose footer carries
// Chinese legal anchors (服务协议, 隐私政策) pointing at opaque rule/policy paths
// must now be detected — proving the link-text script gazetteer closes the gap.
func TestE2EChineseFooterDetection(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	// Real-shape Tencent footer: opaque href, Chinese anchor text.
	home := `<!DOCTYPE html><html><head><title>腾讯网</title></head><body>
<footer>
<a href="https://example.test/rule/202504020001" class="link-item">服务协议</a>
<a href="https://example.test/mb/policy/privacypolicy" class="link-item">隐私政策</a>
<a href="https://www.tencent.com/zh-cn/" class="link-item">关于腾讯</a>
</footer></body></html>`
	realTerms := `<html><head><title>服务协议</title></head><body><h1>服务协议</h1>` +
		strings.Repeat("本协议适用于您使用本服务的行为，服务条款如下。", 30) + `</body></html>`
	realPrivacy := `<html><head><title>隐私政策</title></head><body><h1>隐私政策</h1>` +
		strings.Repeat("本政策说明我们如何收集和使用您的个人信息。", 30) + `</body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(home))
		case strings.HasPrefix(r.URL.Path, "/rule/"):
			_, _ = w.Write([]byte(realTerms))
		case strings.HasPrefix(r.URL.Path, "/mb/policy/"):
			_, _ = w.Write([]byte(realPrivacy))
		default:
			// 114-byte parking stub for any English canonical probe.
			_, _ = w.Write([]byte(`<html><head><meta http-equiv="refresh" content="0;url=/"></head></html>`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Rewrite the absolute example.test footer hrefs onto the test server so the
	// Phase-3 verification GET hits our handlers (keeps the anchor text intact).
	home = strings.ReplaceAll(home, "https://example.test", srv.URL)
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/":
			_, _ = w.Write([]byte(home))
		case strings.HasPrefix(r.URL.Path, "/rule/"):
			_, _ = w.Write([]byte(realTerms))
		case strings.HasPrefix(r.URL.Path, "/mb/policy/"):
			_, _ = w.Write([]byte(realPrivacy))
		default:
			_, _ = w.Write([]byte(`<html><head><meta http-equiv="refresh" content="0;url=/"></head></html>`))
		}
	})
	srv.Config.Handler = mux2

	req := httptest.NewRequest(http.MethodGet, "/?target="+srv.URL, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rec.Body.String())
	}
	byType := map[DocType]DocFinding{}
	for _, d := range resp.Documents {
		byType[d.Type] = d
	}
	if _, ok := byType[DocTermsOfService]; !ok {
		t.Errorf("Chinese 服务协议 footer link should yield terms_of_service; docs=%+v verdict=%q", resp.Documents, resp.Verdict)
	}
	if _, ok := byType[DocPrivacyPolicy]; !ok {
		t.Errorf("Chinese 隐私政策 footer link should yield privacy_policy; docs=%+v", resp.Documents)
	}
	if resp.Verdict == "none" {
		t.Errorf("verdict should not be 'none' for a site with Chinese ToS+Privacy footer links")
	}
}

// TestIDNUnicodeHostNormalisation: a Unicode-form (U-label) IDN host must be
// converted to its ASCII A-label (punycode) before fetch. Pre-fix, the handler
// percent-encoded the UTF-8 host and the fetch failed for every non-ASCII
// domain (a pure geo bug — only non-Western registrants type U-label hosts).
func TestIDNUnicodeHostNormalisation(t *testing.T) {
	cases := []struct {
		in       string
		wantHost string // expected ASCII A-label in the normalised target
	}{
		{"bücher.de", "xn--bcher-kva.de"},
		{"http://münchen.de/impressum", "xn--mnchen-3ya.de"},
		{"https://россия.рф", "xn--h1alffa9f.xn--p1ai"},
		{"日本.jp", "xn--wgv71a.jp"},
		// Already-ASCII host must pass through unchanged.
		{"example.com", "example.com"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			raw := c.in
			if !strings.Contains(raw, "://") {
				raw = "https://" + raw
			}
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatalf("parse %q: %v", raw, err)
			}
			normaliseIDN(u)
			if u.Hostname() != c.wantHost {
				t.Errorf("normalised host for %q = %q, want %q", c.in, u.Hostname(), c.wantHost)
			}
		})
	}
}
