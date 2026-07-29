package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestIsHomepageBlockedRealSignatures reproduces the exact interstitial
// vocabulary captured from REAL production domains during a live 1000-host
// sample run against the deployed 1.3.5 build (2026-07-29): 15/202 "no_data"
// verdicts re-fetched live turned out to be bot-block/WAF-challenge pages, not
// genuinely policy-free sites. Before this fix, all of these were fed straight
// into linkScan and recorded as verdict:"none" — indistinguishable from a real
// negative.
func TestIsHomepageBlockedRealSignatures(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
	}{
		{
			// herogames.com, 2026-07-29, HTTP 403 — Cloudflare JS challenge.
			name: "cloudflare_js_challenge",
			body: `<!DOCTYPE html><html><head><title>Just a moment...</title></head>
<body><div class="main-wrapper"><noscript><div class="h2"><span>Enable JavaScript and cookies to continue</span></div></noscript></div></body></html>`,
			status: 403,
		},
		{
			// photovoltaik-info.eu / gcvcc.org / caseycavanagh.tech, 2026-07-29,
			// HTTP 403 — stock Apache 403 stub.
			name: "apache_403_stub",
			body: `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01//EN" "http://www.w3.org/TR/html4/strict.dtd">
<html><head><title>403 Forbidden</title></head><body><h1>Forbidden</h1>
<p>You don't have permission to access this resource.</p>
<hr><address>Apache Server at example.com Port 443</address></body></html>`,
			status: 403,
		},
		{
			name:   "generic_waf_access_denied",
			body:   `<html><head><title>Access Denied</title></head><body><h1>Access Denied</h1><p>Request Rejected. Your request has been blocked by our security systems.</p></body></html>`,
			status: 403,
		},
		{
			name:   "incapsula_incident",
			body:   `<html><head><title>Request unsuccessful</title></head><body>Incapsula incident ID: 123-456-789</body></html>`,
			status: 403,
		},
		{
			// esc-computer.ch, 2026-07-29, HTTP 200 with a zero-byte body.
			name:   "empty_200",
			body:   ``,
			status: 200,
		},
		{
			// marcone.com, 2026-07-29, HTTP 403 with a 41-byte stub.
			name:   "tiny_stub",
			body:   `Request blocked.`,
			status: 403,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			blocked, reason := isHomepageBlocked(c.body, c.status)
			if !blocked {
				t.Fatalf("expected %q to be classified as blocked", c.name)
			}
			if reason == "" {
				t.Error("expected a non-empty reason token")
			}
		})
	}
}

// TestIsHomepageBlockedDoesNotFalsePositive guards against over-firing: a
// real, substantial homepage — including one that happens to mention
// "access" or "denied" in ordinary prose, or a minimal-but-real personal
// homepage — must never be misclassified as blocked.
func TestIsHomepageBlockedDoesNotFalsePositive(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
	}{
		{
			name: "real_homepage_mentions_access_control",
			body: `<html><head><title>Acme Security Consulting</title></head><body>
<h1>Welcome to Acme</h1><p>We help you design access control policies and manage denied login attempts for your enterprise.</p>` +
				strings.Repeat("Our consultants provide network security services. ", 20) + `</body></html>`,
			status: 200,
		},
		{
			name:   "minimal_real_personal_page",
			body:   `<html><head><title>Jane Doe</title></head><body><h1>Jane Doe — Photographer</h1><p>Welcome to my portfolio.</p></body></html>`,
			status: 200,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if blocked, reason := isHomepageBlocked(c.body, c.status); blocked {
				t.Errorf("%q should not be classified as blocked, got reason=%q", c.name, reason)
			}
		})
	}
}

// TestHandlerHomepageBlockedE2E is the full end-to-end reproduction: a target
// whose homepage is a Cloudflare-style challenge page must be reported as
// status:"blocked" / result:"error" / verdict:"unknown" (HTTP 502) — NOT as a
// false verdict:"none" (HTTP 404 no_data) claiming the site has no legal
// documents when we never actually saw the site.
func TestHandlerHomepageBlockedE2E(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head><title>Just a moment...</title></head>
<body><noscript><span>Enable JavaScript and cookies to continue</span></noscript></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/?target="+srv.URL, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected HTTP 502 for a blocked homepage, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rec.Body.String())
	}
	if resp.Status != StatusBlocked {
		t.Errorf("status=%q, want %q", resp.Status, StatusBlocked)
	}
	if resp.Result != "error" {
		t.Errorf("result=%q, want error", resp.Result)
	}
	if resp.Verdict != "unknown" {
		t.Errorf("verdict=%q, want unknown (must NOT be a false 'none')", resp.Verdict)
	}
	if len(resp.Documents) != 0 {
		t.Errorf("blocked homepage should carry no fabricated documents, got %+v", resp.Documents)
	}
	if resp.Reason == "" {
		t.Error("expected a non-empty machine-readable reason")
	}
}

// TestHandlerRealSmallHomepageStillWorks: a genuinely tiny but real homepage
// with real footer links must NOT be caught by the empty-body guard — proves
// minHomepageBytes / isHomepageBlocked don't regress the common small-site
// case.
func TestHandlerRealSmallHomepageStillWorks(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	home := `<html><head><title>Acme</title></head><body><footer>
<a href="/privacy">Privacy Policy</a><a href="/terms">Terms</a>
</footer></body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(home))
		case "/privacy":
			_, _ = w.Write([]byte(realPrivacyPage()))
		case "/terms":
			_, _ = w.Write([]byte(realTermsPage()))
		default:
			_, _ = w.Write([]byte(parkingStub))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/?target="+srv.URL, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if resp.Status == StatusBlocked {
		t.Error("a genuine small real homepage must not be classified as blocked")
	}
}
