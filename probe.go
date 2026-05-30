package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/baditaflorin/go-common/safehttp"
)

// stalenessThreshold: documents whose Last-Modified header is older than this
// are flagged as `outdated`.
const stalenessThreshold = 2 * 365 * 24 * time.Hour

// verifyBodyBytes caps how much of a candidate document body we read for
// content verification. Enough to capture <head>, <title>, <h1> and a slab of
// opening prose for the legal-vocabulary signal, without slurping whole pages.
const verifyBodyBytes = 64 * 1024

// probeMeta is the raw outcome of a single candidate fetch: status, size,
// freshness header, and a bounded slice of the body for content verification.
type probeMeta struct {
	status     int
	contentLen int64
	lastMod    string
	body       string
	finalURL   string
}

// probeVerify does a GET against the candidate URL and returns status,
// metadata, and a bounded body slice for content classification. Using GET
// (not HEAD) is deliberate: a soft-404 parking page returns 200 to HEAD with a
// misleading Content-Length, so HEAD alone cannot distinguish a real legal
// page from a JS-redirect stub. We read at most verifyBodyBytes.
func probeVerify(ctx context.Context, client *http.Client, rawURL string) (probeMeta, error) {
	var pm probeMeta
	if _, perr := safehttp.CheckURL(ctx, rawURL); perr != nil {
		return pm, perr
	}
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if rerr != nil {
		return pm, rerr
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "en;q=0.9,de;q=0.8,fr;q=0.7,es;q=0.6")
	resp, err := client.Do(req)
	if err != nil {
		return pm, err
	}
	defer resp.Body.Close()
	pm.status = resp.StatusCode
	pm.contentLen = resp.ContentLength
	pm.lastMod = strings.TrimSpace(resp.Header.Get("Last-Modified"))
	if resp.Request != nil && resp.Request.URL != nil {
		pm.finalURL = resp.Request.URL.String()
	}
	if pm.status >= 200 && pm.status < 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, verifyBodyBytes))
		pm.body = string(b)
		// If Content-Length wasn't advertised (-1), backfill from what we read
		// so downstream size heuristics still work for chunked responses.
		if pm.contentLen < 0 {
			pm.contentLen = int64(len(b))
		}
	}
	return pm, nil
}

// probeHead does an HTTP HEAD against the given URL and captures status,
// Content-Length, and Last-Modified. Retained for the link-confirmation pass
// where the page was already discovered (an explicit footer link is itself
// strong evidence) — but the canonical-path discovery pass now uses
// probeVerify to defeat soft-404s. Falls back to GET on 405/501.
func probeHead(ctx context.Context, client *http.Client, rawURL string) (status int, contentLen int64, lastMod string, err error) {
	_, perr := safehttp.CheckURL(ctx, rawURL)
	if perr != nil {
		return 0, 0, "", perr
	}
	doReq := func(method string) (*http.Response, error) {
		req, rerr := http.NewRequestWithContext(ctx, method, rawURL, nil)
		if rerr != nil {
			return nil, rerr
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "text/html,*/*;q=0.5")
		return client.Do(req)
	}
	resp, err := doReq(http.MethodHead)
	if err != nil {
		return 0, 0, "", err
	}
	_ = resp.Body.Close()
	status = resp.StatusCode
	contentLen = resp.ContentLength
	lastMod = strings.TrimSpace(resp.Header.Get("Last-Modified"))

	if status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented {
		resp2, err2 := doReq(http.MethodGet)
		if err2 != nil {
			return status, contentLen, lastMod, nil
		}
		_ = resp2.Body.Close()
		status = resp2.StatusCode
		contentLen = resp2.ContentLength
		if lm := strings.TrimSpace(resp2.Header.Get("Last-Modified")); lm != "" {
			lastMod = lm
		}
	}
	return status, contentLen, lastMod, nil
}

// stalenessOf parses Last-Modified and classifies the document as
// "current" / "outdated" / "" (unknown).
func stalenessOf(lastModified string, now time.Time) (string, string) {
	if lastModified == "" {
		return "", ""
	}
	t, err := http.ParseTime(lastModified)
	if err != nil {
		return "", ""
	}
	iso := t.UTC().Format("2006-01-02")
	if now.Sub(t) > stalenessThreshold {
		return "outdated", iso
	}
	return "current", iso
}
