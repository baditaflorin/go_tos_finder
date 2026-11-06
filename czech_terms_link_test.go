package main

import (
	"net/url"
	"testing"
)

func TestCzechTermsPaths(t *testing.T) {
	for _, tc := range []struct{ path, label string }{
		{"/eshop/obchodni-podminky/", "Obchodní podmínky"},
		{"/stranka/vseobecne-obchodni-podminky", "Všeobecné obchodní podmínky"},
	} {
		got, _, floor := classifyLinkEvidence(tc.path, tc.label)
		if got != DocTermsOfService || floor == ConfNone {
			t.Errorf("%q => %s/%s", tc.path, got, floor)
		}
	}
}
func TestCzechTermsLinkStaysSameSite(t *testing.T) {
	u, _ := url.Parse("https://shop.example.cz/")
	got := linkScan(`<a href="https://other.example/obchodni-podminky">Obchodní podmínky</a><a href="/stranka/kontakt">Kontakt</a>`, u)
	if x := got[DocTermsOfService]; x.URL != "" {
		t.Fatalf("unexpected offsite policy: %+v", x)
	}
}
