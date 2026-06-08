package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/baditaflorin/go-common/meshresult"
)

// TestDeadDomainReturns404 exercises the meshresult unreachable path end to
// end: a non-resolving host produces a DNS NXDOMAIN inside the safehttp
// CheckURL guard, which classifyFetchError (now meshresult.ClassifyFetchError)
// maps to OutcomeUnreachable → HTTP 404. domainscope keys on this 404 to record
// NoData instead of a false-OK 200.
func TestDeadDomainReturns404(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet,
		"/?url=https://nonexistent-tosfinder-r5-probe.invalid", nil)
	rec := httptest.NewRecorder()
	Handler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("dead domain must return HTTP 404, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad json: %v body=%s", err, rec.Body.String())
	}
	if resp.Result != string(meshresult.OutcomeUnreachable) {
		t.Errorf("result=%q, want unreachable", resp.Result)
	}
	if resp.Status != StatusUnreachable {
		t.Errorf("status=%q, want unreachable", resp.Status)
	}
	if resp.Reason == "" {
		t.Error("expected a machine-readable meshresult reason token")
	}
}

// TestOutcomeHTTPCodeContract pins the meshresult HTTP-code contract this
// service depends on (ok→200, no_data/unreachable→404).
func TestOutcomeHTTPCodeContract(t *testing.T) {
	if meshresult.OutcomeOK.HTTPCode() != http.StatusOK {
		t.Errorf("OutcomeOK should map to 200")
	}
	if meshresult.OutcomeNoData.HTTPCode() != http.StatusNotFound {
		t.Errorf("OutcomeNoData should map to 404")
	}
	if meshresult.OutcomeUnreachable.HTTPCode() != http.StatusNotFound {
		t.Errorf("OutcomeUnreachable should map to 404")
	}
}
