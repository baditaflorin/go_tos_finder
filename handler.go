package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/baditaflorin/go-common/fleetfetch"
	"github.com/baditaflorin/go-common/meshresult"
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

// probePhaseTimeout bounds one bounded verification phase.  Canonical-path
// probing and verification of explicitly discovered links are separate phases:
// the latter must receive its own budget, rather than inheriting a context that
// the speculative canonical probes may already have exhausted.
//
// Kept as a variable so the timeout boundary can be exercised without a
// ten-second test delay. Production code leaves it at probeTimeout.
var probePhaseTimeout = probeTimeout

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

	// Body is the verified page's raw fetched body (bounded to
	// verifyBodyBytes — see probe.go), captured only for DocImprint so the
	// structured imprint field-extraction pass (imprint.go's
	// extractImprintFields) can run on content already fetched during
	// discovery/verification instead of costing a second HTTP round trip.
	// Internal extraction input — not serialised.
	Body string `json:"-"`
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

// Status classifies the outcome for the caller (domainscope) so a dead /
// unreachable target is never recorded as a service error. All four are
// returned at HTTP 200 — only a genuine internal fault returns 5xx.
//
//	ok          — homepage fetched, scan completed (documents may be empty).
//	unreachable — target could not be reached (NXDOMAIN, connect/TLS/timeout
//	              after retries, upstream 5xx, SSRF-rejected target).
//	no_data     — reserved for "reached but nothing to analyse"; currently
//	              folded into ok with an empty documents[] + verdict "none".
const (
	StatusOK          = "ok"
	StatusUnreachable = "unreachable"
	StatusNoData      = "no_data"
	// StatusBlocked marks a homepage that fetched successfully (HTTP < 500)
	// but whose body is a bot-block / WAF-challenge interstitial (or an
	// unusably empty stub) rather than real site content — see
	// patterns_block.go / isHomepageBlocked. Distinct from StatusNoData: we
	// genuinely could not see the site, so "no legal documents found" would
	// be a false claim of certainty domainscope should not record as a clean
	// negative.
	StatusBlocked = "blocked"
)

// Response is the JSON payload returned to callers.
type Response struct {
	Tool    string `json:"tool"`
	Version string `json:"Version"`
	Target  string `json:"target"`
	Status  string `json:"status"`
	// Result is the fleet-canonical meshresult.Outcome ("ok" / "no_data" /
	// "unreachable" / "timeout" / "error"). It is the orthogonal,
	// machine-readable enricher-level classification domainscope keys on
	// (alongside the HTTP status code). Additive to the historical body:
	// the success shape is unchanged except for this field.
	Result           string       `json:"result,omitempty"`
	Reason           string       `json:"reason,omitempty"`
	FetchedURL       string       `json:"fetched_url,omitempty"`
	ResolvedIP       string       `json:"resolved_ip,omitempty"`
	Documents        []DocFinding `json:"documents"`
	DocumentsFound   int          `json:"documents_found"`
	DocumentsMissing []DocType    `json:"documents_missing"`
	ImprintPresent   bool         `json:"imprint_present"`
	// Imprint is structured, field-level imprint/Impressum compliance
	// evidence (legal name, address, register, VAT + checksum validation,
	// responsible person, completeness against the applicable EU/DE/AT
	// ruleset) — see imprint.go. ImprintPresent above is kept unchanged for
	// backward compatibility and is equal to Imprint.Present.
	Imprint   Imprint          `json:"imprint"`
	Verdict   string           `json:"verdict"`
	Detection DetectionSummary `json:"detection"`
	Error     string           `json:"error,omitempty"`
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

// fallbackClient is the client fleetfetch falls through to whenever a fetch
// is NOT cache-mediated: the shared fleet fetch cache is unreachable, or (via
// WithFallbackOnTimeout below) it answered too slowly. fleetfetch's own
// RoundTripper (see fetchCacheTransport.RoundTrip in go-common) deliberately
// strips the caller's per-request User-Agent before handing headers to this
// path — a real cache-hit-rate win for the CACHE-mediated case, where the
// cache's own crawler UA is what actually reaches origin, so a per-service UA
// would needlessly fragment a cache key meant to be shared fleet-wide. But
// that same stripped header set is also what reaches this fallback client,
// and nothing else sets a User-Agent for it — go-common's own default
// fallback (an unconfigured safehttp.NewClient()) has no UA option set
// either, so the request that's supposed to be a faithful direct fetch
// silently goes out as Go's bare "Go-http-client/1.1" instead of this
// service's honest, RFC-compliant UA.
//
// Verified live and reproducible against a real 2026-07-31 production
// sample domain: chronicpies.com (redirects to chronicco.com, a WordPress
// "Coming Soon" placeholder behind Cloudflare) returns HTTP 403 — classified
// blocked/error by isHomepageBlocked — to a request carrying no/bare-Go UA,
// but HTTP 200 to the byte-identical request carrying this service's own UA
// string. Exactly this fallback path is what every request takes whenever
// the shared fetch cache is degraded or unreachable — the one moment a
// caller most needs a correct, honestly-identified direct fetch — so the
// stripped UA was silently inflating false "blocked" verdicts specifically
// during cache outages/timeouts. WithUserAgent restores it for this path
// only; the common cache-mediated path (and its shared-cache hit rate for
// every other fleet service) is untouched.
//
// WithoutFetchCache additionally keeps this client direct-only: without it,
// a process-wide safehttp DefaultFetchDelegate — installed by server.New
// whenever FLEET_FETCH_CACHE_URL is set, the fleet-standard deployment shape
// — would make this "direct" fallback consult the very same shared fetch
// cache the caller just fell back FROM, defeating the fallback's purpose
// (and risking a cache-outage feedback loop) instead of genuinely reaching
// origin. WithForceHTTP2 restores the HTTP/2 offering safehttp's custom
// dialer otherwise disables (see that option's own doc), matching the
// pre-fleetfetch safehttp.NewClient() behavior this fallback stands in for.
func fallbackClient() *http.Client {
	return safehttp.NewClient(
		safehttp.WithTimeout(requestTimeout),
		safehttp.WithUserAgent(userAgent),
		safehttp.WithoutFetchCache(),
		safehttp.WithForceHTTP2(),
	)
}

func newClient() *http.Client {
	c := fleetfetch.NewHTTPClient(
		fleetfetch.WithRender(renderMode()),
		fleetfetch.WithFallbackOnTimeout(),
		fleetfetch.WithFallbackClient(fallbackClient()),
	)
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

// writeJSON serialises a Response at the given HTTP status. The success body
// shape (HTTP 200) is unchanged from prior rounds EXCEPT that the
// fleet-canonical top-level "result" field is now stamped ("ok" for a 200
// scan) so domainscope can read the meshresult.Outcome. Error/unreachable/
// no_data callers set r.Result + r.Status themselves and pass the
// meshresult HTTP code.
func writeJSON(w http.ResponseWriter, status int, resp Response) {
	resp.Tool = "go_tos_finder"
	resp.Version = Version
	if resp.Status == "" {
		resp.Status = StatusOK
	}
	if resp.Result == "" && status == http.StatusOK {
		resp.Result = string(meshresult.OutcomeOK)
	}
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

// writeUnreachable records a target that could not be reached (NXDOMAIN,
// connect/TLS/timeout after retries, upstream 5xx) as a machine-readable
// status. The fetch error is classified with the fleet-canonical
// meshresult.ClassifyFetchError so the HTTP status follows the fleet contract
// (unreachable/no_data → 404, timeout → 504, upstream error → 502) instead of
// the old false-OK 200. domainscope keys on that HTTP code; the body keeps its
// historical shape (status "unreachable", verdict "unknown", empty documents)
// plus the standardized top-level result+reason. The original error string is
// preserved in `error` for debugging.
func writeUnreachable(w http.ResponseWriter, r Response, err error) {
	outcome, reason := meshresult.ClassifyFetchError(err)
	r.Result = string(outcome)
	r.Reason = reason
	r.Status = StatusUnreachable
	if err != nil {
		r.Error = err.Error()
	}
	r.Verdict = "unknown"
	writeJSON(w, outcome.HTTPCode(), r)
}

// writeNoData records a target that WAS reached and scanned, but where no
// verified legal document was found. For an extraction service this is
// genuinely "reached, no data" — meshresult.OutcomeNoData → HTTP 404 — so
// domainscope records NoData rather than a false-OK 200 carrying an empty
// documents[]. The body keeps its full historical scan shape (documents[],
// detection trail, verdict "none") so the evidence trail is still visible.
func writeNoData(w http.ResponseWriter, r Response) {
	r.Result = string(meshresult.OutcomeNoData)
	if r.Reason == "" {
		r.Reason = "no_legal_documents_found"
	}
	r.Status = StatusNoData
	writeJSON(w, meshresult.OutcomeNoData.HTTPCode(), r)
}

// writeBlocked records a homepage that fetched (HTTP < 500) but whose body is
// a bot-block/WAF-challenge interstitial or an unusably empty stub — see
// isHomepageBlocked. Mapped to meshresult.OutcomeError (HTTP 502): this is
// neither a clean "ok" (we never saw real content) nor a clean "no_data"
// (we don't actually know whether legal documents exist), so domainscope
// should treat it as a retry-worthy upstream condition rather than a
// verified-empty result. The homepageStatus the origin actually returned is
// preserved in Reason for debugging.
func writeBlocked(w http.ResponseWriter, r Response, homepageStatus int, reason string) {
	r.Result = string(meshresult.OutcomeError)
	r.Reason = fmt.Sprintf("homepage_blocked:%s:http_%d", reason, homepageStatus)
	r.Status = StatusBlocked
	r.Verdict = "unknown"
	writeJSON(w, meshresult.OutcomeError.HTTPCode(), r)
}

func addError(r Response, err error) Response {
	r.Error = err.Error()
	r.Verdict = "unknown"
	return r
}

// isUnreachableCheckError reports whether a safehttp.CheckURL failure means the
// target is unreachable (DNS/no-host, NXDOMAIN) rather than a genuine policy
// reject. ErrBlocked (private IP), ErrInvalidScheme, and ErrMissingHost are
// real bad-request/policy rejects and stay HTTP 400; everything else (a DNS
// resolution failure surfaced by CheckURL's GuardHost) is "unreachable".
func isUnreachableCheckError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, safehttp.ErrBlocked) ||
		errors.Is(err, safehttp.ErrInvalidScheme) ||
		errors.Is(err, safehttp.ErrMissingHost) {
		return false
	}
	return true
}

// isTransientFetchError reports whether an error is worth a short retry: proxy
// timeouts / 5xx / transient connection resets, which behind the fleet fetch
// proxy frequently succeed on a second attempt. A definitive NXDOMAIN is not
// retried (it will not become resolvable in 400ms).
func isTransientFetchError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "no such host") {
		return false
	}
	return strings.Contains(s, "deadline") || strings.Contains(s, "timeout") ||
		strings.Contains(s, "timed out") || strings.Contains(s, "status 5") ||
		strings.Contains(s, "connection reset") || strings.Contains(s, "eof") ||
		strings.Contains(s, "502") || strings.Contains(s, "503") || strings.Contains(s, "504")
}

// checkURLRetry validates target with safehttp.CheckURL, retrying once on a
// transient DNS failure. safehttp.GuardHost documents its own single-shot
// 3s resolver call as non-cached on failure specifically so "the caller
// [can] retry/resolve again" — but the original call site here never did,
// so a live, perfectly-reachable domain (confirmed against stripe.com and
// github.com, not just the flagged low-completion domains) that hits one
// slow/flaky resolver window is permanently recorded as unreachable with
// zero retry, even though the exact same request succeeds on a second try
// moments later. This mirrors fetchPageRetry's existing bounded-retry
// pattern for the homepage GET later in the same handler. A definitive
// policy reject (blocked/invalid scheme/missing host) or a real NXDOMAIN
// ("no such host") is never retried — see isUnreachableCheckError /
// isTransientFetchError.
func checkURLRetry(ctx context.Context, target string) (*url.URL, error) {
	const maxAttempts = 2 // 1 initial + 1 retry; bounded, matches fetchPageRetry
	var u *url.URL
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return u, err
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		u, err = safehttp.CheckURL(ctx, target)
		if err == nil {
			return u, nil
		}
		if !isUnreachableCheckError(err) || !isTransientFetchError(err) {
			return u, err
		}
	}
	return u, err
}

// isWeakUnconfirmed reports whether a finding is a low-confidence hit whose
// body WAS fetched and verified real, but carried NO type-specific vocabulary —
// i.e. it earned only `page_exists_unconfirmed`. These are the headline false
// positives: a footer "Trademark" / "Data Protection" link or a guessed path
// that resolves to a large generic / SPA catch-all body (cloudflare's 400KB+
// /cookie-policy, /trust-hub/gdpr, /trademark all returning the same homepage
// SPA). The page genuinely exists but is not the claimed legal document, so the
// hit over-claims and is dropped.
//
// Deliberately NARROW: a finding kept at medium+ (title or legal-vocab match),
// or one whose body could NOT be fetched and is held up only by the link itself
// (evidence `link_unverified:...` — a real footer link is location evidence the
// site references that doc), is NOT weak and is retained.
func isWeakUnconfirmed(d DocFinding) bool {
	if confidenceRank(d.Confidence) >= confidenceRank(ConfMedium) {
		return false
	}
	if confidenceRank(d.LinkConfidence) >= confidenceRank(ConfMedium) {
		return false
	}
	hasUnconfirmedBody := false
	for _, e := range d.Evidence {
		switch {
		case strings.HasPrefix(e, "title_matches_type"),
			strings.HasPrefix(e, "legal_vocabulary"),
			strings.HasPrefix(e, "link_unverified"),
			strings.HasPrefix(e, "link_target_soft404"):
			// Corroborated, or held up by the link itself / explicitly a soft404
			// link the site references — not a generic-catch-all over-claim.
			return false
		case strings.HasPrefix(e, "page_exists_unconfirmed"):
			hasUnconfirmedBody = true
		}
	}
	return hasUnconfirmedBody
}

// hasEvidence reports whether finding d carries an evidence token with the
// given prefix.
func hasEvidence(d DocFinding, prefix string) bool {
	for _, e := range d.Evidence {
		if strings.HasPrefix(e, prefix) {
			return true
		}
	}
	return false
}

// countSuccess counts docs that are present (link-only with no HEAD result, or
// a verified 2xx) and carry real evidence. Excluded: hub directory pages, weak
// generic-catch-all hits, and footer links whose target turned out to be a
// soft-404 (the link exists but the document does NOT — counting it would be a
// "claimed a doc that 404s" false positive). Such soft-404 links are still
// LISTED in documents[] for transparency, just not counted as found.
func countSuccess(docs []DocFinding) int {
	n := 0
	for _, d := range docs {
		if hubDocTypes[d.Type] {
			continue
		}
		if isWeakUnconfirmed(d) {
			continue
		}
		if hasEvidence(d, "link_target_soft404") {
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

// fetchPageRetry fetches the homepage, retrying transient upstream failures
// (proxy timeout / 5xx / reset) up to 2 extra times with a short backoff,
// deadline-bounded by ctx. This recovers real data for live domains whose first
// fetch trips the fleet fetch proxy's intermittent 504 — the dominant cause of
// the service's false upstream_error rate.
func fetchPageRetry(ctx context.Context, client *http.Client, target string) (string, string, int, error) {
	const maxAttempts = 2 // 1 initial + 1 retry; kept bounded under the outer ctx
	var body, finalURL string
	var status int
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", finalURL, status, err
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		body, finalURL, status, err = fetchPage(ctx, client, target)
		if err == nil {
			return body, finalURL, status, nil
		}
		if !isTransientFetchError(err) {
			return body, finalURL, status, err
		}
	}
	return body, finalURL, status, err
}

func fetchPage(ctx context.Context, client *http.Client, target string) (string, string, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", "", 0, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	req.Header.Set("Accept-Language", "en;q=0.9,de;q=0.8,fr;q=0.7")
	resp, err := client.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return "", resp.Request.URL.String(), resp.StatusCode, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	return readBodyLimited(resp), resp.Request.URL.String(), resp.StatusCode, nil
}

// readBodyLimited reads the response body (capped to maxBodyBytes) and
// decodes it to UTF-8 when the origin declared (via Content-Type header or
// an in-body <meta charset>/http-equiv tag) a different encoding. Real,
// still-common legacy encodings (ISO-8859-1/Windows-1252 on older European
// sites, Shift_JIS/EUC-KR/GBK on older Japanese/Korean/Chinese sites) were
// previously read as raw bytes and treated as UTF-8 by every regex in this
// package (patterns.go, patterns_script.go) and by encoding/json on the way
// out — silently corrupting non-ASCII title/link-text/evidence content and
// making the script-aware CJK/Cyrillic matchers (added for exactly this
// geo-coverage reason) unable to fire against a body that isn't actually
// UTF-8. Only transcodes when the encoding is explicitly DECLARED and
// `certain` (charset.DetermineEncoding's own confidence signal); an
// undeclared/ambiguous body is left untouched exactly as before, so this
// cannot introduce new corruption on already-UTF-8 pages that lack a
// declaration.
func readBodyLimited(resp *http.Response) string {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil && !errors.Is(err, io.EOF) {
		return decodeToUTF8(body, resp.Header.Get("Content-Type"))
	}
	return decodeToUTF8(body, resp.Header.Get("Content-Type"))
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
