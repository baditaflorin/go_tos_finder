package main

import (
	"net/http"
	"strings"
	"testing"

	"github.com/baditaflorin/go-common/fleetfetch"
)

func TestShouldTryJSFallbackOnlyForSparseScriptShells(t *testing.T) {
	shell := `<html><head><script src="/app.js"></script><script src="/runtime.js"></script><script>window.__bootstrap="` + strings.Repeat("x", 600) + `"</script></head><body><div id="root">Loading</div></body></html>`
	withLegalLink := shell + `<a href="/privacy">Privacy Policy</a>`
	withText := `<html><head><script src="/app.js"></script><script src="/runtime.js"></script></head><body>` + strings.Repeat("visible page text ", 40) + `</body></html>`
	withoutScripts := `<html><body>Loading</body></html>`
	cases := []struct {
		name      string
		body      string
		status    int
		links     int
		wantRetry bool
	}{
		{name: "sparse script shell", body: shell, status: http.StatusOK, wantRetry: true},
		{name: "legal link already present", body: withLegalLink, status: http.StatusOK, links: 1, wantRetry: false},
		{name: "substantial server-rendered page", body: withText, status: http.StatusOK, wantRetry: false},
		{name: "no script evidence", body: withoutScripts, status: http.StatusOK, wantRetry: false},
		{name: "blocked response", body: shell, status: http.StatusTooManyRequests, wantRetry: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldTryJSFallback(tc.body, tc.status, tc.links); got != tc.wantRetry {
				t.Fatalf("shouldTryJSFallback()=%t, want %t", got, tc.wantRetry)
			}
		})
	}
}

func TestConfirmedJSRenderRequiresRendererProvenance(t *testing.T) {
	cases := []struct {
		name   string
		header http.Header
		wantJS bool
	}{
		{name: "render metadata", header: http.Header{"X-Fetchcache-Render": []string{fleetfetch.RenderJS}}, wantJS: true},
		{name: "proxy metadata", header: http.Header{"X-Fetchcache-Via": []string{"js-proxy"}}, wantJS: true},
		{name: "HTML proxy is not JavaScript", header: http.Header{"X-Fetchcache-Render": []string{fleetfetch.RenderHTML}}, wantJS: false},
		{name: "direct fallback has no renderer", header: http.Header{"X-Fetchcache-Via-Fallback": []string{"true"}}, wantJS: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := confirmedJSRender(tc.header); got != tc.wantJS {
				t.Fatalf("confirmedJSRender()=%t, want %t", got, tc.wantJS)
			}
		})
	}
}
