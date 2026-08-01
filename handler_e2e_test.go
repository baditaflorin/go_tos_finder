package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/baditaflorin/go-common/safehttp"
)

// allowLoopback lets httptest's 127.0.0.1 server pass the safehttp SSRF guard
// for the duration of a test. The allowlist is parsed once at package init from
// SAFEHTTP_ALLOW_PRIVATE_IPS, so t.Setenv would not take effect — we mutate it
// directly and restore the (empty) default on cleanup.
func allowLoopback(t *testing.T) {
	t.Helper()
	safehttp.SetAllowedPrivateIPs([]net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback})
	t.Cleanup(func() { safehttp.SetAllowedPrivateIPs(nil) })
}

// realPrivacyPage / realTermsPage are substantial, type-titled documents.
func realPrivacyPage() string {
	return `<!DOCTYPE html><html><head><title>Privacy Policy</title></head><body>
<h1>Privacy Policy</h1><p>Effective date: 2026-01-01. This policy describes how we collect and process your personal data.</p>` +
		strings.Repeat("We use your information to provide the service. ", 30) + `</body></html>`
}
func realTermsPage() string {
	return `<!DOCTYPE html><html><head><title>Terms of Service</title></head><body>
<h1>Terms of Service</h1><p>These terms are governed by law and subject to arbitration.</p>` +
		strings.Repeat("By using the service you agree to these terms. ", 30) + `</body></html>`
}

// e2eServer serves a homepage whose footer links to /privacy and /terms, has a
// real /cookies page, and returns a 114-byte parking stub for everything else
// (the production soft-404 pattern) so canonical probes for /dmca, /aup, etc.
// must be rejected.
func e2eServer() *httptest.Server {
	mux := http.NewServeMux()
	home := `<!DOCTYPE html><html><head><title>Acme</title></head><body>
<main>welcome</main>
<footer>
  <a href="/privacy">Privacy Policy</a>
  <a href="/terms">Terms of Service</a>
  <a href="/cookies">Cookie Policy</a>
  <a href="/legal">Legal</a>
</footer></body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(home))
		case "/privacy":
			_, _ = w.Write([]byte(realPrivacyPage()))
		case "/terms":
			_, _ = w.Write([]byte(realTermsPage()))
		case "/cookies":
			_, _ = w.Write([]byte(`<html><head><title>Cookie Policy</title></head><body><h1>Cookie Policy</h1>` +
				strings.Repeat("This cookie policy explains how we use cookies. ", 20) + `</body></html>`))
		default:
			// Soft-404 parking stub on ALL probed canonical paths (/dmca, /aup, ...).
			_, _ = w.Write([]byte(parkingStub))
		}
	})
	return httptest.NewServer(mux)
}

func TestHandlerEndToEnd(t *testing.T) {
	allowLoopback(t) // let httptest loopback through the safehttp SSRF guard
	srv := e2eServer()
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/?target="+srv.URL, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rec.Body.String())
	}

	// Success body must carry the standardized top-level result:"ok" (additive;
	// the rest of the shape is unchanged) so domainscope can read the outcome.
	if resp.Result != "ok" {
		t.Errorf("success body must carry result:\"ok\", got %q", resp.Result)
	}
	if resp.Status != "ok" {
		t.Errorf("success status should be ok, got %q", resp.Status)
	}

	byType := map[DocType]DocFinding{}
	for _, d := range resp.Documents {
		byType[d.Type] = d
	}

	// Privacy + Terms + Cookies discovered via footer links and verified.
	for _, want := range []DocType{DocPrivacyPolicy, DocTermsOfService, DocCookiePolicy} {
		d, ok := byType[want]
		if !ok {
			t.Errorf("expected %s in documents", want)
			continue
		}
		if confidenceRank(d.Confidence) < confidenceRank(ConfMedium) {
			t.Errorf("%s expected >= medium confidence, got %q", want, d.Confidence)
		}
	}

	// The /legal hub link should be recorded but NOT counted as a document.
	if _, ok := byType[DocLegalHub]; !ok {
		t.Error("expected legal_hub to be recorded from footer link")
	}

	// Canonical probes for /dmca, /aup, /copyright (soft-404 parking stubs)
	// MUST be rejected, not counted.
	for _, bad := range []DocType{DocDMCA, DocAcceptableUse, DocCopyrightPolicy} {
		if _, ok := byType[bad]; ok {
			t.Errorf("%s should NOT be detected (it is a soft-404 parking stub)", bad)
		}
	}
	if resp.Detection.ProbesRejected == 0 {
		t.Error("expected at least one probe rejected as soft-404")
	}

	// documents_found counts only real docs (privacy, terms, cookies) — hubs
	// and rejected probes excluded.
	if resp.DocumentsFound != 3 {
		t.Errorf("documents_found=%d, want 3 (privacy/terms/cookies)", resp.DocumentsFound)
	}
	if resp.Verdict != "comprehensive" {
		t.Errorf("verdict=%q, want comprehensive (ToS+Privacy+1)", resp.Verdict)
	}
	if !resp.Detection.HomepageFetched {
		t.Error("detection.homepage_fetched should be true")
	}
}

// TestFallbackClientSendsRealUserAgent guards against a real regression:
// fleetfetch.NewHTTPClient's RoundTripper deliberately strips the caller's
// per-request User-Agent before handing headers to its direct-fetch fallback
// path (see fallbackClient's doc comment) — a real cache-hit-rate win when
// the fetch is cache-mediated, but with nothing else to set a UA on the
// fallback path itself. In this test environment (as in local/dev generally,
// and in production whenever the shared fetch cache is unreachable or slow)
// the fleet fetch cache is not reachable, so every request the handler makes
// already exercises exactly this fallback path — making it the right place
// to pin the fix: the origin must see this service's own UA, not Go's bare
// default ("Go-http-client/1.1"), which real-world WAFs (verified live
// against chronicpies.com, a production sample domain) are far more likely
// to 403/block than a UA that honestly identifies the caller.
func TestFallbackClientSendsRealUserAgent(t *testing.T) {
	allowLoopback(t)
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Acme</title></head><body>hi</body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/?target="+srv.URL, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if gotUA == "" {
		t.Fatal("origin received no User-Agent header at all")
	}
	if !strings.Contains(gotUA, "go_tos_finder") {
		t.Errorf("origin received User-Agent %q, want it to identify this service (contain %q) — "+
			"the fallback-path UA-stripping regression this test guards against", gotUA, "go_tos_finder")
	}
}

// TestHandlerProbedRealDoc: no footer links, but the canonical /privacy and
// /terms paths serve real documents — they must be found via verified probe.
func TestHandlerProbedRealDoc(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Bare</title></head><body>no footer links here</body></html>`))
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

	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	byType := map[DocType]DocFinding{}
	for _, d := range resp.Documents {
		byType[d.Type] = d
	}
	priv, ok := byType[DocPrivacyPolicy]
	if !ok {
		t.Fatal("privacy should be found via verified probe")
	}
	if priv.Source != "probed_path" {
		t.Errorf("privacy source=%q want probed_path", priv.Source)
	}
	if priv.Confidence != ConfHigh {
		t.Errorf("probed privacy with matching title should be high, got %q", priv.Confidence)
	}
	if _, ok := byType[DocTermsOfService]; !ok {
		t.Error("terms should be found via verified probe")
	}
}
