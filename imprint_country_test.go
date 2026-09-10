package main

import "testing"

func TestNormalizeCountryEasternEuropeanEndonyms(t *testing.T) {
	cases := map[string]string{
		"Srbija": "RS", "Bosna i Hercegovina": "BA", "Crna Gora": "ME",
		"Severna Makedonija": "MK", "Moldova Republicii": "MD",
	}
	for input, want := range cases {
		if got := normalizeCountry(input); got != want {
			t.Errorf("normalizeCountry(%q) = %q, want %q", input, got, want)
		}
	}
}
