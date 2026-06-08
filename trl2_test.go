package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/baditaflorin/go-common/safehttp"
)

// genericCatchAll is a large, real page that contains NO type-specific legal
// vocabulary — the production "SPA / wildcard catch-all" shape that earned a
// spurious page_exists_unconfirmed finding (cloudflare's 400KB /trademark etc).
func genericCatchAll() string {
	return `<!DOCTYPE html><html><head><title>Acme Home</title></head><body>` +
		strings.Repeat("Welcome to Acme. We build great products for everyone. ", 200) +
		`</body></html>`
}

// TestWeakCatchAllProbeDropped: a SINGLE-SIGNAL footer link (path matches but
// anchor text does not corroborate → low floor) whose target resolves to a
// generic catch-all (real 2xx, no type vocabulary, only page_exists_unconfirmed)
// must NOT be claimed as a document — this is the headline FP fix. A
// path+text-corroborated link (medium floor) would legitimately be kept, since
// the link itself is location evidence the site references that doc.
func TestWeakCatchAllProbeDropped(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	// Anchor text "More info" does NOT corroborate the /copyright path → low
	// link floor; the target is a generic catch-all → page_exists_unconfirmed.
	home := `<html><head><title>Acme</title></head><body><footer>
<a href="/copyright">More info</a>
</footer></body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(home))
			return
		}
		_, _ = w.Write([]byte(genericCatchAll())) // large generic page, 200
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := doHandler(t, srv.URL)
	for _, d := range resp.Documents {
		if d.Type == DocCopyrightPolicy {
			t.Errorf("single-signal copyright link to a generic catch-all must be dropped, got %+v", d)
		}
	}
	if resp.DocumentsFound != 0 {
		t.Errorf("documents_found=%d, want 0 (catch-all is not a real doc)", resp.DocumentsFound)
	}
	if resp.Status != StatusOK {
		t.Errorf("status=%q, want ok", resp.Status)
	}
}

// TestSoftFourОhFourLinkNotCounted: a footer link whose target is a soft-404
// must be listed but NOT counted (claiming a doc that 404s is a FP).
func TestSoftFourOhFourLinkNotCounted(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	home := `<html><head><title>Acme</title></head><body><footer>
<a href="/terms">Terms of Service</a>
</footer></body></html>`
	parking := `<!DOCTYPE html><html><head><title>404 Not Found</title></head><body>Page not found</body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte(home))
			return
		}
		_, _ = w.Write([]byte(parking)) // soft-404: 200 but "not found"
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := doHandler(t, srv.URL)
	if resp.DocumentsFound != 0 {
		t.Errorf("documents_found=%d, want 0 (terms target is a soft-404)", resp.DocumentsFound)
	}
}

// TestUnreachableStatusOnFetchFail: when the homepage cannot be fetched, the
// service must report status=unreachable at HTTP 200, not a 5xx service error.
func TestUnreachableStatusOnFetchFail(t *testing.T) {
	allowLoopback(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if conn, _, err := hj.Hijack(); err == nil {
			_ = conn.Close() // abrupt reset → fetch error
		}
	}))
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/?url="+srv.URL, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unreachable target must return HTTP 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != StatusUnreachable {
		t.Errorf("status=%q, want unreachable (reason=%q err=%q)", resp.Status, resp.Reason, resp.Error)
	}
	if resp.Reason == "" {
		t.Error("expected a machine-readable reason for unreachable target")
	}
}

// TestUnreachableCheckErrorClassification: a DNS-style CheckURL failure is
// unreachable; an SSRF/scheme reject is a real bad request.
func TestUnreachableCheckErrorClassification(t *testing.T) {
	if !isUnreachableCheckError(net.UnknownNetworkError("no such host: lookup foo")) {
		t.Error("a DNS-style CheckURL error should classify as unreachable")
	}
	if isUnreachableCheckError(safehttp.ErrBlocked) {
		t.Error("ErrBlocked (private IP) is a policy reject, not unreachable")
	}
	if isUnreachableCheckError(safehttp.ErrInvalidScheme) {
		t.Error("ErrInvalidScheme is a policy reject, not unreachable")
	}
}

// TestRetryRecoversTransient: a homepage that 5xx's once then succeeds must be
// recovered by the retry path (real-data recovery, not a false upstream_error).
func TestRetryRecoversTransient(t *testing.T) {
	allowLoopback(t)
	var calls int32
	good := `<html><head><title>Acme</title></head><body><footer>
<a href="/legal/privacy">Privacy Policy</a>
<a href="/legal/terms">Terms of Service</a>
</footer></body></html>`
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if atomic.AddInt32(&calls, 1) == 1 {
				w.WriteHeader(http.StatusBadGateway) // transient 5xx first
				return
			}
			_, _ = w.Write([]byte(good))
			return
		}
		// real doc targets
		switch {
		case strings.Contains(r.URL.Path, "privacy"):
			_, _ = w.Write([]byte(`<html><head><title>Privacy Policy</title></head><body><h1>Privacy Policy</h1>` +
				strings.Repeat("We collect and process your personal data. ", 30) + `</body></html>`))
		case strings.Contains(r.URL.Path, "terms"):
			_, _ = w.Write([]byte(`<html><head><title>Terms of Service</title></head><body><h1>Terms of Service</h1>` +
				strings.Repeat("By using the service you agree to these terms. ", 30) + `</body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp := doHandler(t, srv.URL)
	if resp.Status != StatusOK {
		t.Fatalf("status=%q want ok after retry (err=%q)", resp.Status, resp.Error)
	}
	if !resp.Detection.HomepageFetched {
		t.Fatal("homepage should have been fetched on retry")
	}
	if resp.DocumentsFound < 2 {
		t.Errorf("documents_found=%d, want >=2 (privacy+terms) after transient retry", resp.DocumentsFound)
	}
	if atomic.LoadInt32(&calls) < 2 {
		t.Errorf("homepage was fetched %d times, expected a retry (>=2)", calls)
	}
}

// TestSelftestFixtureDiscovers: the in-process classify check the selftest
// suite runs must find both ToS and Privacy in the fixture (proves /selftest
// exercises real discovery logic, not a static 200).
func TestSelftestFixtureDiscovers(t *testing.T) {
	_ = newSelftest(serviceName, Version) // suite builds without panic
	base, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	hits := linkScan(selftestFixture, base)
	if _, ok := hits[DocTermsOfService]; !ok {
		t.Error("selftest fixture should yield terms_of_service")
	}
	if _, ok := hits[DocPrivacyPolicy]; !ok {
		t.Error("selftest fixture should yield privacy_policy")
	}
}

// doHandler runs Handler against target and returns the parsed Response,
// failing the test on a non-200 or bad JSON.
func doHandler(t *testing.T, target string) Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/?url="+target, nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rec.Body.String())
	}
	return resp
}
