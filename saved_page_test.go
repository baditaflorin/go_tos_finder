package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSavedPageReceiptAndEvidence(t *testing.T) {
	for _, tc := range []struct{ body, status, evidence string }{
		{"<footer><a href=\"/privacy-policy\">Privacy Policy</a><a href=\"/terms-of-service?ref=footer\">Terms of Service</a></footer>", "parsed", "/privacy-policy"},
		{"<p>Welcome to our homepage.</p>", "empty", ""},
	} {
		body := sha256.Sum256([]byte(tc.body))
		digest := hex.EncodeToString(body[:])
		req := httptest.NewRequest(http.MethodPost, "/v1/parse", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "text/html; charset=utf-8")
		req.Header.Set("X-Source-URL", "https://example.com/")
		req.Header.Set("X-Source-SHA256", digest)
		req.Header.Set("X-Capture-ID", "test-cap")
		w := httptest.NewRecorder()
		savedPageHandler.ServeHTTP(w, req)
		var result struct {
			Schema string            `json:"schema_version"`
			Status string            `json:"status"`
			Input  map[string]string `json:"input"`
			Parser map[string]string `json:"parser"`
			Data   json.RawMessage   `json:"data"`
		}
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &result) != nil ||
			result.Schema != "snapshot.parse.v1" || result.Status != tc.status ||
			result.Input["sha256"] != digest || result.Input["capture_id"] != "test-cap" ||
			result.Parser["id"] != "tos-finder" || result.Parser["version"] != Version ||
			!strings.Contains(strings.ToLower(string(result.Data)), strings.ToLower(tc.evidence)) {
			t.Fatalf("saved page returned %d %s", w.Code, w.Body)
		}
	}
}
