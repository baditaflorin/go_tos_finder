package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/baditaflorin/go-common/fleetfetch"
	"github.com/baditaflorin/go-common/safehttp"
	"golang.org/x/net/idna"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	userAgent      = "go_tos_finder/" + Version + " (+https://github.com/baditaflorin/go_tos_finder)"
	requestTimeout = 12 * time.Second
	probeTimeout   = 10 * time.Second
	maxBodyBytes   = 4 * 1024 * 1024
	maxRedirects   = 5
	maxProbes      = 30
)

// DocFinding is one entry of the documents[] array in the response.
type DocFinding struct {
	Type         DocType  `json:"type"`
	URL          string   `json:"url"`
	Status       int      `json:"status,omitempty"`
	ContentLen   int64    `json:"content_length,omitempty"`
	LastModified string   `json:"last_modified,omitempty"`
	Staleness    string   `json:"staleness,omitempty"`
	Source       string   `json:"source"`
	LinkText     string   `json:"link_text,omitempty"`
	Match        string   `json:"match,omitempty"`
	Confidence   string   `json:"confidence,omitempty"`
	Title        string   `json:"title,omitempty"`
	Evidence     []string `json:"evidence,omitempty"`

	// LinkConfidence is the location-confidence floor derived from the link
	// itself (path-token / anchor-text corroboration) before any body fetch.
	// It is internal scoring metadata — not serialised — used to keep a
	// discovered link from collapsing to "low" when its target body GET fails
	// (a frequent timeout behind the fleet fetch proxy in production).
	LinkConfidence string `json:"-"`
}

// DetectionSummary is an aggregate evidence trail describing how the result
// was produced — part of the TRL-6/7 "evidence trail" requirement.
type DetectionSummary struct {
	HomepageFetched  bool   `json:"homepage_fetched"`
	RenderMode       string `json:"render_mode"`
	LinksScanned     int    `json:"links_scanned"`
	PathsProbed      int    `json:"paths_probed"`
	ProbesRejected   int    `json:"probes_rejected_soft404"`
	HighConfidence   int    `json:"high_confidence"`
	MediumConfidence int    `json:"medium_confidence"`
	LowConfidence    int    `json:"low_confidence"`
}

// Response is the JSON payload returned to callers.
type Response struct {
	Tool             string           `json:"tool"`
	Version          string           `json:"Version"`
	Target           string           `json:"target"`
	FetchedURL       string           `json:"fetched_url,omitempty"`
	ResolvedIP       string           `json:"resolved_ip,omitempty"`
	Documents        []DocFinding     `json:"documents"`
	DocumentsFound   int              `json:"documents_found"`
	DocumentsMissing []DocType        `json:"documents_missing"`
	ImprintPresent   bool             `json:"imprint_present"`
	Verdict          string           `json:"verdict"`
	Detection        DetectionSummary `json:"detection"`
	Error            string           `json:"error,omitempty"`
}

// renderMode is resolved once from env. The fleet JS-render proxy
// (RenderJS) is expensive and frequently unhealthy; defaulting to a direct
// SSRF-safe fetch (RenderDefault) is both cheaper and far more reliable for
// the static legal pages this service targets. Operators can opt back into
// JS rendering with TOS_FINDER_RENDER=js (or html).
func renderMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TOS_FINDER_RENDER"))) {
	case "js":
		return fleetfetch.RenderJS
	case "html":
		return fleetfetch.RenderHTML
	default:
		return fleetfetch.RenderDefault
	}
}

func newClient() *http.Client {
	c := fleetfetch.NewHTTPClient(fleetfetch.WithRender(renderMode()), fleetfetch.WithFallbackOnTimeout())
	c.Timeout = requestTimeout
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		_, err := safehttp.CheckURL(req.Context(), req.URL.String())
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", userAgent)
		return nil
	}
	return c
}

func writeJSON(w http.ResponseWriter, status int, resp Response) {
	resp.Tool = "go_tos_finder"
	resp.Version = Version
	if resp.Documents == nil {
		resp.Documents = []DocFinding{}
	}
	if resp.DocumentsMissing == nil {
		resp.DocumentsMissing = []DocType{}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

func addError(r Response, err error) Response {
	r.Error = err.Error()
	r.Verdict = "unknown"
	return r
}

// countSuccess counts docs that are present (link-only with no HEAD result, or
// a verified 2xx). Hub directory pages don't count toward documents_found —
// they're navigation, not a document.
func countSuccess(docs []DocFinding) int {
	n := 0
	for _, d := range docs {
		if hubDocTypes[d.Type] {
			continue
		}
		if d.Status == 0 || (d.Status >= 200 && d.Status < 400) {
			n++
		}
	}
	return n
}

// verdictOf maps documents_found + missing list to a coarse verdict label.
func verdictOf(found int, missing []DocType) string {
	// Critical = ToS + Privacy. If neither is found, it's "none".
	hasToS, hasPrivacy := true, true
	for _, m := range missing {
		if m == DocTermsOfService {
			hasToS = false
		}
		if m == DocPrivacyPolicy {
			hasPrivacy = false
		}
	}
	switch {
	case !hasToS && !hasPrivacy && found == 0:
		return "none"
	case found >= 5:
		return "comprehensive"
	case found >= 2 && hasToS && hasPrivacy:
		return "comprehensive"
	case found >= 2:
		return "partial"
	case found == 1:
		return "sparse"
	default:
		return "none"
	}
}

// joinBaseAndPath returns an absolute URL formed by replacing base's path with
// path. Returns "" if base is invalid.
func joinBaseAndPath(base *url.URL, path string) string {
	if base == nil {
		return ""
	}
	u := *base
	u.Path = path
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func fetchPage(ctx context.Context, client *http.Client, target string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "en;q=0.9,de;q=0.8,fr;q=0.7")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return "", resp.Request.URL.String(), fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	return readBodyLimited(resp), resp.Request.URL.String(), nil
}

func readBodyLimited(resp *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil && !errors.Is(err, io.EOF) {
		return string(body)
	}
	return string(body)
}

// normaliseIDN converts a Unicode-form (U-label) host on u to its ASCII A-label
// (punycode) in place, using the IDNA2008 lookup profile. Without this Go
// percent-encodes the raw UTF-8 host bytes ("bücher.de" → "b%C3%BCcher.de"),
// which neither the DNS resolver nor the fleet fetch proxy can resolve — so the
// homepage fetch silently fails for every non-ASCII domain. That is a pure
// geo-bias bug: only non-Western registrants type U-label hosts. Pure Go,
// CGO-free, no new outbound calls; an already-ASCII or unconvertible host is
// left unchanged.
func normaliseIDN(u *url.URL) {
	h := u.Hostname()
	if h == "" || isASCII(h) {
		return
	}
	ascii, err := idna.Lookup.ToASCII(h)
	if err != nil || ascii == "" {
		return
	}
	u.Host = rebuildHost(ascii, u.Port())
}

// isASCII reports whether s contains only 7-bit ASCII bytes. Used to gate the
// (relatively expensive) IDNA conversion to the rare non-ASCII-host case.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false // any non-ASCII byte → not ASCII
		}
	}
	return true
}

// rebuildHost reassembles a host:port from an (already ASCII) host and an
// optional port. Empty port yields a bare host.
func rebuildHost(host, port string) string {
	if port == "" {
		return host
	}
	return net.JoinHostPort(host, port)
}

func firstResolvedIP(ctx context.Context, host string) string {
	if host == "" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	ips, err := (&net.Resolver{}).LookupIP(lookupCtx, "ip", host)
	if err != nil || len(ips) == 0 {
		return ""
	}
	return ips[0].String()
}
