package main

// Regression tests for 1.9.0 (CPU / budget / precision / recall pass). Every
// fixture below is a real shape found on a real production-sample domain; the
// domain is named next to it. None of these tests touches the network.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- fastRE: the prefilter must never change what a regexp matches -----------

func allVocabularyRegexps() map[string]*regexp.Regexp {
	m := map[string]*regexp.Regexp{
		"softNotFound": softNotFoundRE, "legalVocab": legalVocabRE,
		"scriptLegalVocab": scriptLegalVocabRE, "homepageBlock": homepageBlockRE,
	}
	for t, re := range docTypeSignalRE {
		m["signal/"+string(t)] = re
	}
	for _, p := range patterns {
		if p.scriptRegex != nil {
			m["script/"+string(p.docType)] = p.scriptRegex
		}
	}
	return m
}

func TestFastREEquivalentToRegexp(t *testing.T) {
	pieces := []string{
		"Terms of Service", "PRIVACY POLICY", "page not found", "Page   Not\nFound", "404", "(404) 555-0100",
		"coming soon", "Access Denied", "just a moment...", "verify you are human", "Datenschutzerklärung",
		"利用規約", "개인정보", "Политика конфиденциальности", "we collect your personal data", "liability", "x", "lorem ipsum",
		"These terms", "DMCA takedown", "sub-processors", "99.9% uptime", "Impressum", "mentions légales", "GDPR",
		"<b>", "</p>", "cookies", "  ", "refund", "shipping", "this policy", "Last   updated", "ünïcödé", "İstanbul", "KELVIN",
	}
	rng := rand.New(rand.NewSource(7))
	for name, re := range allVocabularyRegexps() {
		f := newFastRE(re)
		for i := 0; i < 400; i++ {
			var sb strings.Builder
			for k, n := 0, rng.Intn(8); k < n; k++ {
				sb.WriteString(pieces[rng.Intn(len(pieces))])
				sb.WriteByte(" .,;"[rng.Intn(4)])
			}
			text := sb.String()
			if got, want := f.Match(text, strings.ToLower(text)), re.MatchString(text); got != want {
				t.Fatalf("%s: fastRE=%v regexp=%v on %q (anchors=%q)", name, got, want, text, f.anchors)
			}
		}
	}
	// The hot regexes must actually have a prefilter, or the CPU fix is void.
	for _, name := range []string{"softNotFound", "legalVocab", "homepageBlock", "scriptLegalVocab", "signal/terms_of_service", "signal/privacy_policy"} {
		if newFastRE(allVocabularyRegexps()[name]).anchors == nil {
			t.Errorf("%s has no derivable prefilter", name)
		}
	}
}

// --- linear-time HTML helpers ------------------------------------------------

func TestVisibleText(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"drops script and style", `<p>Hello</p><script>console.log("element not found")</script><style>.a{}</style><p>World</p>`, "Hello World"},
		{"drops comment", `a<!-- x --> b`, "a b"},
		{"entities and nbsp", `Terms&nbsp;&amp;&nbsp;Conditions &ndash; Acme`, "Terms & Conditions – Acme"},
		{"upper-case tags", `<SCRIPT>bad()</SCRIPT><DIV>ok</DIV>`, "ok"},
		{"unterminated script", `ok<script>never closed`, "ok"},
		{"lt in text", `a < b and c`, "a < b and c"},
		{"whitespace", "a \n\t b", "a b"},
	}
	for _, c := range cases {
		if got := visibleText(c.in, 0); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if got := visibleText(strings.Repeat("word ", 1000), 100); len(got) > 110 {
		t.Errorf("limit not honoured: %d bytes", len(got))
	}
}

func TestScanAnchorsMatchesRegexp(t *testing.T) {
	docs := []string{
		`<a href="/a">A</a><a href='/b' class=x>B</a><a href=/c>C</a>`,
		`<A HREF="/up">Up</A>`,
		`<a class="x" data-x="1" href = "/spaced">S</a>`,
		`<a name="top">no href</a><a href="/after">after</a>`,
		`<abbr href="/no">x</abbr><a href="/yes">y</a>`,
		`<a href="/nested"><span>in</span> <img alt="i"></a>`,
		`<a href="/a">one<a href="/b">two</a>`,
		`<a href="/unclosed">never closed`,
		`<a href="">empty</a>`,
		"<a\nhref=\"/nl\"\n>nl</a>",
		`<a title="x>y" href="/t">t</a>`,
		`<a href=/c>C</a >`,
		`<a href="/multi
line">x</a>`,
		`text <a href=/u>u</a> more <a href="/v">v</a>`,
	}
	for _, d := range docs {
		var want []anchor
		for _, m := range anchorRE.FindAllStringSubmatchIndex(d, -1) {
			var href, inner string
			switch {
			case m[2] != -1:
				href = d[m[2]:m[3]]
			case m[4] != -1:
				href = d[m[4]:m[5]]
			case m[6] != -1:
				href = d[m[6]:m[7]]
			}
			if m[8] != -1 {
				inner = d[m[8]:m[9]]
			}
			want = append(want, anchor{Href: cleanHref(href), Inner: inner, Offset: m[0]})
		}
		got := scanAnchors(d)
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("scanAnchors != anchorRE on %q\n got  %+v\n want %+v", d, got, want)
		}
	}
}

// --- soft-404 gate: script text must not reject real legal pages ------------

// academyart.edu/terms-of-use and avophotonics.com/terms: genuine documents
// whose inline JavaScript contains "not found" (console.log("... element not
// found ...")) were rejected as soft-404 — the visible-text-free gate read the
// script source.
func TestClassifyBodyIgnoresScriptText(t *testing.T) {
	body := `<html><head><title>Terms of Use - Academy of Art University</title>
<script>if(tries>=5){console.log("*** pm_script: IT element not found after 5 attempts");}</script></head>
<body><h1>Terms of Use</h1><p>These terms govern your use of the site.</p>` + strings.Repeat("By using the site you agree. ", 40) + `</body></html>`
	vr := classifyBody(body, 200, DocTermsOfService)
	if !vr.IsReal || vr.Confidence != ConfHigh {
		t.Fatalf("real terms page with 'not found' in a script must be accepted: %+v", vr)
	}
}

func TestClassifyBodySoftTextRule(t *testing.T) {
	long := `<html><head><title>Privacy Policy</title></head><body><h1>Privacy Policy</h1><p>` +
		strings.Repeat("We process personal data. ", 400) + ` If a provision is found invalid, the page was not found to be... call (404) 555-0100.</p></body></html>`
	if vr := classifyBody(long, 200, DocPrivacyPolicy); !vr.IsReal {
		t.Errorf("a long legal page mentioning 404/not found in prose must stay real: %+v", vr)
	}
	short := `<html><head><title>Acme</title></head><body><nav>Home Products Contact</nav><main><h2>Sorry</h2><p>The page you requested was not found.</p>` + strings.Repeat("x ", 150) + `</main></body></html>`
	if vr := classifyBody(short, 200, DocPrivacyPolicy); vr.IsReal {
		t.Errorf("a short branded error template must still be rejected: %+v", vr)
	}
	h1 := `<html><head><title>Acme</title></head><body><h1>Page Not Found</h1>` + strings.Repeat("nav ", 3000) + `</body></html>`
	if vr := classifyBody(h1, 200, DocPrivacyPolicy); vr.IsReal {
		t.Errorf("an h1 naming a 404 must reject even a long page: %+v", vr)
	}
}

// connectionsfoodservice.com: /terms-of-use answered 200 with the PRIVACY POLICY
// page and was recorded as the ToS (medium, title "Privacy Policy").
func TestClassifyBodyRejectsOtherDocumentTitle(t *testing.T) {
	body := `<html><head><title>Privacy Policy</title></head><body><h1>Privacy Policy</h1><p>This policy describes liability and jurisdiction.</p>` + strings.Repeat("We collect data. ", 60) + `</body></html>`
	if vr := classifyBody(body, 200, DocTermsOfService); vr.IsReal {
		t.Errorf("a page titled Privacy Policy is no evidence for terms_of_service: %+v", vr)
	}
	if vr := classifyBody(body, 200, DocPrivacyPolicy); !vr.IsReal || vr.Confidence != ConfHigh {
		t.Errorf("but it IS the privacy policy: %+v", vr)
	}
	// A combined page satisfies both.
	both := strings.Replace(body, "Privacy Policy", "Terms of Use and Privacy Policy", -1)
	for _, ty := range []DocType{DocTermsOfService, DocPrivacyPolicy} {
		if vr := classifyBody(both, 200, ty); !vr.IsReal {
			t.Errorf("combined page must satisfy %s: %+v", ty, vr)
		}
	}
}

func TestClassifyBodyPDF(t *testing.T) {
	vr := classifyBody("%PDF-1.7\n%âãÏÓ\n1 0 obj", 200, DocPrivacyPolicy)
	if !vr.IsReal || !hasEvidenceTok(vr.Evidence, "pdf_document") {
		t.Errorf("a PDF behind a legal link is a real document: %+v", vr)
	}
}

// --- homepage block gate ------------------------------------------------------

func TestIsHomepageBlockedSizeAndTitle(t *testing.T) {
	// ancestry.ca: a 540 KB branded 403 — big, but the title says it.
	big := `<html><head><title>Access denied | Ancestry -- a48dbaad</title></head><body>` + strings.Repeat("<p>nav item</p>", 40000) + `</body></html>`
	if blocked, _ := isHomepageBlocked(big, 403); !blocked {
		t.Error("a large block page that names itself in <title> must still be blocked")
	}
	// A big real page that merely contains the phrase in a script string.
	real := `<html><head><title>Acme Hosting</title><script>var m="Access denied";var j="Just a moment...";</script></head><body>` +
		strings.Repeat("<p>We host websites. </p>", 8000) + `</body></html>`
	if blocked, _ := isHomepageBlocked(real, 200); blocked {
		t.Error("a 200 KB real homepage with 'access denied' in a script string is not an interstitial")
	}
}

// --- link discovery recall ----------------------------------------------------

func TestLinkScanRecallFixes(t *testing.T) {
	base, _ := url.Parse("https://www.example.com/")
	cases := []struct {
		name   string
		html   string
		want   DocType
		urlHas string
	}{
		// ascendyoursuccess.com: "Terms &amp; Conditions" + /terms-%26-conditions
		{"entity text + encoded ampersand path", `<footer><a href="/terms-%26-conditions">Terms &amp; Conditions</a></footer>`, DocTermsOfService, "terms-%26-conditions"},
		{"entity text only", `<footer><a href="/p/123">Terms &amp; Conditions</a></footer>`, DocTermsOfService, "/p/123"},
		// daximart.com: /pages/aviso-de-privacidad
		{"aviso de privacidad path", `<footer><a href="/pages/aviso-de-privacidad"></a></footer>`, DocPrivacyPolicy, "aviso-de-privacidad"},
		{"aviso de privacidad text", `<footer><a href="/x/9">Aviso de Privacidad</a></footer>`, DocPrivacyPolicy, "/x/9"},
		// ecleanpro.com: /pdf/privacy.pdf
		{"pdf privacy", `<footer><a href="/pdf/privacy.pdf">Privacy Policy</a></footer>`, DocPrivacyPolicy, "privacy.pdf"},
		// dannypaulson.com: HTML inside a JS string -> href=\"...\" artefacts / %22 URLs
		{"escaped quote href", `<script>var t='<a href=\"https://www.example.com/privacy-policy\">Privacy Policy</a>';</script>`, DocPrivacyPolicy, "/privacy-policy"},
		// apex link when the site fetched as www (sameSiteHost used to reject it)
		{"apex link from www base", `<footer><a href="https://example.com/privacy">Privacy</a></footer>`, DocPrivacyPolicy, "https://example.com/privacy"},
		{"sibling subdomain", `<footer><a href="https://legal.example.com/terms">Terms</a></footer>`, DocTermsOfService, "legal.example.com"},
	}
	for _, c := range cases {
		hits := linkScan(c.html, base)
		got, ok := hits[c.want]
		if !ok {
			t.Errorf("%s: %s not found in %+v", c.name, c.want, hits)
			continue
		}
		if !strings.Contains(got.URL, c.urlHas) || strings.Contains(got.URL, "%22") || strings.Contains(got.URL, `\`) {
			t.Errorf("%s: URL %q should contain %q and be free of quote artefacts", c.name, got.URL, c.urlHas)
		}
	}
	// Platform/third-party links are still not this site's documents.
	for _, h := range []string{
		`<footer><a href="https://policies.google.com/privacy">Privacy</a></footer>`,
		`<footer><a href="https://www.facebook.com/policy.php">Privacy</a></footer>`,
		`<footer><a href="https://other-site.org/terms">Terms</a></footer>`,
	} {
		if hits := linkScan(h, base); len(hits) != 0 {
			t.Errorf("third-party legal link must not be adopted: %s -> %+v", h, hits)
		}
	}
}

func TestSameSiteHostRegistrableDomain(t *testing.T) {
	mk := func(s string) *url.URL { u, _ := url.Parse(s); return u }
	yes := [][2]string{
		{"https://example.com/p", "https://www.example.com/"}, {"https://a.b.example.co.uk/p", "https://example.co.uk/"},
		{"https://www.example.com/p", "https://www.example.com/"},
	}
	no := [][2]string{
		{"https://policies.google.com/p", "https://www.example.com/"},
		{"https://bar.myshopify.com/p", "https://foo.myshopify.com/"}, // multi-tenant public suffix
		{"https://example.org/p", "https://example.com/"},
		{"https://evilexample.com/p", "https://example.com/"},
	}
	for _, c := range yes {
		if !sameSiteHost(mk(c[0]), mk(c[1])) {
			t.Errorf("%s should be same-site as %s", c[0], c[1])
		}
	}
	for _, c := range no {
		if sameSiteHost(mk(c[0]), mk(c[1])) {
			t.Errorf("%s must not be same-site as %s", c[0], c[1])
		}
	}
}

// --- verdict ------------------------------------------------------------------

// megogo.net: /en/rules#agreement, #agreement-privacy, #agreement-privacy-cookie-policy
// were reported as three documents -> "comprehensive" from ONE page.
func TestVerdictCountsDistinctDocuments(t *testing.T) {
	one := []DocFinding{
		{Type: DocTermsOfService, URL: "https://megogo.net/en/rules", Confidence: ConfHigh, Status: 200},
		{Type: DocPrivacyPolicy, URL: "https://megogo.net/en/rules", Confidence: ConfHigh, Status: 200},
		{Type: DocCookiePolicy, URL: "https://megogo.net/en/rules/", Confidence: ConfMedium, Status: 200},
	}
	if v := verdictOfDocs(one); v != "partial" {
		t.Errorf("one combined page (3 labels) must be partial, got %q", v)
	}
	two := append([]DocFinding{}, one...)
	two[2].URL = "https://megogo.net/en/cookies"
	if v := verdictOfDocs(two); v != "comprehensive" {
		t.Errorf("ToS+privacy+cookie on 2 distinct URLs with ToS and privacy is comprehensive, got %q", v)
	}
	// A dead link must not satisfy "has privacy".
	dead := []DocFinding{
		{Type: DocTermsOfService, URL: "https://x.com/terms", Confidence: ConfHigh, Status: 200},
		{Type: DocPrivacyPolicy, URL: "https://x.com/privacy", Confidence: ConfNone, rejected: true},
		{Type: DocCookiePolicy, URL: "https://x.com/cookies", Confidence: ConfHigh, Status: 200},
	}
	if v := verdictOfDocs(dead); v != "partial" {
		t.Errorf("rejected privacy link must not count; want partial got %q", v)
	}
	if v := verdictOfDocs(nil); v != "none" {
		t.Errorf("no documents -> none, got %q", v)
	}
}

// --- end-to-end behaviours ----------------------------------------------------

func scanJSON(t *testing.T, target string) (int, Response) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/?target="+url.QueryEscape(target), nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rec.Body.String())
	}
	return rec.Code, resp
}

// buffalonew.com / bleepingcomputer.com / blackrock.com: the footer links were
// real, but the verification GET of the target was answered 429 / 403 (rate
// limit / bot challenge) after the homepage fetch and the burst of probes. The
// links were dropped and the site recorded as NoData.
func TestHandlerKeepsLinksWhenVerificationIsChallenged(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Acme</title></head><body><footer><a href="/terms-of-use">Terms of Use</a> <a href="/privacy">Privacy Policy</a></footer></body></html>`))
		default:
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`<html><head><title>Just a moment...</title></head></html>`))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, resp := scanJSON(t, srv.URL)
	if code != http.StatusOK || resp.DocumentsFound != 2 {
		t.Fatalf("challenged-but-linked docs must still be found: code=%d found=%d resp=%+v", code, resp.DocumentsFound, resp.Documents)
	}
	if !resp.Detection.RateLimited {
		t.Error("detection.rate_limited should be set")
	}
	for _, d := range resp.Documents {
		if !hasEvidence(d, "link_unverified:http_429") || d.Status != 0 {
			t.Errorf("challenged link must be link_unverified with no fabricated status: %+v", d)
		}
	}
}

// A footer link to a page that really is dead is NOT a document: not in
// documents[] (so not in domain_legal_doc_items / documents_present), reported in
// rejected_links, and it cannot make a verdict "comprehensive".
func TestHandlerDeadLinkIsRejectedNotListed(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Acme</title></head><body><footer><a href="/privacy">Privacy Policy</a> <a href="/terms">Terms</a> <a href="/gone">Cookie Policy</a></footer></body></html>`))
		case "/privacy":
			_, _ = w.Write([]byte(realPrivacyPage()))
		case "/terms":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("nope"))
		case "/gone":
			_, _ = w.Write([]byte(`<html><head><title>Page not found</title></head><body><h1>Page not found</h1></body></html>`))
		default:
			_, _ = w.Write([]byte(parkingStub))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, resp := scanJSON(t, srv.URL)
	if code != http.StatusOK {
		t.Fatalf("code=%d", code)
	}
	for _, d := range resp.Documents {
		if d.Type == DocTermsOfService || d.Type == DocCookiePolicy {
			t.Errorf("dead link listed as a document: %+v", d)
		}
	}
	got := map[DocType]bool{}
	for _, r := range resp.RejectedLinks {
		got[r.Type] = true
	}
	if !got[DocTermsOfService] || !got[DocCookiePolicy] {
		t.Errorf("rejected_links should name terms and cookie: %+v", resp.RejectedLinks)
	}
	if resp.Verdict != "sparse" {
		t.Errorf("verdict=%q want sparse (only privacy is real)", resp.Verdict)
	}
}

// The scan must ANSWER inside its budget. A homepage that answers but whose
// every other page hangs used to push the scan past DomainScope's 30 s limit.
func TestHandlerAnswersInsideBudget(t *testing.T) {
	allowLoopback(t)
	oldBudget, oldPhase := totalBudget, probePhaseTimeout
	totalBudget, probePhaseTimeout = 1500*time.Millisecond, 5*time.Second
	t.Cleanup(func() { totalBudget, probePhaseTimeout = oldBudget, oldPhase })
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><head><title>Acme</title></head><body><footer><a href="/privacy">Privacy Policy</a></footer></body></html>`))
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	start := time.Now()
	code, resp := scanJSON(t, srv.URL)
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("scan took %v, budget was 1.5s", el)
	}
	if code != http.StatusOK || resp.DocumentsFound != 1 {
		t.Fatalf("the unverifiable footer link should still be answered: code=%d found=%d", code, resp.DocumentsFound)
	}
	if !resp.Detection.BudgetExhausted {
		t.Error("detection.budget_exhausted should be set")
	}
}

type rtFunc func(*http.Request) (*http.Response, error)

func (f rtFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// amf-france.org: the apex presents a certificate valid only for www.<domain>
// (and the DomainScope target is the bare apex). One www / http fallback recovers
// those homepages; a timeout must NOT trigger further origins.
func TestFetchHomepageFallsBackToWWW(t *testing.T) {
	var seen []string
	var mu sync.Mutex
	client := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		seen = append(seen, r.URL.String())
		mu.Unlock()
		if r.URL.Host == "amf-france.org" {
			return nil, errors.New(`tls: failed to verify certificate: x509: certificate is valid for www.amf-france.org, not amf-france.org`)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody, Request: r}, nil
	})}
	u, _ := url.Parse("https://amf-france.org")
	hp, err := fetchHomepage(context.Background(), client, u)
	if err != nil || hp.fallback != "www" {
		t.Fatalf("want www fallback, got hp=%+v err=%v seen=%v", hp, err, seen)
	}

	seen = nil
	slow := &http.Client{Transport: rtFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		seen = append(seen, r.URL.String())
		mu.Unlock()
		return nil, fmt.Errorf("Get %q: dial tcp 1.2.3.4:443: i/o timeout", r.URL.String())
	})}
	if _, err := fetchHomepage(context.Background(), slow, u); err == nil {
		t.Fatal("expected an error")
	}
	if len(seen) != 1 {
		t.Errorf("a timeout must not be retried or fall back to other origins, saw %v", seen)
	}
}

func TestFetchKind(t *testing.T) {
	cases := map[string]string{
		"lookup x: no such host":                            "dns",
		"tls: failed to verify certificate: x509: foo":      "tls",
		"dial tcp 1.2.3.4:443: connect: connection refused": "refused",
		"dial tcp 1.2.3.4:443: i/o timeout":                 "timeout",
		"Get x: EOF":                                        "reset",
		"something else":                                    "other",
	}
	for in, want := range cases {
		if got := fetchKind(errors.New(in)); got != want {
			t.Errorf("fetchKind(%q)=%q want %q", in, got, want)
		}
	}
	if fetchKind(context.DeadlineExceeded) != "timeout" {
		t.Error("deadline -> timeout")
	}
}

// Overload is answered fast and honestly instead of queueing until the caller's
// own deadline fires.
func TestHandlerShedsLoadWhenSaturated(t *testing.T) {
	old := inflightSem
	oldWait := slotWait
	inflightSem = make(chan struct{}, 1)
	slotWait = 50 * time.Millisecond
	t.Cleanup(func() { inflightSem, slotWait = old, oldWait })
	inflightSem <- struct{}{} // saturated
	req := httptest.NewRequest(http.MethodGet, "/?target=https://example.com", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusServiceUnavailable || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("want 503 + Retry-After, got %d %v", rec.Code, rec.Header())
	}
	var resp Response
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Result != "error" || resp.Reason != "overloaded" {
		t.Errorf("want result=error reason=overloaded, got %+v", resp)
	}
}

// 1.8.x built two clients (each with its own *http.Transport and idle pool) per
// request and never closed them: 3 -> 724 goroutines after 40 requests. With one
// shared client the goroutine count stays bounded.
func TestNoGoroutineLeakAcrossRequests(t *testing.T) {
	allowLoopback(t)
	resetSharedClient()
	t.Cleanup(resetSharedClient)
	srv := e2eServer()
	defer srv.Close()
	runtime.GC()
	before := runtime.NumGoroutine()
	for i := 0; i < 40; i++ {
		if code, _ := scanJSON(t, srv.URL); code != http.StatusOK {
			t.Fatalf("code=%d", code)
		}
	}
	time.Sleep(300 * time.Millisecond)
	runtime.GC()
	if grew := runtime.NumGoroutine() - before; grew > 120 {
		t.Fatalf("goroutines grew by %d over 40 requests (1.8.x leaked ~680)", grew)
	}
}

// Identical bodies (a catch-all SPA answering every canonical probe with one
// shell) are analysed once.
func TestScanStateMemoisesBodies(t *testing.T) {
	st := newScanState("<html>home</html>")
	a := st.view("<html><head><title>x</title></head><body>shell</body></html>")
	b := st.view("<html><head><title>x</title></head><body>shell</body></html>")
	if a != b {
		t.Error("same body must return the same memoised view")
	}
	if st.sameAsHomepage("<html>home</html>") {
		t.Error("tiny bodies are not treated as homepage clones")
	}
	home := "<html><head><title>Home</title></head><body>" + strings.Repeat("content ", 200) + "</body></html>"
	st2 := newScanState(home)
	if !st2.sameAsHomepage(home) {
		t.Error("a probe that returns the homepage bytes carries no evidence")
	}
}

func TestProbePlanKeepsImprintAndCapsTotal(t *testing.T) {
	all := allCanonicalProbes(documentTypeOrder, maxProbes)
	if len(all) > maxProbes || len(all) == 0 {
		t.Fatalf("probe count %d out of range", len(all))
	}
	haveImprint, haveRefund := false, false
	for _, p := range all {
		if p.DocType == DocImprint {
			haveImprint = true
		}
		if p.DocType == DocRefundPolicy || p.DocType == DocShippingPolicy || p.DocType == DocSLA {
			haveRefund = true
		}
	}
	if !haveImprint {
		t.Error("imprint canonical paths were starved by ToS+privacy in 1.8.x; they must be probed")
	}
	if haveRefund {
		t.Error("refund/shipping/SLA are never found by probing; they must not be probed")
	}
}

// The set-based path matcher must accept exactly what the reference regexps
// (pathRE / pathREContains) accept.
func TestPathMatcherEquivalentToRegexps(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	segs := []string{"privacy", "privacy-policy", "Privacy.HTML", "privacy.php5", "privacy.policy.html", "terms", "terms-of-service.aspx", "tos", "agb.pdf",
		"cookie-settings", "blog", "my-cookies-page", "de", "en", "legal", "politique-confidentialite", "datenschutzerklärung", "informații-legale",
		"aviso-de-privacidad", "x", "", "terms-&-conditions", "impressum.htm", "dmca.asp", "sla", "SLA.PDF", "thermometer", "paralegal"}
	for i := 0; i < 3000; i++ {
		var parts []string
		for k, n := 0, 1+rng.Intn(4); k < n; k++ {
			parts = append(parts, segs[rng.Intn(len(segs))])
		}
		path := "/" + strings.Join(parts, "/")
		if rng.Intn(5) == 0 {
			path += "/"
		}
		for _, p := range patterns {
			want := matchPathOnly(path, p.pathPatterns)
			got := p.pathFast.match(path, pathSegments(path))
			if got != want {
				t.Fatalf("%s: path %q fast=%v regexps=%v", p.docType, path, got, want)
			}
		}
	}
}

// A redirect to a different host (google.vu -> www.google.com, eclektikvegan.com
// -> caligurllyfe.com) is reported so consumers can discount "documents" that
// belong to the redirect target.
func TestOffsiteRedirectIsReported(t *testing.T) {
	allowLoopback(t)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			http.Redirect(w, r, strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)+"/home", http.StatusFound)
		case "/home":
			_, _ = w.Write([]byte(`<html><head><title>Elsewhere</title></head><body><footer><a href="/privacy">Privacy Policy</a></footer></body></html>`))
		case "/privacy":
			_, _ = w.Write([]byte(realPrivacyPage()))
		}
	}))
	defer srv.Close()
	code, resp := scanJSON(t, srv.URL)
	if code != http.StatusOK || resp.Detection.OffsiteRedirect != "localhost" {
		t.Fatalf("want offsite_redirect=localhost, got code=%d %+v", code, resp.Detection)
	}
}

// bufaloscr.com answers EVERY path with the same SPA shell (differing only in a
// per-request nonce, so a byte hash cannot match). 1.8.x reported it as having
// ToS+privacy from the /tos and /privacy-policy probes; 1.9.0 first widened that
// to AUP/DMCA/imprint/cookies — the catch-all detector must reject all of them.
func TestHandlerRejectsCatchAllProbes(t *testing.T) {
	allowLoopback(t)
	var n int
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		nonce := n
		mu.Unlock()
		_, _ = w.Write([]byte(fmt.Sprintf(`<html><head><title>Los Bufalos Mojados</title><meta name="nonce" content="%d"></head><body><div id="app">`+
			strings.Repeat("Welcome to our restaurant. Liability and jurisdiction apply to nothing here. ", 40)+`</div></body></html>`, nonce)))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, resp := scanJSON(t, srv.URL)
	if code != http.StatusNotFound || resp.DocumentsFound != 0 {
		t.Fatalf("a catch-all SPA must yield NoData, got code=%d found=%d docs=%+v", code, resp.DocumentsFound, resp.Documents)
	}
	if resp.Detection.ProbesRejected == 0 {
		t.Error("probes_rejected_soft404 should count the rejected catch-all hits")
	}
}

// dict.cc answers every /<word> with a dictionary look-up titled "<word> |
// Übersetzung ...": /terms, /dmca and /cookie-policy "verified" as ToS / DMCA /
// cookie policy (title_matches_type for "dmca"). budongsam.com's "Copyright" link
// pointed at "/" and the homepage became a copyright policy.
func TestHandlerRejectsPathEchoTemplates(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		word := strings.Trim(r.URL.Path, "/")
		if word == "" {
			word = "home"
		}
		_, _ = w.Write([]byte(`<html><head><title>` + word + ` | Translation English-German</title></head><body><h1>` + word + `</h1>` +
			strings.Repeat("Translations for this word: liability, jurisdiction, warranty. ", 40) + `</body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, resp := scanJSON(t, srv.URL)
	if code != http.StatusNotFound || resp.DocumentsFound != 0 {
		t.Fatalf("path-echo template must yield NoData, got code=%d docs=%+v", code, resp.Documents)
	}
}

func TestRootLinkIsNeverALegalDocument(t *testing.T) {
	base, _ := url.Parse("https://www.example.com/")
	for _, h := range []string{
		`<footer><a href="/">Copyright</a></footer>`,
		`<footer><a href="https://www.example.com">Terms</a></footer>`,
		`<footer><a href="https://example.com/">Privacy Policy</a></footer>`,
	} {
		if hits := linkScan(h, base); len(hits) != 0 {
			t.Errorf("link to the site root must be ignored: %s -> %+v", h, hits)
		}
	}
}

// osmfoundation.org links its privacy policy as "Privacy Policy" to a wiki page;
// when the verification fetch fails the full-phrase anchor text is still real
// location evidence, whereas a bare-word in-page link is not.
func TestUnverifiedLinkWithFullPhraseTextIsKept(t *testing.T) {
	keep := DocFinding{Type: DocPrivacyPolicy, URL: "https://x.org/wiki/Privacy_policy", Source: "page_link", LinkText: "Privacy Policy",
		LinkConfidence: ConfLow, Confidence: ConfLow, Evidence: []string{"link_unverified:fetch_failed:floor=low"}}
	if isWeakUnconfirmed(keep) {
		t.Error("an unverified in-page link with the full phrase 'Privacy Policy' must be kept")
	}
	drop := keep
	drop.LinkText = "Privacy"
	if !isWeakUnconfirmed(drop) {
		t.Error("an unverified in-page link with only the bare word 'Privacy' (an article?) must stay weak")
	}
}

// businesstimesofspain.com: every /legal/* link answers 200 with Anubis'
// "Making sure you're not a bot!" page. The link is real, the page was never seen.
func TestChallengeBodyIsNotADocumentButKeepsTheLink(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(`<html><head><title>Times</title></head><body><footer><a href="/legal/terms">Terms &amp; Conditions</a> <a href="/legal/privacy">Privacy Policy</a></footer></body></html>`))
			return
		}
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Making sure you&#39;re not a bot!</title></head><body><h1>Making sure you&#39;re not a bot!</h1><p>Protected by Anubis</p></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	code, resp := scanJSON(t, srv.URL)
	if code != http.StatusOK || resp.DocumentsFound != 2 {
		t.Fatalf("challenged links must stay as unverified documents: code=%d docs=%+v", code, resp.Documents)
	}
	for _, d := range resp.Documents {
		if !hasEvidence(d, "link_unverified:bot_challenge") || d.Title != "" {
			t.Errorf("challenge page must not become the document's title/evidence: %+v", d)
		}
	}
	if vr := classifyBody(`<html><head><title>Making sure you're not a bot!</title></head><body>x</body></html>`, 200, DocTermsOfService); vr.IsReal {
		t.Error("a bot-challenge body is never a verified document")
	}
}
