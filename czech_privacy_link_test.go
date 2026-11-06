package main

import (
	"net/url"
	"testing"
)

func TestCzechPrivacyLinkAndTitle(t *testing.T) {
	typ, _, floor := classifyLinkEvidence("/technicke-stranky/ochrana-osobnich-udaju/", "Ochrana osobních údajů")
	if typ != DocPrivacyPolicy || floor != ConfMedium {
		t.Fatalf("type %s confidence %s", typ, floor)
	}
	r := classifyBody(`<html><head><title>Ochrana osobních údajů | Vaillant</title></head><body><h1>Zásady ochrany osobních údajů</h1><p>Ochrana osobních údajů a zpracování dat pro zákazníky.</p></body></html>`, 200, DocPrivacyPolicy)
	if !r.IsReal || r.Confidence != ConfHigh {
		t.Fatalf("result %+v", r)
	}
	u, _ := url.Parse("https://example.cz/")
	m := linkScan(`<a href="https://other.example/ochrana-osobnich-udaju/">Ochrana osobních údajů</a>`, u)
	if m[DocPrivacyPolicy].URL != "" {
		t.Fatal("accepted external link")
	}
	typ, _, _ = classifyLinkEvidence("/blog/ochrana-osobnich-udaju-produkt", "Zboží")
	if typ == DocPrivacyPolicy {
		t.Fatal("matched longer path")
	}
}
