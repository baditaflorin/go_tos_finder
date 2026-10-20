package main

import (
	"net/url"
	"testing"
)

func TestLinkScanResolvesStaticLegalURLsFromFirstPartyJavaScriptAnchors(t *testing.T) {
	page := `<html><footer>
		<a href="javascript:showPrivacyPolicy();">Privacy Policy</a>
		<a href="javascript:showTerms();">Terms of Service</a>
	</footer><script>
		function showPrivacyPolicy() { showDialog('https://legal-cdn.other.test/websites/legal/privacy-policy.html', 'Privacy Policy'); }
		function showTerms() { showDialog('https://legal-cdn.other.test/websites/legal/terms-of-service.html', 'Terms of Service'); }
	</script></html>`
	base, err := url.Parse("https://publisher.example.test/")
	if err != nil {
		t.Fatal(err)
	}

	hits := linkScan(page, base)
	want := map[DocType]string{
		DocPrivacyPolicy:  "https://legal-cdn.other.test/websites/legal/privacy-policy.html",
		DocTermsOfService: "https://legal-cdn.other.test/websites/legal/terms-of-service.html",
	}
	for docType, target := range want {
		hit, ok := hits[docType]
		if !ok {
			t.Errorf("linkScan() missed %s", docType)
			continue
		}
		if hit.URL != target {
			t.Errorf("%s URL = %q, want %q", docType, hit.URL, target)
		}
		if hit.Source != "footer_link" {
			t.Errorf("%s source = %q, want footer_link", docType, hit.Source)
		}
		if hit.LinkConfidence != ConfMedium {
			t.Errorf("%s link confidence = %q, want medium", docType, hit.LinkConfidence)
		}
	}
}

func TestStaticScriptLegalTargetRequiresSpecificMatchingEvidence(t *testing.T) {
	cases := []struct {
		name string
		page string
		href string
		text string
	}{
		{
			name: "offsite ordinary anchor stays rejected by link scan",
			page: `<a href="https://legal-cdn.other.test/privacy-policy">Privacy Policy</a>`,
			href: `javascript:openPrivacy();`,
			text: "Privacy Policy",
		},
		{
			name: "path and link label must agree",
			page: `<script>function openPolicy() { show('https://legal-cdn.other.test/privacy-policy.html'); }</script>`,
			href: `javascript:openPolicy();`,
			text: "Terms of Service",
		},
		{
			name: "argument-bearing calls are not evaluated",
			page: `<script>function openPolicy(url) { show(url); }</script>`,
			href: `javascript:openPolicy('https://legal-cdn.example.test/privacy-policy.html');`,
			text: "Privacy Policy",
		},
		{
			name: "dynamic URL is not followed",
			page: `<script>function openPolicy() { show(buildPolicyURL()); }</script>`,
			href: `javascript:openPolicy();`,
			text: "Privacy Policy",
		},
		{
			name: "multiple static URLs are ambiguous",
			page: `<script>function openPolicy() { track('https://analytics.other.test/privacy-policy'); show('https://legal.other.test/privacy-policy'); }</script>`,
			href: `javascript:openPolicy();`,
			text: "Privacy Policy",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := staticScriptLegalTarget(staticScriptLegalURLs(tc.page), tc.href, tc.text); ok {
				t.Fatal("staticScriptLegalTarget() accepted ambiguous or mismatched evidence")
			}
		})
	}
}

func TestLinkScanKeepsOffsitePolicyLinksIgnoredWithoutScriptProvenance(t *testing.T) {
	page := `<footer><a href="https://policies.other.test/privacy-policy">Privacy Policy</a></footer>`
	base, err := url.Parse("https://publisher.example.test/")
	if err != nil {
		t.Fatal(err)
	}
	if hits := linkScan(page, base); len(hits) != 0 {
		t.Fatalf("linkScan() accepted unrelated offsite policies: %#v", hits)
	}
}
