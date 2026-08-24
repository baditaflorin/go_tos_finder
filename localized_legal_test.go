package main

import "testing"

func TestLocalizedLegalAndImprintEvidence(t *testing.T) {
	cases := []struct {
		path, text string
		want       DocType
	}{
		{"/ro/informatii-legale", "Informații legale", DocImprint},
		{"/it/termini-e-condizioni", "Termini e condizioni", DocTermsOfService},
	}
	for _, tc := range cases {
		got, _, floor := classifyLinkEvidence(tc.path, tc.text)
		if got != tc.want || floor == ConfNone {
			t.Errorf("%s/%s => %s/%s", tc.path, tc.text, got, floor)
		}
	}
}

func TestDisclaimerOnlyEvidenceIsDiscarded(t *testing.T) {
	merged := map[DocType]DocFinding{DocDisclaimer: {Type: DocDisclaimer, URL: "https://example.test/disclaimer", Status: 200}}
	discardStandaloneDisclaimer(merged)
	if _, ok := merged[DocDisclaimer]; ok {
		t.Fatal("standalone disclaimer was retained as a legal document")
	}
	if got, _, _ := classifyLinkEvidence("/blog/legal-guide", "Disclaimer"); got != "" {
		t.Fatalf("disclaimer prose link classified as %s", got)
	}
}
