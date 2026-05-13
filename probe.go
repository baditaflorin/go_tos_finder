package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/baditaflorin/go-common/safehttp"
)

// stalenessThreshold: documents whose Last-Modified header is older than this
// are flagged as `outdated`.
const stalenessThreshold = 2 * 365 * 24 * time.Hour

// probeHead does an HTTP HEAD against the given URL and captures status,
// Content-Length, and Last-Modified. If HEAD is rejected by the server with
// 405/501, falls back to a GET with no body read.
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
