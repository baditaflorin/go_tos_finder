package main

import (
	"strings"
	"testing"
)

// Cases come from a 2026-11 check on 1,025 sites whose operator and country are known from the
// company register (21 registries): the legal-form suffix alone gave the wrong country for most
// .ch, .sk, .com.co and .com.au sites.

func TestHostCountry(t *testing.T) {
	for host, want := range map[string]string{
		"druckerei-schoetz.ch": "CH", "teamup.sk": "SK", "pacifika.com.co": "CO", "correiodoestado.com.br": "BR",
		"centrestone.com.au": "AU", "cosapiinmobiliaria.com.pe": "PE", "flywaterford.ie": "IE", "x.co.uk": "GB",
		"foo.co": "", "stripe.com": "", "something.io": "", "ludopolis.eu": "", "localhost": "",
	} {
		if got := hostCountry(host); got != want {
			t.Errorf("hostCountry(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestPhoneCountryNeedsAClearWinner(t *testing.T) {
	if got := phoneCountry("Tel: +41 44 123 45 67, Fax +41 44 123 45 68"); got != "CH" {
		t.Errorf("two Swiss numbers: got %q", got)
	}
	if got := phoneCountry("Tel +41 44 123 45 67 / +49 30 1234567"); got != "" {
		t.Errorf("one Swiss and one German number must not decide, got %q", got)
	}
	if got := phoneCountry("Call 0049 30 1234567"); got != "DE" {
		t.Errorf("00-prefixed number: got %q", got)
	}
}

func TestRefineCountry(t *testing.T) {
	kana := strings.Repeat("会社概要お問い合わせ", 10)
	for _, tc := range []struct {
		name, country, suffix string
		fixed                 bool
		host, text, want      string
	}{
		{"swiss AG on .ch", "AT", "AG", false, "druckerei-schoetz.ch", "", "CH"},
		{"slovak s.r.o. on .sk", "CZ", "s.r.o.", false, "teamup.sk", "", "SK"},
		{"colombian S.A.S. on .com.co", "FR", "S.A.S.", false, "avsasa.com.co", "", "CO"},
		{"Inc. on .com.au", "US", "Inc.", false, "example.com.au", "", "AU"},
		{"Ltd on .ie", "GB", "Ltd", false, "flywaterford.ie", "", "IE"},
		{"GmbH with a Swiss phone on a .com", "DE", "GmbH", false, "example.com", "Tel: +41 44 123 45 67", "CH"},
		{"Japanese page with an English Inc.", "US", "Inc.", false, "rakko.inc", kana, "JP"},
		{"a VAT prefix or register number is never overridden", "DE", "GmbH", true, "firma.ch", "", "DE"},
		{"a form that is not native in the TLD country keeps the suffix country", "JP", "株式会社", false, "hino.com.au", "", "JP"},
		{"DE versus AT stays with refineDEATCountry", "DE", "GmbH", false, "firma.at", "", "DE"},
		{"same country", "FR", "SAS", false, "exemple.fr", "", "FR"},
		{"no evidence", "GB", "Ltd", false, "example.com", "", "GB"},
	} {
		if got := refineCountry(tc.country, tc.suffix, tc.fixed, tc.host, tc.text); got != tc.want {
			t.Errorf("%s: refineCountry(%q, %q, %v, %q) = %q, want %q", tc.name, tc.country, tc.suffix, tc.fixed, tc.host, got, tc.want)
		}
	}
}

func TestNormalizeLegalNameRedetectsSuffixAfterLabel(t *testing.T) {
	// nrlnt.com.au: found through the word "Company"; the name's own legal form is Limited.
	got, suffix, ok := normalizeLegalName("Company Name: ARLC NT Limited", "Company")
	if !ok || got != "ARLC NT Limited" || suffix != "Limited" {
		t.Errorf("got %q, %q, %v", got, suffix, ok)
	}
	for _, tc := range []struct{ in, suffix string }{
		{"+1268 AG", "AG"},                     // foragerfunds.com: a phone number
		{"Company number 15040220", "Company"}, // dantetalent.com
		{"Forme juridique : SARL", "SARL"},     // restaurant-le-16.fr
		{"Rechtsvorm: NV", "NV"},               // rosahotel.be
	} {
		if got, _, ok := normalizeLegalName(tc.in, tc.suffix); ok {
			t.Errorf("normalizeLegalName(%q) = %q, true; want rejected", tc.in, got)
		}
	}
}
