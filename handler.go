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
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/baditaflorin/go-common/fleetfetch"
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
	c := fleetfetch.NewHTTPClient(fleetfetch.WithRender(renderMode()))
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
	resp.Detection.RenderMode = renderMode()
	if resp.Detection.RenderMode == "" {
		resp.Detection.RenderMode = "direct"
	}

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
	resp.Detection.HomepageFetched = true

	base, _ := url.Parse(finalURL)
	if base == nil {
		base = u
	}

	// Phase 1: scan links in the rendered HTML.
	linkHits := linkScan(html, base)
	resp.Detection.LinksScanned = len(linkHits)

	// Phase 2: for any expected doc type missing from linkHits, probe a small
	// set of canonical paths — now content-verified to reject soft-404s.
	missing := make([]DocType, 0)
	for _, t := range documentTypeOrder {
		if hubDocTypes[t] {
			continue
		}
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
	var probedCount, rejectedCount int

	g, gctx := errgroup.WithContext(probeCtx)
	g.SetLimit(5)

	now := time.Now()
	for _, cand := range probeList {
		cand := cand
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
			pm, perr := probeVerify(gctx, probeClient, probeURL)
			probedMu.Lock()
			probedCount++
			probedMu.Unlock()
			if perr != nil {
				return nil
			}
			if pm.status < 200 || pm.status >= 400 {
				return nil
			}
			vr := classifyBody(pm.body, pm.status, cand.DocType)
			if !vr.IsReal {
				probedMu.Lock()
				rejectedCount++
				probedMu.Unlock()
				return nil
			}
			staleness, isoDate := stalenessOf(pm.lastMod, now)
			hit := DocFinding{
				Type:         cand.DocType,
				URL:          probeURL,
				Status:       pm.status,
				ContentLen:   pm.contentLen,
				LastModified: isoDate,
				Staleness:    staleness,
				Source:       "probed_path",
				Confidence:   vr.Confidence,
				Title:        vr.Title,
				Evidence:     vr.Evidence,
			}
			probedMu.Lock()
			if cur, exists := probedHits[cand.DocType]; !exists || confidenceRank(vr.Confidence) > confidenceRank(cur.Confidence) {
				probedHits[cand.DocType] = hit
			}
			probedMu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	resp.Detection.PathsProbed = probedCount
	resp.Detection.ProbesRejected = rejectedCount

	// Phase 3: for each link-discovered doc, GET-verify to capture status,
	// freshness, and content confidence (rejecting links that point at
	// soft-404s). A footer link is strong location evidence, so an unconfirmed
	// real page still keeps at least low confidence.
	g2, g2ctx := errgroup.WithContext(probeCtx)
	g2.SetLimit(5)
	var linkMu sync.Mutex

	for t, hit := range linkHits {
		t, hit := t, hit
		g2.Go(func() error {
			pm, perr := probeVerify(g2ctx, probeClient, hit.URL)
			if perr != nil || pm.status == 0 {
				// Could not verify (fetch noise) — keep the link finding but
				// mark its confidence low; the link itself is real evidence.
				if hit.Confidence == "" {
					hit.Confidence = ConfLow
				}
				linkMu.Lock()
				linkHits[t] = hit
				linkMu.Unlock()
				return nil
			}
			staleness, isoDate := stalenessOf(pm.lastMod, now)
			hit.Status = pm.status
			hit.ContentLen = pm.contentLen
			hit.LastModified = isoDate
			hit.Staleness = staleness
			if pm.status >= 200 && pm.status < 400 {
				vr := classifyBody(pm.body, pm.status, t)
				if !vr.IsReal {
					// Footer link to a soft-404: downgrade but keep — the link
					// existing is itself a (weak) signal the site references it.
					hit.Confidence = ConfLow
					hit.Evidence = append([]string{"link_target_soft404"}, vr.Evidence...)
				} else {
					if vr.Title != "" {
						hit.Title = vr.Title
					}
					hit.Confidence = vr.Confidence
					if confidenceRank(vr.Confidence) < confidenceRank(ConfMedium) {
						// An explicit footer/page link to a real page is at
						// least medium confidence regardless of body vocab.
						hit.Confidence = ConfMedium
					}
					hit.Evidence = append(hit.Evidence, vr.Evidence...)
				}
			} else {
				hit.Confidence = ConfLow
			}
			linkMu.Lock()
			linkHits[t] = hit
			linkMu.Unlock()
			return nil
		})
	}
	_ = g2.Wait()

	// Merge: prefer link hits over probed hits (an explicit link is stronger
	// location evidence than a guess), but if a probe found a higher-confidence
	// verified document, keep the probe.
	merged := make(map[DocType]DocFinding)
	for k, v := range probedHits {
		merged[k] = v
	}
	for k, v := range linkHits {
		if cur, ok := merged[k]; ok && confidenceRank(cur.Confidence) > confidenceRank(v.Confidence) {
			continue
		}
		merged[k] = v
	}

	// Build documents[] in documentTypeOrder, and confidence tallies.
	docs := make([]DocFinding, 0, len(merged))
	for _, t := range documentTypeOrder {
		if d, ok := merged[t]; ok {
			docs = append(docs, d)
			switch d.Confidence {
			case ConfHigh:
				resp.Detection.HighConfidence++
			case ConfMedium:
				resp.Detection.MediumConfidence++
			case ConfLow:
				resp.Detection.LowConfidence++
			}
		}
	}

	// Missing is computed against the *expected* set, not every type.
	missingFinal := make([]DocType, 0)
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
