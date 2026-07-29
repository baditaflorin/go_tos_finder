package main

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// TestDecodeToUTF8DeclaredISO88591 reproduces a real, still-common production
// shape found in our own 1000-domain sample (2026-07-29): several real German
// sites (photovoltaik-info.eu, dentlein.de, campgrnds.tech, kukma.net) declare
// `charset=iso-8859-1` / `charset=windows-1252` in the HTTP Content-Type
// header. A German "Datenschutzerklärung" (privacy policy) link/title in that
// encoding carries the umlaut as a single Latin-1 byte (0xE4 for 'ä'), which
// is NOT valid UTF-8 on its own. Pre-fix, that raw byte was read straight into
// a Go string and handed to every regex in patterns.go as if it were already
// UTF-8 — an invalid byte sequence that regexp's Unicode-aware matching
// cannot match against the pattern's (valid UTF-8) 'ä'/'ü'/'ö' alternatives,
// silently failing the very geo-coverage patterns added for this purpose.
func TestDecodeToUTF8DeclaredISO88591(t *testing.T) {
	utf8Title := "Datenschutzerklärung"
	latin1Bytes, err := charmap.ISO8859_1.NewEncoder().String(utf8Title)
	if err != nil {
		t.Fatalf("failed to build ISO-8859-1 test fixture: %v", err)
	}
	raw := []byte("<html><head><title>" + latin1Bytes + "</title></head><body>" + latin1Bytes + "</body></html>")

	// Pre-fix behavior: treat raw bytes as UTF-8 directly. The title regex
	// (which matches "datenschutz(?:erkl[äa]rung|hinweise)?") must fail
	// against the still-Latin1-encoded bytes.
	preFix := string(raw)
	if titleRegexFor(DocPrivacyPolicy).MatchString(preFix) {
		t.Fatal("test fixture invariant broken: raw Latin-1 bytes should NOT already match the UTF-8 title regex (if they do, this test proves nothing)")
	}

	decoded := decodeToUTF8(raw, "text/html; charset=iso-8859-1")
	if !strings.Contains(decoded, utf8Title) {
		t.Fatalf("decodeToUTF8 did not recover the UTF-8 title: %q", decoded)
	}
	if !titleRegexFor(DocPrivacyPolicy).MatchString(decoded) {
		t.Errorf("decoded body should match the privacy title regex, got: %q", decoded)
	}

	// The full classifyBody path should now score this as a real, high-
	// confidence privacy policy — proving the fix closes the gap end-to-end,
	// not just at the decode step.
	vr := classifyBody(decoded, 200, DocPrivacyPolicy)
	if !vr.IsReal || vr.Confidence != ConfHigh {
		t.Errorf("classifyBody on decoded ISO-8859-1 body: real=%v conf=%q ev=%v", vr.IsReal, vr.Confidence, vr.Evidence)
	}
}

// TestDecodeToUTF8DeclaredWindows1252 covers the Windows-1252 variant (the
// most common real-world "Latin-1" mislabel — mrgori.com in our sample
// declared charset=windows-1252) using a French accented phrase.
func TestDecodeToUTF8DeclaredWindows1252(t *testing.T) {
	utf8Text := "Conditions générales d'utilisation"
	win1252Bytes, err := charmap.Windows1252.NewEncoder().String(utf8Text)
	if err != nil {
		t.Fatalf("failed to build Windows-1252 test fixture: %v", err)
	}
	raw := []byte("<html><body>" + win1252Bytes + "</body></html>")

	decoded := decodeToUTF8(raw, "text/html; charset=windows-1252")
	if !strings.Contains(decoded, utf8Text) {
		t.Fatalf("decodeToUTF8 did not recover the UTF-8 text: %q", decoded)
	}
}

// TestDecodeToUTF8LeavesUndeclaredBodyUnchanged is the safety-net test: a body
// with NO charset declaration (the common case — most modern sites are UTF-8
// and don't bother declaring it) must be returned byte-for-byte unchanged.
// charset.DetermineEncoding's own documented fallback for undeclared HTML is
// windows-1252 with certain=false; blindly transcoding on that guess would
// corrupt every already-UTF-8 multi-byte sequence in an undeclared page. This
// pins the `certain` gate that prevents that regression.
func TestDecodeToUTF8LeavesUndeclaredBodyUnchanged(t *testing.T) {
	orig := "<html><head><title>プライバシーポリシー</title></head><body>本ポリシーは個人情報の取り扱いについて定めます。</body></html>"
	got := decodeToUTF8([]byte(orig), "text/html")
	if got != orig {
		t.Errorf("undeclared UTF-8 body must be returned unchanged, got %q want %q", got, orig)
	}
}

// TestDecodeToUTF8DeclaredUTF8IsNoop: an explicit charset=utf-8 declaration
// must be a no-op (not re-decoded/double-processed).
func TestDecodeToUTF8DeclaredUTF8IsNoop(t *testing.T) {
	orig := "<html><body>Café — déjà vu</body></html>"
	got := decodeToUTF8([]byte(orig), "text/html; charset=utf-8")
	if got != orig {
		t.Errorf("explicit utf-8 declaration must be a no-op, got %q want %q", got, orig)
	}
}

// TestDecodeToUTF8EmptyBody: an empty body must not panic and returns empty.
func TestDecodeToUTF8EmptyBody(t *testing.T) {
	if got := decodeToUTF8(nil, "text/html; charset=iso-8859-1"); got != "" {
		t.Errorf("expected empty string for nil input, got %q", got)
	}
}

// TestGermanCompoundPrivacyTitleHighConfidence is a standalone regression test
// for the "Datenschutzerklärung" titleRegex/docTypeSignalRE fix (patterns.go /
// verify.go): the standard, near-universal German privacy-policy page title is
// the COMPOUND word "Datenschutzerklärung", not the bare word "Datenschutz".
// Before the fix, `\bdatenschutz\b` could never match inside that compound (no
// word boundary between "...schutz" and "erklärung..."), so a real German
// privacy page titled exactly this way — even with perfectly-decoded UTF-8 —
// fell through to low/medium confidence, and could be REJECTED outright by the
// canonical-probe false-positive guard (bodyHasTypeSignal) if the body used
// only German vocabulary with no English "privacy"/"we collect" phrasing.
func TestGermanCompoundPrivacyTitleHighConfidence(t *testing.T) {
	body := `<html><head><title>Datenschutzerklärung</title></head><body><h1>Datenschutzerklärung</h1>` +
		strings.Repeat("Diese Datenschutzerklärung informiert Sie über die Verarbeitung Ihrer personenbezogenen Daten. ", 10) +
		`</body></html>`
	vr := classifyBody(body, 200, DocPrivacyPolicy)
	if !vr.IsReal {
		t.Fatalf("real German privacy page should be accepted, ev=%v", vr.Evidence)
	}
	if vr.Confidence != ConfHigh {
		t.Errorf("Datenschutzerklärung title should be high confidence, got %q (ev=%v)", vr.Confidence, vr.Evidence)
	}

	// The canonical-probe false-positive guard (bodyHasTypeSignal) must also
	// accept a German-only body with no English legal vocabulary at all.
	germanOnlyBody := "Diese Datenschutzerklärung regelt die Erhebung personenbezogener Daten unserer Nutzer."
	if !bodyHasTypeSignal(germanOnlyBody, DocPrivacyPolicy) {
		t.Error("bodyHasTypeSignal should accept a German-only Datenschutzerklärung body via docTypeSignalRE")
	}
}
