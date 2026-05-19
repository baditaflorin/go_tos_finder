package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/baditaflorin/go-common/safehttp"
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
	Type         DocType `json:"type"`
	URL          string  `json:"url"`
	Status       int     `json:"status,omitempty"`
	ContentLen   int64   `json:"content_length,omitempty"`
	LastModified string  `json:"last_modified,omitempty"`
	Staleness    string  `json:"staleness,omitempty"`
	Source       string  `json:"source"`
	LinkText     string  `json:"link_text,omitempty"`
	Match        string  `json:"match,omitempty"`
}

// Response is the JSON payload returned to callers.
type Response struct {
	Tool            string       `json:"tool"`
	Version         string       `json:"Version"`
	Target          string       `json:"target"`
	FetchedURL      string       `json:"fetched_url,omitempty"`
	ResolvedIP      string       `json:"resolved_ip,omitempty"`
	Documents       []DocFinding `json:"documents"`
	DocumentsFound  int          `json:"documents_found"`
	DocumentsMissing []DocType   `json:"documents_missing"`
	ImprintPresent  bool         `json:"imprint_present"`
	Verdict         string       `json:"verdict"`
	Error           string       `json:"error,omitempty"`
}

func newClient() *http.Client {
	c := safehttp.NewClient()
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

// Handler implements the /t/{token}/... routes.
func Handler(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("target"))
	if raw == "" {
		raw = strings.TrimSpace(r.URL.Query().Get("url"))
	}
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, Response{Error: "Missing 'target' or 'url' query parameter"})
		return
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, Response{Target: raw, Error: "invalid URL: " + err.Error()})
		return
	}

	resp := Response{Target: u.String()}

	if _, err := safehttp.CheckURL(r.Context(), u.String()); err != nil {
		writeJSON(w, http.StatusBadRequest, addError(resp, err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout+probeTimeout+10*time.Second)
	defer cancel()

	resp.ResolvedIP = firstResolvedIP(ctx, u.Hostname())

	client := newClient()
	html, finalURL, err := fetchPage(ctx, client, u.String())
	if err != nil {
		writeJSON(w, http.StatusBadGateway, addError(resp, err))
		return
	}
	resp.FetchedURL = finalURL

	base, _ := url.Parse(finalURL)
	if base == nil {
		base = u
	}

	// Phase 1: scan links in the rendered HTML.
	linkHits := linkScan(html, base)

	// Phase 2: for any expected doc type missing from linkHits, probe a small
	// set of canonical paths. Capped at maxProbes.
	missing := make([]DocType, 0)
	for _, t := range documentTypeOrder {
		if _, ok := linkHits[t]; !ok {
			missing = append(missing, t)
		}
	}
	probeList := allCanonicalProbes(missing, maxProbes)

	probeCtx, probeCancel := context.WithTimeout(ctx, probeTimeout)
	defer probeCancel()

	probeClient := newClient()

	probedHits := make(map[DocType]DocFinding)
	var probedMu sync.Mutex

	g, gctx := errgroup.WithContext(probeCtx)
	g.SetLimit(5)

	for _, cand := range probeList {
		cand := cand
		// Skip if we already discovered this type via link scan during this loop
		// (multiple paths per type — first hit wins).
		probedMu.Lock()
		_, already := probedHits[cand.DocType]
		probedMu.Unlock()
		if already {
			continue
		}
		probeURL := joinBaseAndPath(base, cand.Path)
		if probeURL == "" {
			continue
		}
		g.Go(func() error {
			status, clen, lastMod, perr := probeHead(gctx, probeClient, probeURL)
			if perr != nil {
				return nil
			}
			if status >= 400 || status == 0 {
				// Record the negative result only if no other path of this type
				// has succeeded yet, so callers can see what was probed.
				return nil
			}
			staleness, isoDate := stalenessOf(lastMod, time.Now())
			hit := DocFinding{
				Type:         cand.DocType,
				URL:          probeURL,
				Status:       status,
				ContentLen:   clen,
				LastModified: isoDate,
				Staleness:    staleness,
				Source:       "probed_path",
			}
			probedMu.Lock()
			if _, exists := probedHits[cand.DocType]; !exists {
				probedHits[cand.DocType] = hit
			}
			probedMu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	// Phase 3: for each link-discovered doc, fire a HEAD to capture status and
	// Last-Modified (bounded concurrency).
	g2, g2ctx := errgroup.WithContext(probeCtx)
	g2.SetLimit(5)
	var linkMu sync.Mutex

	for t, hit := range linkHits {
		t, hit := t, hit
		g2.Go(func() error {
			status, clen, lastMod, perr := probeHead(g2ctx, probeClient, hit.URL)
			if perr != nil {
				return nil
			}
			staleness, isoDate := stalenessOf(lastMod, time.Now())
			hit.Status = status
			hit.ContentLen = clen
			hit.LastModified = isoDate
			hit.Staleness = staleness
			linkMu.Lock()
			linkHits[t] = hit
			linkMu.Unlock()
			return nil
		})
	}
	_ = g2.Wait()

	// Merge: prefer link hits over probed hits (an explicit footer link is
	// stronger signal than a guess).
	merged := make(map[DocType]DocFinding)
	for k, v := range probedHits {
		merged[k] = v
	}
	for k, v := range linkHits {
		merged[k] = v
	}

	// Build documents[] in documentTypeOrder.
	docs := make([]DocFinding, 0, len(merged))
	missingFinal := make([]DocType, 0)
	for _, t := range documentTypeOrder {
		if d, ok := merged[t]; ok {
			docs = append(docs, d)
		}
	}
	// Missing is computed against the *expected* set, not every type.
	for _, t := range expectedDocTypes {
		if _, ok := merged[t]; !ok {
			missingFinal = append(missingFinal, t)
		}
	}
	sort.Slice(missingFinal, func(i, j int) bool { return string(missingFinal[i]) < string(missingFinal[j]) })

	resp.Documents = docs
	resp.DocumentsFound = countSuccess(docs)
	resp.DocumentsMissing = missingFinal
	_, resp.ImprintPresent = merged[DocImprint]
	resp.Verdict = verdictOf(resp.DocumentsFound, missingFinal)

	writeJSON(w, http.StatusOK, resp)
}

// countSuccess counts docs whose Status is 0 (link-only, no HEAD result) or 2xx.
func countSuccess(docs []DocFinding) int {
	n := 0
	for _, d := range docs {
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
