package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestImprintResponseURLProvenance(t *testing.T) {
	const requested = "https://example.test/selected"
	const resolved = "https://example.test/resolved"
	cases := []struct {
		name             string
		cached, fallback bool
		requestURL       string
		reported         []string
		want, wantBasis  string
		wantRedacted     bool
	}{
		{name: "direct response equal to request is ambiguous", requestURL: requested, want: ""},
		{name: "direct redirect", requestURL: resolved, want: resolved, wantBasis: "response_request"},
		{name: "missing response request", want: ""},
		{name: "cache synthesized selected URL", cached: true, requestURL: requested, want: ""},
		{name: "cache explicitly reports final URL", cached: true, requestURL: requested, reported: []string{resolved}, want: resolved, wantBasis: "response_header"},
		{name: "cache final header conflicts", cached: true, requestURL: requested, reported: []string{requested, resolved}, want: ""},
		{name: "cache malformed final URL", cached: true, requestURL: requested, reported: []string{"javascript:alert(1)"}, want: ""},
		{name: "cache userinfo is redacted", cached: true, requestURL: requested, reported: []string{"https://user:password@example.test/path"}, want: "https://example.test/path", wantBasis: "response_header", wantRedacted: true},
		{name: "query and fragment are redacted", cached: true, requestURL: requested, reported: []string{"https://example.test/path?token=hidden#fragment"}, want: "https://example.test/path", wantBasis: "response_header", wantRedacted: true},
		{name: "empty query delimiter is redacted", cached: true, requestURL: requested, reported: []string{"https://example.test/path?"}, want: "https://example.test/path", wantBasis: "response_header", wantRedacted: true},
		{name: "empty fragment delimiter is redacted", cached: true, requestURL: requested, reported: []string{"https://example.test/path#"}, want: "https://example.test/path", wantBasis: "response_header", wantRedacted: true},
		{name: "fallback same URL is ambiguous", cached: true, fallback: true, requestURL: requested, want: ""},
		{name: "fallback actual redirect", cached: true, fallback: true, requestURL: resolved, want: resolved, wantBasis: "response_request"},
		{name: "origin spoof is only a header report", cached: true, requestURL: requested, reported: []string{"https://spoof.example/path"}, want: "https://spoof.example/path", wantBasis: "response_header"},
		{name: "fallback ignores spoofed final header", cached: true, fallback: true, requestURL: resolved, reported: []string{"https://spoof.example/path"}, want: resolved, wantBasis: "response_request"},
		{name: "synthesized request is only a response report", cached: true, fallback: true, requestURL: "https://spoof.example/path", reported: []string{"https://spoof.example/path"}, want: "https://spoof.example/path", wantBasis: "response_request"},
		{name: "unchanged fallback ignores spoofed final header", cached: true, fallback: true, requestURL: requested, reported: []string{"https://spoof.example/path"}, want: ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			header := http.Header{}
			if c.cached {
				header.Set("X-FetchCache-Hit", "false")
			}
			if c.fallback {
				header.Set("X-FetchCache-Via-Fallback", "true")
			}
			for _, value := range c.reported {
				header.Add("X-FetchCache-Final-Url", value)
			}
			resp := &http.Response{Header: header}
			if c.requestURL != "" {
				resp.Request, _ = http.NewRequest(http.MethodGet, c.requestURL, nil)
			}
			if got, basis, redacted := observedResponseURL(resp, requested); got != c.want || basis != c.wantBasis || redacted != c.wantRedacted {
				t.Errorf("unexpected sanitized URL, basis or redaction state")
			}
		})
	}
}

func TestImprintResponseTruncationMarkerReachesEvidence(t *testing.T) {
	allowLoopback(t)
	cases := []struct {
		name     string
		cached   bool
		fallback bool
		marker   string
		want     bool
	}{
		{"true", true, false, "true", true}, {"case insensitive", true, false, " TRUE ", true},
		{"absent", true, false, "", false}, {"false", true, false, "false", false},
		{"direct header is only a response report", false, false, "true", true},
		{"fallback origin header is only a response report", true, true, "true", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
				headers := http.Header{}
				if c.cached {
					headers.Set("X-FetchCache-Hit", "false")
					headers.Set("X-FetchCache-Final-Url", req.URL.String())
				}
				if c.fallback {
					headers.Set("X-FetchCache-Via-Fallback", "true")
				}
				if c.marker != "" {
					headers.Set("X-FetchCache-Truncated", c.marker)
				}
				body := `<script type="application/ld+json">{"@type":"Organization","legalName":"Example GmbH"}</script>`
				return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})}
			source := "http://127.0.0.1/selected"
			pm, err := probeVerify(context.Background(), client, source)
			if err != nil {
				t.Fatal(err)
			}
			if pm.bodyLimitReached || pm.responseReportedTruncated != c.want {
				t.Fatal("incorrect short-body provenance")
			}
			hit := DocFinding{}
			hit.captureImprintBody(pm)
			im := extractImprintFieldsWithSource(source, hit.Body, "127.0.0.1", hit.BodyURL, hit.BodyURLBasis, hit.BodyURLRedacted, hit.BodyLimitReached, hit.BodyResponseReportedTruncated, hit.BodyReadError)
			data, err := json.Marshal(im)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), `"response_reported_truncated":true`) != c.want {
				t.Fatal("response marker not preserved independently")
			}
			if c.fallback && im.Evidence.ReportedFetchedURL != "" {
				t.Fatal("fallback origin header must not invent a final URL")
			}
			if strings.Contains(string(data), `"cache_reported_truncated"`) {
				t.Fatal("response header was attributed to the cache")
			}
			if strings.Contains(string(data), `"source_limit_reached":true`) {
				t.Fatal("short body cannot reach local source limit")
			}
		})
	}
}

type imprintInterruptedBody struct{ returned bool }

func (r *imprintInterruptedBody) Read(p []byte) (int, error) {
	if r.returned {
		return 0, errors.New("RAW_ERROR_MUST_NOT_LEAK")
	}
	r.returned = true
	body := `<script type="application/ld+json">{"@type":"Organization","legalName":"Example GmbH"}</script>`
	return copy(p, body), errors.New("RAW_ERROR_MUST_NOT_LEAK")
}

func TestImprintSourceReadErrorIsSanitized(t *testing.T) {
	allowLoopback(t)
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(&imprintInterruptedBody{}), Request: req}, nil
	})}
	source := "http://127.0.0.1/imprint"
	pm, err := probeVerify(context.Background(), client, source)
	if err != nil || !pm.bodyReadError || pm.bodyLimitReached {
		t.Fatal("interrupted short read was not classified")
	}
	hit := DocFinding{}
	hit.captureImprintBody(pm)
	im := extractImprintFieldsWithSource(source, hit.Body, "127.0.0.1", hit.BodyURL, hit.BodyURLBasis, hit.BodyURLRedacted, hit.BodyLimitReached, hit.BodyResponseReportedTruncated, hit.BodyReadError)
	encoded, err := json.Marshal(im)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"source_read_error":true`) || strings.Contains(string(encoded), "RAW_ERROR_MUST_NOT_LEAK") {
		t.Fatal("read failure should be a sanitized boolean")
	}
	empty := extractImprintFieldsWithSource(source, "", "127.0.0.1", "", "", false, false, false, true)
	if empty.Evidence == nil || !empty.Evidence.SourceReadError || len(empty.Evidence.Observations) != 0 {
		t.Fatal("empty partial read lost its capture status")
	}
}

func TestImprintReportedURLRedactionPropagation(t *testing.T) {
	allowLoopback(t)
	client := &http.Client{Transport: rtFunc(func(req *http.Request) (*http.Response, error) {
		headers := http.Header{}
		headers.Set("X-FetchCache-Hit", "false")
		headers.Set("X-FetchCache-Final-Url", "https://user:password@example.test/imprint?token=TOPSECRET#PRIVATEFRAGMENT")
		body := `<script type="application/ld+json">{"@type":"Organization","legalName":"Example GmbH"}</script>`
		return &http.Response{StatusCode: http.StatusOK, Header: headers, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})}
	source := "http://127.0.0.1/imprint"
	pm, err := probeVerify(context.Background(), client, source)
	if err != nil {
		t.Fatal(err)
	}
	hit := DocFinding{}
	hit.captureImprintBody(pm)
	im := extractImprintFieldsWithSource(source, hit.Body, "127.0.0.1", hit.BodyURL, hit.BodyURLBasis, hit.BodyURLRedacted, hit.BodyLimitReached, hit.BodyResponseReportedTruncated, hit.BodyReadError)
	encoded, err := json.Marshal(im.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"user", "password", "TOPSECRET", "PRIVATEFRAGMENT"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("reported URL leaked a stripped component")
		}
	}
	if im.Evidence.ReportedFetchedURL != "https://example.test/imprint" || im.Evidence.ReportedFetchedURLBasis != "response_header" || !im.Evidence.ReportedFetchedURLRedacted {
		t.Fatal("redacted URL, basis or flag not preserved")
	}
	legacy := "https://example.test/selected?legacy=unchanged#part"
	legacyResult := extractImprintFieldsWithSource(legacy, hit.Body, "example.test", hit.BodyURL, hit.BodyURLBasis, hit.BodyURLRedacted, false, false, false)
	if legacyResult.URL != legacy || legacyResult.Evidence.URL != legacy {
		t.Fatal("legacy selected-link fields changed")
	}
}
