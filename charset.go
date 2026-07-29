package main

import (
	"golang.org/x/net/html/charset"
)

// decodeToUTF8 transcodes raw to UTF-8 when the page explicitly declares a
// non-UTF-8 encoding (via the HTTP Content-Type charset parameter or an
// in-document <meta charset>/http-equiv tag) that charset.DetermineEncoding
// is CERTAIN about. Real production sites still serve ISO-8859-1/
// Windows-1252 (common on older European sites — umlauts/accents in
// "Datenschutzerklärung", "Termini e condizioni", etc. are single bytes in
// those encodings but multi-byte in UTF-8) and Shift_JIS/EUC-KR/GBK (older
// Japanese/Korean/Chinese sites). Reading those bytes as if they were already
// UTF-8 corrupts every non-ASCII title/link-text/body match — including the
// CJK/Cyrillic/Greek/Arabic scriptRegex matchers in patterns_script.go, which
// exist specifically to close the non-Western-script gap but can never fire
// against a body that was never actually UTF-8 to begin with — and produces
// invalid UTF-8 that encoding/json silently mangles (U+FFFD replacement) on
// the way out to callers.
//
// Only acts when `certain` is true: an undeclared/ambiguous body (the common
// case — most modern sites ARE UTF-8 and simply don't bother declaring it, or
// declare it in a way DetermineEncoding can't pin down) is returned
// unchanged, exactly matching pre-fix behavior. This avoids the opposite risk
// — charset.DetermineEncoding's own documented fallback for genuinely
// undeclared HTML is windows-1252 with certain=false, and blindly applying
// that to an undeclared UTF-8 page would corrupt every multi-byte UTF-8
// sequence it contains. Gating on `certain` keeps this a pure bug fix with no
// new false-corruption surface.
func decodeToUTF8(raw []byte, contentType string) string {
	if len(raw) == 0 {
		return ""
	}
	enc, name, certain := charset.DetermineEncoding(raw, contentType)
	if !certain || enc == nil || name == "utf-8" {
		return string(raw)
	}
	decoded, err := enc.NewDecoder().Bytes(raw)
	if err != nil || len(decoded) == 0 {
		return string(raw)
	}
	return string(decoded)
}
