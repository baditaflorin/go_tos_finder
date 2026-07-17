package main

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/baditaflorin/go-common/safehttp"
)

func TestClassifyLinkByPath(t *testing.T) {
	cases := []struct {
		path string
		want DocType
	}{
		{"/privacy", DocPrivacyPolicy},
		{"/privacy-policy", DocPrivacyPolicy},
		{"/datenschutz", DocPrivacyPolicy},
		{"/legal/terms", DocTermsOfService},
		{"/tos", DocTermsOfService},
		{"/agb", DocTermsOfService},
		{"/cookies", DocCookiePolicy},
		{"/dmca", DocDMCA},
		{"/aup", DocAcceptableUse},
		{"/impressum", DocImprint},
		{"/sla", DocSLA},
		{"/sub-processors", DocSubprocessors},
		{"/about", ""},
		{"/", ""},
	}
	for _, c := range cases {
		got, _ := classifyLink(c.path, "")
		if got != c.want {
			t.Errorf("classifyLink(%q): got %q want %q", c.path, got, c.want)
		}
	}
}

func TestClassifyLinkByText(t *testing.T) {
	cases := []struct {
		text string
		want DocType
	}{
		{"Terms of Service", DocTermsOfService},
		{"Privacy Policy", DocPrivacyPolicy},
		{"Cookie Policy", DocCookiePolicy},
		{"Impressum", DocImprint},
		{"Acceptable Use", DocAcceptableUse},
		{"DMCA", DocDMCA},
		{"Sub-processors", DocSubprocessors},
	}
	for _, c := range cases {
		got, _ := classifyLink("/unrelated-path", c.text)
		if got != c.want {
			t.Errorf("classifyLink text %q: got %q want %q", c.text, got, c.want)
		}
	}
}

func TestLinkScanPrefersFooter(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	html := `<html><body>
<header><a href="/legal/privacy">Privacy</a></header>
<main>content</main>
<footer><a href="/privacy">Privacy</a><a href="/terms">Terms</a></footer>
</body></html>`
	hits := linkScan(html, base)
	priv, ok := hits[DocPrivacyPolicy]
	if !ok {
		t.Fatal("privacy not found")
	}
	if priv.Source != "footer_link" {
		t.Errorf("expected footer_link, got %s url=%s", priv.Source, priv.URL)
	}
	if !strings.HasSuffix(priv.URL, "/privacy") {
		t.Errorf("expected /privacy, got %s", priv.URL)
	}
	if _, ok := hits[DocTermsOfService]; !ok {
		t.Fatal("terms not found")
	}
}

func TestLinkScanRelativeURL(t *testing.T) {
	base, _ := url.Parse("https://example.com/about")
	html := `<a href="/legal/privacy">Datenschutz</a>`
	hits := linkScan(html, base)
	priv := hits[DocPrivacyPolicy]
	if priv.URL != "https://example.com/legal/privacy" {
		t.Errorf("got %q", priv.URL)
	}
}

func TestLinkScanRejectsThirdPartyLegalPage(t *testing.T) {
	base, _ := url.Parse("https://shop.example.com/")
	hits := linkScan(`<footer><a href="https://platform.example/privacy">Privacy</a><a href="https://privacy.shop.example.com/policy">Privacy</a></footer>`, base)
	got, ok := hits[DocPrivacyPolicy]
	if !ok || got.URL != "https://privacy.shop.example.com/policy" {
		t.Fatalf("want same-site policy only, got %#v", hits)
	}
}

func TestLinkScanSkipsJunkSchemes(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	html := `<a href="mailto:foo@bar.com">terms of service</a>
<a href="javascript:void(0)">privacy</a>`
	hits := linkScan(html, base)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d: %+v", len(hits), hits)
	}
}

func TestCheckURLBlocksLoopback(t *testing.T) {
	_, err := safehttp.CheckURL(context.Background(), "http://127.0.0.1/")
	if err == nil {
		t.Fatal("expected loopback to be blocked")
	}
}

func TestCheckURLBlocksPrivate(t *testing.T) {
	_, err := safehttp.CheckURL(context.Background(), "http://10.0.0.1/")
	if err == nil {
		t.Fatal("expected private network to be blocked")
	}
}

func TestCheckURLBlocksScheme(t *testing.T) {
	_, err := safehttp.CheckURL(context.Background(), "file:///etc/passwd")
	if err == nil {
		t.Fatal("expected file scheme to be blocked")
	}
}

func TestStalenessOf(t *testing.T) {
	now := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	// 1 year old -> current
	s, d := stalenessOf("Wed, 15 Mar 2025 10:00:00 GMT", now)
	if s != "current" || d != "2025-03-15" {
		t.Errorf("got s=%q d=%q", s, d)
	}
	// 3 years old -> outdated
	s, d = stalenessOf("Mon, 01 Sep 2022 10:00:00 GMT", now)
	if s != "outdated" {
		t.Errorf("expected outdated, got %q (date=%s)", s, d)
	}
	// Empty / unparseable
	if s, _ := stalenessOf("", now); s != "" {
		t.Errorf("expected empty staleness")
	}
	if s, _ := stalenessOf("not a date", now); s != "" {
		t.Errorf("expected empty staleness for garbage")
	}
}

func TestVerdictOf(t *testing.T) {
	cases := []struct {
		found   int
		missing []DocType
		want    string
	}{
		{6, []DocType{}, "comprehensive"},
		{3, []DocType{DocAcceptableUse, DocDMCA, DocCookiePolicy}, "comprehensive"}, // ToS+Privacy+1
		{2, []DocType{DocCookiePolicy, DocAcceptableUse, DocDMCA}, "comprehensive"},
		{2, []DocType{DocTermsOfService, DocAcceptableUse, DocDMCA}, "partial"}, // no ToS
		{1, []DocType{DocTermsOfService, DocPrivacyPolicy, DocCookiePolicy, DocAcceptableUse, DocDMCA}, "sparse"},
		{0, []DocType{DocTermsOfService, DocPrivacyPolicy, DocCookiePolicy, DocAcceptableUse, DocDMCA}, "none"},
	}
	for _, c := range cases {
		got := verdictOf(c.found, c.missing)
		if got != c.want {
			t.Errorf("verdictOf(%d,%v): got %q want %q", c.found, c.missing, got, c.want)
		}
	}
}

func TestExtractAnchorsBasic(t *testing.T) {
	html := `<a href="/a">A</a><a href='/b' class=x>B</a><a href=/c>C</a>`
	a := extractAnchors(html)
	if len(a) != 3 {
		t.Fatalf("got %d anchors: %+v", len(a), a)
	}
	if a[0].Href != "/a" || a[1].Href != "/b" || a[2].Href != "/c" {
		t.Errorf("hrefs: %+v", a)
	}
}

func TestFooterBoundsExplicit(t *testing.T) {
	html := `<html><body><p>hi</p><footer><a>x</a></footer></body></html>`
	start, end := footerBounds(html)
	if start <= 0 || end <= start {
		t.Errorf("bad bounds %d %d", start, end)
	}
	if !strings.HasPrefix(strings.ToLower(html[start:end]), "<footer") {
		t.Errorf("footer not located: %q", html[start:end])
	}
}

func TestAllCanonicalProbesCapped(t *testing.T) {
	probes := allCanonicalProbes(documentTypeOrder, 7)
	if len(probes) != 7 {
		t.Errorf("expected 7 probes, got %d", len(probes))
	}
}

// TestCheckURLRetryRejectsBlockedWithoutRetry verifies a definitive policy
// reject (private-IP SSRF block) returns immediately on the first attempt —
// it must never be retried, since retrying would not change the verdict and
// would only add latency.
func TestCheckURLRetryRejectsBlockedWithoutRetry(t *testing.T) {
	start := time.Now()
	_, err := checkURLRetry(context.Background(), "http://127.0.0.1/")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatalf("expected an error for a private-IP target")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Errorf("expected an ErrBlocked-flavoured error, got %v", err)
	}
	// A retry would sleep >=400ms; a single-attempt reject should be near-instant.
	if elapsed > 300*time.Millisecond {
		t.Errorf("policy reject took %v — looks like it was retried", elapsed)
	}
}

// TestCheckURLRetryRespectsContextCancellation verifies that a context which
// is already done short-circuits the retry loop instead of blocking forever
// or panicking on a nil deref.
func TestCheckURLRetryRespectsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := checkURLRetry(ctx, "https://example.invalid/")
	if err == nil {
		t.Fatalf("expected an error with an already-cancelled context")
	}
}

// TestCheckURLRetrySucceedsOnValidURL is a smoke test that the happy path
// (a syntactically valid, non-blocked URL) still returns cleanly through the
// new wrapper — i.e. checkURLRetry is behaviourally transparent for the
// common case and only changes behavior on a transient DNS failure.
func TestCheckURLRetrySucceedsOnValidURL(t *testing.T) {
	u, err := checkURLRetry(context.Background(), "https://example.com/")
	if err != nil {
		var zero url.URL
		if u != nil && *u != zero {
			t.Fatalf("unexpected error for a well-formed public URL: %v", err)
		}
		// A DNS hiccup in this sandbox is acceptable (matches production
		// reality); just make sure we didn't panic and got *an* answer.
		t.Skipf("network unavailable in test sandbox: %v", err)
	}
	if u == nil || u.Hostname() != "example.com" {
		t.Errorf("expected resolved URL for example.com, got %+v", u)
	}
}
