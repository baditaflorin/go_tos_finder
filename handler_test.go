package main

import (
	"net/url"
	"strings"
	"testing"
	"time"
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

func TestLinkScanSkipsJunkSchemes(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	html := `<a href="mailto:foo@bar.com">terms of service</a>
<a href="javascript:void(0)">privacy</a>`
	hits := linkScan(html, base)
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %d: %+v", len(hits), hits)
	}
}

func TestValidateURLBlocksLoopback(t *testing.T) {
	u, _ := url.Parse("http://127.0.0.1/")
	if err := validateURL(u); err == nil {
		t.Fatal("expected loopback to be blocked")
	}
}

func TestValidateURLBlocksPrivate(t *testing.T) {
	u, _ := url.Parse("http://10.0.0.1/")
	if err := validateURL(u); err == nil {
		t.Fatal("expected private network to be blocked")
	}
}

func TestValidateURLBlocksScheme(t *testing.T) {
	u, _ := url.Parse("file:///etc/passwd")
	if err := validateURL(u); err == nil {
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
