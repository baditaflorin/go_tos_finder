package main

import (
	"strings"
	"testing"
)

// The production soft-404 fingerprint: a 200 with a ~114-byte JS-redirect stub
// that bounces to a parking lander. ~528k of these were being counted as real
// documents at v1.1.0. classifyBody must reject them.
const parkingStub = `<!DOCTYPE html><html><head><script>window.onload=function(){window.location.href="/lander"}</script></head></html>`

func TestClassifyBodyRejectsParkingStub(t *testing.T) {
	vr := classifyBody(parkingStub, 200, DocDMCA)
	if vr.IsReal {
		t.Fatalf("parking stub must be rejected, got IsReal=true conf=%s ev=%v", vr.Confidence, vr.Evidence)
	}
	if vr.Confidence != ConfNone {
		t.Errorf("rejected doc must be conf=none, got %q", vr.Confidence)
	}
}

func TestClassifyBodyRejectsSoftVariants(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"meta_refresh", `<!DOCTYPE html><html><head><meta http-equiv="refresh" content="0;url=/home"></head><body></body></html>`},
		{"location_replace", `<html><head><script>location.replace("/parked")</script></head></html>`},
		{"text_404", `<html><head><title>404 Page Not Found</title></head><body><h1>Not found</h1></body></html>`},
		{"for_sale", `<html><head><title>This domain is for sale</title></head><body>Buy this domain</body></html>`},
		{"coming_soon", `<html><head><title>Coming Soon</title></head></html>`},
		{"empty", `<html><head></head><body></body></html>`},
	}
	for _, c := range cases {
		vr := classifyBody(c.body, 200, DocPrivacyPolicy)
		if vr.IsReal {
			t.Errorf("%s: expected rejection, got IsReal=true conf=%s ev=%v", c.name, vr.Confidence, vr.Evidence)
		}
	}
}

func TestClassifyBodyHighConfidenceTitleMatch(t *testing.T) {
	body := `<!DOCTYPE html><html><head><title>Privacy Policy | Acme Inc</title></head>
<body><h1>Privacy Policy</h1><p>This policy describes how we collect and process your personal data. Last updated: January 2026.</p>` +
		strings.Repeat("We respect your privacy. ", 40) + `</body></html>`
	vr := classifyBody(body, 200, DocPrivacyPolicy)
	if !vr.IsReal {
		t.Fatal("real privacy page should be accepted")
	}
	if vr.Confidence != ConfHigh {
		t.Errorf("title match should be high confidence, got %q (ev=%v)", vr.Confidence, vr.Evidence)
	}
	if vr.Title != "Privacy Policy | Acme Inc" {
		t.Errorf("title extraction wrong: %q", vr.Title)
	}
}

func TestClassifyBodyMediumOnVocabOnly(t *testing.T) {
	// Title doesn't name the type, but the body has clear legal vocabulary.
	body := `<html><head><title>Acme — Legal</title></head><body>` +
		strings.Repeat("These terms are governed by the laws of Delaware and any dispute is subject to binding arbitration. ", 10) +
		`</body></html>`
	vr := classifyBody(body, 200, DocTermsOfService)
	if !vr.IsReal {
		t.Fatal("real legal page should be accepted")
	}
	if vr.Confidence != ConfMedium {
		t.Errorf("vocab-only should be medium, got %q (ev=%v)", vr.Confidence, vr.Evidence)
	}
}

func TestClassifyBodyLowForGenericPage(t *testing.T) {
	// A substantial real page with no type-naming title and no legal vocab.
	body := `<html><head><title>Welcome</title></head><body>` +
		strings.Repeat("Lorem ipsum dolor sit amet consectetur. ", 30) + `</body></html>`
	vr := classifyBody(body, 200, DocTermsOfService)
	if !vr.IsReal {
		t.Fatal("substantial page should be real")
	}
	if vr.Confidence != ConfLow {
		t.Errorf("generic page should be low, got %q", vr.Confidence)
	}
}

func TestClassifyBodyTitleMatchSurvivesSmallBody(t *testing.T) {
	// A genuine, short German imprint page — small but clearly titled. It must
	// NOT be rejected just for being small (no redirect, no 404 text).
	body := `<html><head><title>Impressum</title></head><body><h1>Impressum</h1>
<p>Acme GmbH, Musterstraße 1, 10115 Berlin. Geschäftsführer: Max Mustermann. Handelsregister HRB 12345.</p></body></html>`
	vr := classifyBody(body, 200, DocImprint)
	if !vr.IsReal {
		t.Fatalf("titled imprint should be accepted, ev=%v", vr.Evidence)
	}
	if vr.Confidence != ConfHigh {
		t.Errorf("imprint title match should be high, got %q", vr.Confidence)
	}
}

func TestConfidenceRankOrdering(t *testing.T) {
	if !(confidenceRank(ConfHigh) > confidenceRank(ConfMedium) &&
		confidenceRank(ConfMedium) > confidenceRank(ConfLow) &&
		confidenceRank(ConfLow) > confidenceRank(ConfNone)) {
		t.Fatal("confidence rank ordering broken")
	}
}
