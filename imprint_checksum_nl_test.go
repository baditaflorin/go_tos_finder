package main

import "testing"

// TestValidateIdentifierNLIndeterminateVsInvalid guards the elfproef
// remainder-10 distinction added to validateVAT's NL case: a genuine
// post-2020 Dutch sole-trader (eenmanszaak/zzp) VAT that fails elfproef
// because it isn't elfproef-derived at all (real evidence:
// simpelbootverhuurutrecht.nl's NL005444107B41) must report format_valid,
// but a plain typo'd or fabricated NL VAT — a definite digit mismatch, not
// the ambiguous remainder-10 case — must still report checksum_invalid.
// This repo had no NL checksum test at all before this fix; the sibling
// go_legal_entity repo's own pre-existing equivalent test caught the
// original blanket "always formatValid on elfproef failure" fallback as a
// real regression, so this test is added here too to guard the same class
// of mistake in the future.
func TestValidateIdentifierNLIndeterminateVsInvalid(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  validity
	}{
		{"genuine sole-trader VAT, elfproef remainder 10 (indeterminate)", "NL005444107B41", formatValid},
		{"synthetic broken VAT, definite mismatch", "NL123456789B01", checksumInvalid},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := validateIdentifier("VAT", c.value, "NL")
			if got != c.want {
				t.Errorf("validateIdentifier(\"VAT\", %q, \"NL\") = %s, want %s", c.value, got, c.want)
			}
		})
	}
}
