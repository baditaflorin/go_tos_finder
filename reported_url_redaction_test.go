package main

import (
	"net/http"
	"reflect"
	"testing"
)

// Reflection keeps this regression runnable against the previous release,
// whose helper returns only URL and basis and does not yet expose redaction.
func TestImprintReportedURLRedactionDistinctVariants(t *testing.T) {
	var clean string
	for _, raw := range []string{"https://example.test/imprint?token=first#one", "https://example.test/imprint?token=second#two"} {
		h := http.Header{}
		h.Set("X-FetchCache-Hit", "true")
		h.Set("X-FetchCache-Final-Url", raw)
		response := &http.Response{Header: h}
		out := reflect.ValueOf(observedResponseURL).Call([]reflect.Value{reflect.ValueOf(response), reflect.ValueOf("https://example.test/selected")})
		if len(out) != 3 {
			t.Fatal("reported URL redaction status is absent")
		}
		got, basis, redacted := out[0].String(), out[1].String(), out[2].Bool()
		if got != "https://example.test/imprint" || basis != "response_header" || !redacted {
			t.Fatal("URL components or redaction status were not retained safely")
		}
		if clean != "" && clean != got {
			t.Fatal("unexpected sanitized source path")
		}
		clean = got
	}
	// Equal displayed paths with redaction markers do not establish equal source
	// URLs; consumers must retain observation identity instead of deduplicating.
}
