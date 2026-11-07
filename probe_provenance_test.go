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
		want             string
	}{
		{name: "direct response equal to request is ambiguous", requestURL: requested, want: ""},
		{name: "direct redirect", requestURL: resolved, want: resolved},
		{name: "missing response request", want: ""},
		{name: "cache synthesized selected URL", cached: true, requestURL: requested, want: ""},
		{name: "cache explicitly reports final URL", cached: true, requestURL: requested, reported: []string{resolved}, want: resolved},
		{name: "cache final header conflicts", cached: true, requestURL: requested, reported: []string{requested, resolved}, want: ""},
		{name: "cache malformed final URL", cached: true, requestURL: requested, reported: []string{"javascript:alert(1)"}, want: ""},
		{name: "cache final URL has user info", cached: true, requestURL: requested, reported: []string{"https://secret@example.test/path"}, want: ""},
		{name: "fallback same URL is ambiguous", cached: true, fallback: true, requestURL: requested, want: ""},
		{name: "fallback actual redirect", cached: true, fallback: true, requestURL: resolved, want: resolved},
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
			if got := observedResponseURL(resp, requested); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
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
			im := extractImprintFieldsWithSource(source, hit.Body, "127.0.0.1", hit.BodyURL, hit.BodyLimitReached, hit.BodyResponseReportedTruncated, hit.BodyReadError)
			data, err := json.Marshal(im)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), `"response_reported_truncated":true`) != c.want {
				t.Fatal("response marker not preserved independently")
			}
			if c.fallback && im.Evidence.FetchedURL != "" {
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
	im := extractImprintFieldsWithSource(source, hit.Body, "127.0.0.1", hit.BodyURL, hit.BodyLimitReached, hit.BodyResponseReportedTruncated, hit.BodyReadError)
	encoded, err := json.Marshal(im)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"source_read_error":true`) || strings.Contains(string(encoded), "RAW_ERROR_MUST_NOT_LEAK") {
		t.Fatal("read failure should be a sanitized boolean")
	}
	empty := extractImprintFieldsWithSource(source, "", "127.0.0.1", "", false, false, true)
	if empty.Evidence == nil || !empty.Evidence.SourceReadError || len(empty.Evidence.Observations) != 0 {
		t.Fatal("empty partial read lost its capture status")
	}
}
