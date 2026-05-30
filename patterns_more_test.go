package main

import "testing"

// TestClassifyLinkBroadenedPaths covers the multilingual + CMS-convention paths
// added in 1.2.0, derived from real homepage anchors (Cloudflare, GitLab,
// Mozilla, Shopify, Stripe, Squarespace, etc.).
func TestClassifyLinkBroadenedPaths(t *testing.T) {
	cases := []struct {
		path string
		want DocType
	}{
		// Terms — locale prefixes, CMS suffixes, extra languages.
		{"/en-ro/terms", DocTermsOfService},
		{"/terms-and-conditions", DocTermsOfService},
		{"/conditions-generales", DocTermsOfService},
		{"/cgu", DocTermsOfService},
		{"/termini-di-servizio", DocTermsOfService},
		{"/terminos", DocTermsOfService},
		{"/termos", DocTermsOfService},
		{"/voorwaarden", DocTermsOfService},
		{"/regulamin", DocTermsOfService},
		{"/legal/terms.php", DocTermsOfService},
		// Privacy — extra languages + center variants.
		{"/de/privacy/websites", DocPrivacyPolicy},
		{"/data-privacy", DocPrivacyPolicy},
		{"/privacidad", DocPrivacyPolicy},
		{"/privacidade", DocPrivacyPolicy},
		{"/privacyverklaring", DocPrivacyPolicy},
		{"/polityka-prywatnosci", DocPrivacyPolicy},
		{"/datenschutzhinweise", DocPrivacyPolicy},
		// Cookie settings / preferences.
		{"/cookie-settings", DocCookiePolicy},
		{"/cookiebeleid", DocCookiePolicy},
		// Acceptable use & restricted businesses.
		{"/acceptable-use-policy", DocAcceptableUse},
		{"/legal/restricted-businesses", DocAcceptableUse},
		// DMCA / takedown.
		{"/takedown", DocDMCA},
		{"/copyright-infringement", DocDMCA},
		// Trust center / hub.
		{"/trust-hub/", DocTrustCenter},
		{"/trust-center", DocTrustCenter},
		{"/security-center", DocTrustCenter},
		// Imprint extra langs.
		{"/de/about/legal/impressum/", DocImprint},
		{"/note-legali", DocImprint},
		{"/aviso-legal", DocImprint},
		// Disclaimer.
		{"/disclaimer", DocDisclaimer},
		// Non-matches.
		{"/about", ""},
		{"/blog/cookies-are-tasty", DocCookiePolicy}, // "cookies" segment — acceptable
	}
	for _, c := range cases {
		got, _ := classifyLink(c.path, "")
		if got != c.want {
			t.Errorf("classifyLink(%q): got %q want %q", c.path, got, c.want)
		}
	}
}

// TestLegalHubDisambiguation: a bare /legal or /policies link is the hub; a
// /legal/privacy link must classify as the specific doc, not the hub.
func TestLegalHubDisambiguation(t *testing.T) {
	cases := []struct {
		path string
		text string
		want DocType
	}{
		{"/legal", "Legal", DocLegalHub},
		{"/legal/", "Legal", DocLegalHub},
		{"/policies/", "Policies", DocLegalHub},
		{"/de/about/legal/", "Legal", DocLegalHub},
		{"/legal/privacy", "Privacy Policy", DocPrivacyPolicy},
		{"/policies/terms/", "Terms of use", DocTermsOfService},
		{"/legal/terms", "Terms", DocTermsOfService},
		{"/policies/privacy/", "Privacy policy", DocPrivacyPolicy},
	}
	for _, c := range cases {
		got, _ := classifyLink(c.path, c.text)
		if got != c.want {
			t.Errorf("classifyLink(%q,%q): got %q want %q", c.path, c.text, got, c.want)
		}
	}
}

// TestClassifyLinkBroadenedText covers multilingual link text.
func TestClassifyLinkBroadenedText(t *testing.T) {
	cases := []struct {
		text string
		want DocType
	}{
		{"Terms and Conditions", DocTermsOfService},
		{"Conditions générales", DocTermsOfService},
		{"Términos y condiciones", DocTermsOfService},
		{"Termos de uso", DocTermsOfService},
		{"Privacy statement", DocPrivacyPolicy},
		{"Política de privacidad", DocPrivacyPolicy},
		{"Cookie settings", DocCookiePolicy},
		{"Trust Center", DocTrustCenter},
		{"Trust & Safety", DocTrustCenter},
		{"Code of Conduct", DocCommunityGuidelines},
		{"Aviso legal", DocImprint},
	}
	for _, c := range cases {
		got, _ := classifyLink("/unrelated-path", c.text)
		if got != c.want {
			t.Errorf("classifyLink text %q: got %q want %q", c.text, got, c.want)
		}
	}
}

// TestHubsExcludedFromProbes: hub types must never be canonical-probed (they
// have no canonicalPaths and would be soft-404 magnets).
func TestHubsExcludedFromProbes(t *testing.T) {
	probes := allCanonicalProbes([]DocType{DocLegalHub, DocTrustCenter, DocPrivacyPolicy}, 30)
	for _, p := range probes {
		if hubDocTypes[p.DocType] {
			t.Errorf("hub type %s should not be probed", p.DocType)
		}
	}
	// Privacy still produces probes.
	found := false
	for _, p := range probes {
		if p.DocType == DocPrivacyPolicy {
			found = true
		}
	}
	if !found {
		t.Error("privacy should still be probed")
	}
}
