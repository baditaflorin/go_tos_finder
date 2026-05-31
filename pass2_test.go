package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestClassifyLinkEvidenceFloor verifies the location-confidence floor derived
// purely from the link (path token + anchor text), independent of any body
// fetch. Dual-signal (path AND text agree) => medium floor; single-signal =>
// low; no match => none.
func TestClassifyLinkEvidenceFloor(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		text      string
		wantType  DocType
		wantFloor string
	}{
		{"path+text agree -> medium", "/legal/privacy", "Privacy Policy", DocPrivacyPolicy, ConfMedium},
		{"path only -> low", "/privacy", "More", DocPrivacyPolicy, ConfLow},
		{"text only -> low", "/info", "Terms of Service", DocTermsOfService, ConfLow},
		{"tos path+text agree -> medium", "/terms", "Terms & Conditions", DocTermsOfService, ConfMedium},
		{"cookie path+text agree -> medium", "/cookie-policy", "Cookie Policy", DocCookiePolicy, ConfMedium},
		{"impressum path+text agree -> medium", "/impressum", "Impressum", DocImprint, ConfMedium},
		{"no match -> none", "/about", "About Us", "", ConfNone},
		{"hub -> low even if both", "/legal", "Legal", DocLegalHub, ConfLow},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dt, _, floor := classifyLinkEvidence(c.path, c.text)
			if dt != c.wantType {
				t.Errorf("type: got %q want %q", dt, c.wantType)
			}
			if floor != c.wantFloor {
				t.Errorf("floor: got %q want %q", floor, c.wantFloor)
			}
		})
	}
}

// TestClassifyLinkBackCompat: the legacy two-return classifyLink still behaves.
func TestClassifyLinkBackCompat(t *testing.T) {
	dt, how := classifyLink("/legal/privacy", "Privacy Policy")
	if dt != DocPrivacyPolicy || how != "url_path" {
		t.Errorf("got %q/%q want privacy_policy/url_path", dt, how)
	}
}

// TestBodyHasTypeSignal: the false-positive guard for canonical probes. A body
// with type-specific vocabulary (or generic legal vocab) passes; an unrelated
// page fails.
func TestBodyHasTypeSignal(t *testing.T) {
	cases := []struct {
		name string
		body string
		want DocType
		ok   bool
	}{
		{"shipping page has shipping vocab", "Your order will ship via carrier within 3 days. Delivery info below.", DocShippingPolicy, true},
		{"refund page has refund vocab", "We offer a 30-day money-back refund on all returns.", DocRefundPolicy, true},
		{"dmca page has dmca vocab", "Submit a DMCA takedown notice to our copyright agent.", DocDMCA, true},
		{"imprint has impressum vocab", "Impressum: Acme GmbH, Handelsregister HRB 12345, Geschäftsführer Max.", DocImprint, true},
		// A generic marketing/landing page that 200s for a /shipping probe on a
		// site that sells nothing — must NOT pass for shipping.
		{"marketing page not shipping", "Welcome to our amazing video platform. Watch and share.", DocShippingPolicy, false},
		{"marketing page not refund", "Discover thousands of creators on our community video site.", DocRefundPolicy, false},
		// Generic legal vocab corroborates any legal doc type.
		{"legal vocab corroborates", "This agreement is governed by law. Last updated January 2026.", DocSLA, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bodyHasTypeSignal(c.body, c.want); got != c.ok {
				t.Errorf("bodyHasTypeSignal(%s)=%v want %v", c.want, got, c.ok)
			}
		})
	}
}

// TestProbeRejectsNicheFalsePositive: end-to-end, a site whose homepage has no
// legal links and serves a large GENERIC page (no type vocab) on the
// /shipping and /refunds canonical probes must NOT report shipping/refund docs
// — the false-positive guard rejects them — while a real /privacy page with
// type vocab is still detected via probe.
func TestProbeRejectsNicheFalsePositive(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	// A large generic page (clears the 600-byte soft-404 gate, no legal vocab,
	// no type-specific vocab). Returned for /shipping, /refunds, /returns, etc.
	genericLarge := `<html><head><title>Catalog</title></head><body>` +
		strings.Repeat("Browse our wonderful selection of inspirational quotes and wallpapers. ", 30) +
		`</body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<html><head><title>Quotes</title></head><body>no legal links</body></html>`))
		case "/privacy":
			// Real privacy doc — type vocab present, title matches.
			_, _ = w.Write([]byte(`<html><head><title>Privacy Policy</title></head><body><h1>Privacy Policy</h1>` +
				strings.Repeat("We collect and process your personal data. ", 30) + `</body></html>`))
		default:
			// Every other probed canonical path (/shipping, /refunds, /terms, ...)
			// returns this large generic page with NO type vocabulary.
			_, _ = w.Write([]byte(genericLarge))
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

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

	// Privacy: real type vocab + title match -> detected, high confidence.
	priv, ok := byType[DocPrivacyPolicy]
	if !ok {
		t.Fatalf("real /privacy should be detected via probe; docs=%+v", resp.Documents)
	}
	if priv.Confidence != ConfHigh {
		t.Errorf("probed privacy with type vocab should be high, got %q", priv.Confidence)
	}

	// Niche generic 200s must be rejected, not reported as documents.
	for _, bad := range []DocType{DocShippingPolicy, DocRefundPolicy, DocTermsOfService, DocCookiePolicy} {
		if d, ok := byType[bad]; ok {
			t.Errorf("%s should be rejected (generic page, no type vocab); got conf=%q src=%q", bad, d.Confidence, d.Source)
		}
	}
	if resp.Detection.ProbesRejected == 0 {
		t.Error("expected generic-page probes to be rejected")
	}
}

// TestLinkFloorSurvivesUnverifiableTarget: a footer link whose path AND anchor
// text agree (medium floor) must hold medium confidence even when the link
// target cannot be fetched (the production proxy-timeout case). A single-signal
// footer link stays low.
func TestLinkFloorSurvivesUnverifiableTarget(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	// Homepage footer: /privacy with corroborating text "Privacy Policy"
	// (medium floor) and /terms with NON-corroborating text "More" (low floor,
	// path-only). The targets themselves are served as connection resets so the
	// Phase-3 verification GET fails — exercising the floor fallback.
	home := `<html><head><title>Site</title></head><body><footer>
<a href="/privacy">Privacy Policy</a>
<a href="/terms">More</a>
</footer></body></html>`
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(home))
		default:
			// Hijack and abruptly close to force a fetch error on the target.
			hj, ok := w.(http.Hijacker)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			conn, _, err := hj.Hijack()
			if err == nil {
				_ = conn.Close()
			}
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
		t.Fatal("privacy footer link should be recorded even when target unfetchable")
	}
	if priv.Confidence != ConfMedium {
		t.Errorf("path+text-corroborated link should hold medium floor on fetch failure, got %q (ev=%v)", priv.Confidence, priv.Evidence)
	}
	terms, ok := byType[DocTermsOfService]
	if !ok {
		t.Fatal("terms footer link should be recorded")
	}
	if terms.Confidence != ConfLow {
		t.Errorf("single-signal (path-only) link should stay low on fetch failure, got %q", terms.Confidence)
	}
}
