package main

import (
	"context"
	"errors"
	"hash/fnv"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/baditaflorin/go-common/meshresult"
	"golang.org/x/sync/errgroup"
)

// maxInflight bounds the scans running at once in this process. A scan is
// mostly I/O wait, so the cap is generous; it exists so that a burst (DomainScope
// fans out hundreds of domains) cannot grow memory without bound or starve the
// CPU until every scan misses the caller's 30 s deadline. A scan that cannot get
// a slot within slotWait is answered 503 "overloaded" immediately (honest,
// cheap) instead of being queued until the caller gives up.
var (
	maxInflight = envInt("TOS_FINDER_MAX_INFLIGHT", 32)
	slotWait    = 2 * time.Second
	inflightSem = make(chan struct{}, maxInflight)
)

func envInt(name string, def int) int {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func acquireSlot(ctx context.Context) bool {
	select {
	case inflightSem <- struct{}{}:
		return true
	default:
	}
	t := time.NewTimer(slotWait)
	defer t.Stop()
	select {
	case inflightSem <- struct{}{}:
		return true
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func releaseSlot() { <-inflightSem }

// wwwAlternate returns the https://www.<host> variant of u, or nil when u is an
// IP, already www., or carries an explicit port.
func wwwAlternate(u *url.URL) *url.URL {
	h := u.Hostname()
	if h == "" || net.ParseIP(h) != nil || strings.HasPrefix(strings.ToLower(h), "www.") || u.Port() != "" || !strings.Contains(h, ".") {
		return nil
	}
	a := *u
	a.Scheme = "https"
	a.Host = "www." + h
	return &a
}

// fetchKind buckets a fetch error so the fallback policy can decide whether a
// different origin is worth trying. A timeout is NOT retried on another origin
// (the target is slow; a second 10 s would only burn the budget).
func fetchKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return "timeout"
	}
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "no such host"):
		return "dns"
	case strings.Contains(s, "tls:") || strings.Contains(s, "x509") || strings.Contains(s, "certificate") || strings.Contains(s, "handshake"):
		return "tls"
	case strings.Contains(s, "connection refused"):
		return "refused"
	case strings.Contains(s, "i/o timeout") || strings.Contains(s, "timeout") || strings.Contains(s, "deadline"):
		return "timeout"
	case strings.Contains(s, "eof") || strings.Contains(s, "connection reset"):
		return "reset"
	}
	return "other"
}

// homepageResult is a successfully fetched homepage.
type homepageResult struct {
	html     string
	finalURL string
	status   int
	fallback string // "", "www" or "http"
}

// fetchHomepage fetches the homepage within ctx. Policy (the old one retried
// every transient error once, so a dead origin cost 2 x 12 s and starved the
// whole budget):
//   - one attempt, <= homepageTimeout;
//   - ONE immediate retry only for a fast (< 3 s) reset/EOF/5xx — never for a
//     timeout;
//   - when https://<domain> itself fails with a TLS-cert / refused / reset / DNS
//     error, try https://www.<domain> and then http://<domain>: many sites only
//     serve a valid certificate (or any listener) on www (amf-france.org,
//     anthonyraneri.com in the 150-domain sample), and a bare "domain" is all
//     DomainScope passes.
func fetchHomepage(ctx context.Context, client *http.Client, u *url.URL) (homepageResult, error) {
	var hp homepageResult
	attempt := func(target string) (string, string, int, error) {
		actx, cancel := context.WithTimeout(ctx, homepageTimeout)
		defer cancel()
		return fetchPage(actx, client, target)
	}
	tries := []struct{ target, tag string }{{u.String(), ""}}
	var firstErr error
	for idx := 0; idx < len(tries); idx++ {
		start := time.Now()
		body, finalURL, status, err := attempt(tries[idx].target)
		if err != nil && isTransientFetchError(err) && fetchKind(err) != "timeout" && time.Since(start) < 3*time.Second && ctx.Err() == nil {
			select {
			case <-ctx.Done():
			case <-time.After(300 * time.Millisecond):
				body, finalURL, status, err = attempt(tries[idx].target)
			}
		}
		if err == nil {
			hp = homepageResult{html: body, finalURL: finalURL, status: status, fallback: tries[idx].tag}
			return hp, nil
		}
		if firstErr == nil {
			firstErr = err
		}
		if idx == 0 && ctx.Err() == nil {
			switch fetchKind(err) {
			case "tls", "refused", "reset", "dns":
				if alt := wwwAlternate(u); alt != nil {
					tries = append(tries, struct{ target, tag string }{alt.String(), "www"})
				}
				if u.Scheme == "https" {
					h := *u
					h.Scheme = "http"
					tries = append(tries, struct{ target, tag string }{h.String(), "http"})
				}
			}
		}
		// Do not start another origin with less than ~4 s left.
		if dl, ok := ctx.Deadline(); ok && time.Until(dl) < 4*time.Second {
			break
		}
	}
	return hp, firstErr
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

	// IDN normalisation: a Unicode-form host (U-label, e.g. "bücher.de",
	// "日本.jp", "россия.рф") must be converted to its ASCII A-label
	// (punycode) before fetch/DNS. See normaliseIDN.
	normaliseIDN(u)

	resp := Response{Target: u.String()}
	resp.Detection.RenderMode = renderMode()
	if resp.Detection.RenderMode == "" {
		resp.Detection.RenderMode = "direct"
	}

	// One wall-clock budget for the whole scan (see totalBudget). Every phase
	// below, including the DNS pre-check that used to run on the bare request
	// context, is bounded by it.
	ctx, cancel := context.WithTimeout(r.Context(), totalBudget)
	defer cancel()

	if !acquireSlot(ctx) {
		resp.Result = string(meshresult.OutcomeError)
		resp.Reason = "overloaded"
		resp.Status = StatusUnreachable
		resp.Verdict = "unknown"
		w.Header().Set("Retry-After", "5")
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	defer releaseSlot()

	if _, err := checkURLRetry(ctx, u.String()); err != nil {
		// Distinguish "target unreachable" (NXDOMAIN / no records — the target
		// simply does not resolve) from a genuine policy reject (private IP,
		// invalid scheme). The former is data, not a service error: record it
		// as unreachable (404) so domainscope doesn't log upstream_error
		// for every dead domain. Only an actual SSRF/scheme block stays 400.
		if isUnreachableCheckError(err) {
			// A bare apex that does not resolve may still exist as www.<apex>.
			if alt := wwwAlternate(u); alt != nil && fetchKind(err) == "dns" {
				if _, err2 := checkURLRetry(ctx, alt.String()); err2 == nil {
					u = alt
					resp.Target = u.String()
					resp.Detection.FallbackOrigin = "www"
					err = nil
				}
			}
			if err != nil {
				writeUnreachable(w, resp, err)
				return
			}
		} else {
			writeJSON(w, http.StatusBadRequest, addError(resp, err))
			return
		}
	}

	ipCh := make(chan string, 1)
	go func() { ipCh <- firstResolvedIP(ctx, u.Hostname()) }()
	collectIP := func() {
		select {
		case ip := <-ipCh:
			resp.ResolvedIP = ip
		default:
		}
	}

	client := httpClient()
	hp, err := fetchHomepage(ctx, client, u)
	if err != nil {
		// Homepage could not be fetched — the target is unreachable, not an
		// internal fault. Classified via meshresult so the HTTP status follows
		// the fleet contract (404/504/502, never false-OK).
		collectIP()
		writeUnreachable(w, resp, err)
		return
	}
	html, finalURL, homepageStatus := hp.html, hp.finalURL, hp.status
	if hp.fallback != "" {
		resp.Detection.FallbackOrigin = hp.fallback
	}
	resp.FetchedURL = finalURL
	resp.Detection.HomepageFetched = true
	collectIP()

	// The homepage fetch succeeded (HTTP < 500) but the body may itself be a
	// bot-block / WAF-challenge interstitial (Cloudflare "Just a moment...",
	// a stock Apache/Nginx 403 stub, generic "Access Denied") rather than real
	// site content — see patterns_block.go. Report these honestly as
	// blocked/unverifiable rather than a false "no data".
	if blocked, reason := isHomepageBlocked(html, homepageStatus); blocked {
		writeBlocked(w, resp, homepageStatus, reason)
		return
	}

	base, _ := url.Parse(finalURL)
	if base == nil {
		base = u
	}
	if !sameSiteHost(base, u) {
		resp.Detection.OffsiteRedirect = strings.ToLower(base.Hostname())
	}

	// Phase 1: scan links in the rendered HTML.
	linkHits := linkScan(html, base)
	resp.Detection.LinksScanned = len(linkHits)

	// Phase 2/3 run CONCURRENTLY (they used to run back to back, doubling the
	// worst-case latency): canonical-path probes for the types the homepage did
	// not link, and GET-verification of every link-discovered document. Each has
	// its own bounded deadline so slow speculative probes can never starve the
	// verification of links the homepage actually exposed (an explicit link is
	// stronger evidence than a guess). A single request holds at most
	// probeWorkers+linkWorkers fetches in flight.
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

	st := newScanState(html)
	now := time.Now()
	probeCtx, probeCancel := context.WithTimeout(ctx, probePhaseTimeout)
	defer probeCancel()
	linkCtx, linkCancel := context.WithTimeout(ctx, probePhaseTimeout)
	defer linkCancel()

	var wg sync.WaitGroup
	probedHits := make(map[DocType]DocFinding)
	var probedMu sync.Mutex
	var probedCount, rejectedCount int

	wg.Add(1)
	go func() {
		defer wg.Done()
		g, gctx := errgroup.WithContext(probeCtx)
		g.SetLimit(probeWorkers)
		for rank, cand := range probeList {
			cand, rank := cand, rank
			probedMu.Lock()
			_, already := probedHits[cand.DocType]
			probedMu.Unlock()
			if already || st.rateLimited.Load() || gctx.Err() != nil {
				continue
			}
			probeURL := joinBaseAndPath(base, cand.Path)
			if probeURL == "" {
				continue
			}
			g.Go(func() error {
				// A type may have been satisfied by an earlier probe while this one
				// waited for a worker slot.
				probedMu.Lock()
				_, done := probedHits[cand.DocType]
				probedMu.Unlock()
				if done || st.rateLimited.Load() {
					return nil
				}
				pm, perr := probeVerify(gctx, client, probeURL)
				probedMu.Lock()
				probedCount++
				probedMu.Unlock()
				if perr != nil {
					return nil
				}
				if st.noteStatus(pm.status) {
					return nil
				}
				if pm.status < 200 || pm.status >= 400 {
					return nil
				}
				view := st.view(pm.body)
				vr := classifyView(view, pm.status, cand.DocType)
				if vr.IsReal {
					st.noteProbeFP(view.fp, cand.DocType)
				}
				if !vr.IsReal || st.sameAsHomepage(pm.body) {
					probedMu.Lock()
					rejectedCount++
					probedMu.Unlock()
					return nil
				}
				// False-positive guard for canonical probes: a guessed path that
				// merely "exists" (low, page_exists_unconfirmed) but carries no
				// type-specific vocabulary is almost always an unrelated page (or a
				// large wildcard catch-all that slipped past the soft-404 gate) —
				// e.g. /shipping or /refunds 200ing on a site that sells nothing.
				// Unlike an explicit link, a probe has no independent location
				// evidence, so reject it. Link-discovered hits are handled below
				// and are never subject to this guard.
				if confidenceRank(vr.Confidence) <= confidenceRank(ConfLow) && !view.hasTypeSignal(cand.DocType) {
					probedMu.Lock()
					rejectedCount++
					probedMu.Unlock()
					return nil
				}
				// Last line of defence: a generic template that answers any path.
				if st.isTemplateEcho(gctx, client, base, view, cand.Path) {
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
					probeRank:    rank,
					fp:           view.fp,
				}
				if cand.DocType == DocImprint {
					// vr.IsReal is already confirmed true above — capture the
					// verified body so extractImprintFields can run on it
					// without a second fetch. See DocFinding.Body.
					hit.Body = pm.body
				}
				probedMu.Lock()
				if cur, exists := probedHits[cand.DocType]; !exists ||
					confidenceRank(vr.Confidence) > confidenceRank(cur.Confidence) ||
					(confidenceRank(vr.Confidence) == confidenceRank(cur.Confidence) && rank < cur.probeRank) {
					probedHits[cand.DocType] = hit
				}
				probedMu.Unlock()
				return nil
			})
		}
		_ = g.Wait()
	}()

	// Link verification. Results go into a SEPARATE map (ranging over linkHits
	// while goroutines write back into it is a data race); pre-seeded with the
	// discovered hits so a goroutine the limiter never started (ctx cancelled)
	// still contributes its link-only evidence.
	var linkMu sync.Mutex
	linkResults := make(map[DocType]DocFinding, len(linkHits))
	for t, hit := range linkHits {
		linkResults[t] = hit
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		g2, g2ctx := errgroup.WithContext(linkCtx)
		g2.SetLimit(linkWorkers)
		for t, hit := range linkHits {
			t, hit := t, hit
			g2.Go(func() error {
				out := verifyLink(g2ctx, client, st, t, hit, now)
				linkMu.Lock()
				linkResults[t] = out
				linkMu.Unlock()
				return nil
			})
		}
		_ = g2.Wait()
	}()
	wg.Wait()
	// Catch-all probe hits: a page fingerprint answered for two or more document
	// types (or the homepage itself) is a wildcard route, not a document.
	for t, h := range probedHits {
		if st.catchAll(h.fp) {
			delete(probedHits, t)
			rejectedCount++
		}
	}
	resp.Detection.PathsProbed = probedCount
	resp.Detection.ProbesRejected = rejectedCount
	resp.Detection.RateLimited = st.rateLimited.Load()
	resp.Detection.BudgetExhausted = ctx.Err() != nil || probeCtx.Err() == context.DeadlineExceeded || linkCtx.Err() == context.DeadlineExceeded
	linkHits = linkResults

	// Dead links (404/410/soft-404) are reported separately, never as documents.
	for t, d := range linkHits {
		if d.rejected {
			reason := "dead_link"
			if len(d.Evidence) > 0 {
				reason = d.Evidence[0]
			}
			resp.RejectedLinks = append(resp.RejectedLinks, RejectedLink{Type: t, URL: d.URL, Reason: reason})
			delete(linkHits, t)
		}
	}
	sort.Slice(resp.RejectedLinks, func(i, j int) bool { return resp.RejectedLinks[i].Type < resp.RejectedLinks[j].Type })

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

	// Drop weak unconfirmed catch-all hits (the headline FP fix): a non-hub
	// finding that only earned "page_exists_unconfirmed" on a large generic /
	// SPA body, with no medium link-confidence floor, is navigation noise, not
	// a legal document — don't claim it at all. Hubs are kept as legal-surface
	// evidence (they're never counted as documents anyway).
	for t, d := range merged {
		if hubDocTypes[t] {
			continue
		}
		if isWeakUnconfirmed(d) {
			delete(merged, t)
		}
	}
	discardStandaloneDisclaimer(merged)

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
	if imprintHit, ok := merged[DocImprint]; ok {
		hostname := ""
		if base != nil {
			hostname = base.Hostname()
		}
		resp.Imprint = extractImprintFields(imprintHit.URL, imprintHit.Body, hostname)
	}
	resp.ImprintPresent = resp.Imprint.Present
	resp.Verdict = verdictOfDocs(docs)

	// Extraction-service no-data rule: the homepage was reached and scanned,
	// but no VERIFIED legal document was found (Round 2 tightened
	// documents_found to verified doc links only — see countSuccess). "Reached
	// but genuinely empty" is meshresult.OutcomeNoData → HTTP 404, so
	// domainscope records NoData rather than a false-OK 200 over an empty
	// documents[]. The full scan body (detection trail, empty documents[],
	// verdict "none") is preserved for the evidence trail.
	if resp.DocumentsFound == 0 {
		writeNoData(w, resp)
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// probeWorkers / linkWorkers bound one scan's concurrent fetches.
const (
	probeWorkers = 4
	linkWorkers  = 4
)

// scanState is the per-request shared state of the verification phases.
type scanState struct {
	rateLimited atomic.Bool

	mu       sync.Mutex
	views    map[uint64]*bodyView
	homeHash uint64
	homeFP   uint64
	// fpTypes records, per page fingerprint (title + visible text), which
	// document types' canonical probes answered with that exact page. A real site
	// never serves the same text as its /terms AND its /dmca; a fingerprint
	// shared by probes of two or more types is a catch-all route (bufaloscr.com
	// answers every path with its 45 KB SPA shell, differing only in a nonce, so
	// a byte hash cannot see it) and carries no evidence about any of them.
	fpTypes map[uint64]map[DocType]bool

	ctrlOnce sync.Once
	ctrlView *bodyView
	ctrlTok  string
	ctrlOK   bool
}

func newScanState(homepage string) *scanState {
	st := &scanState{views: map[uint64]*bodyView{}, fpTypes: map[uint64]map[DocType]bool{}}
	hp := truncateUTF8(homepage, verifyBodyBytes)
	st.homeHash = bodyHash(hp)
	st.homeFP = analyzeBody(hp).fp
	return st
}

// controlToken is a deterministic per-host nonexistent path token (random-looking
// but stable so recorded runs replay identically).
func controlToken(host string) string {
	return "tosf-" + strconv.FormatUint(bodyHash(host)%0xffffffff, 16)
}

// control lazily fetches ONE page at a path that cannot exist and remembers the
// answer. It is only requested when a canonical probe is about to be accepted.
func (st *scanState) control(ctx context.Context, client *http.Client, base *url.URL) (*bodyView, string, bool) {
	st.ctrlOnce.Do(func() {
		tok := controlToken(base.Hostname())
		pm, err := probeVerify(ctx, client, joinBaseAndPath(base, "/"+tok))
		if err != nil || pm.status < 200 || pm.status >= 400 {
			return
		}
		st.ctrlView, st.ctrlTok, st.ctrlOK = st.view(pm.body), tok, true
	})
	return st.ctrlView, st.ctrlTok, st.ctrlOK
}

// pathToken is the lower-cased last path segment without extension.
func pathToken(p string) string {
	p = strings.ToLower(strings.Trim(p, "/"))
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	if i := strings.IndexByte(p, '.'); i > 0 {
		p = p[:i]
	}
	return p
}

// titleTemplate replaces every occurrence of the requested path (full "legal/terms"
// first, then its last segment; hyphen, space and underscore forms) in a title by a
// placeholder; "" when the title does not contain it at all.
func titleTemplate(title, path string) string {
	lt := strings.ToLower(title)
	full := strings.ToLower(strings.Trim(path, "/"))
	if i := strings.IndexByte(full, '.'); i > 0 {
		full = full[:i]
	}
	found := false
	for _, base := range []string{full, pathToken(path)} {
		for _, form := range []string{base, strings.ReplaceAll(base, "-", " "), strings.ReplaceAll(base, "-", "_")} {
			if form != "" && strings.Contains(lt, form) {
				lt = strings.ReplaceAll(lt, form, "\u00a7")
				found = true
			}
		}
	}
	if !found {
		return ""
	}
	return lt
}

// isTemplateEcho reports whether a probed page is the site's generic template
// answering ANY path: the same fingerprint as a nonexistent path, or a title that
// merely echoes the requested path token inside an otherwise identical template
// (dict.cc answers /terms, /dmca and /cookie-policy with dictionary look-ups titled
// "terms | Übersetzung Englisch-Deutsch").
func (st *scanState) isTemplateEcho(ctx context.Context, client *http.Client, base *url.URL, view *bodyView, probePath string) bool {
	cv, ctok, ok := st.control(ctx, client, base)
	if !ok {
		return false
	}
	if cv.fp == view.fp {
		return true
	}
	tp, tc := titleTemplate(view.title, probePath), titleTemplate(cv.title, "/"+ctok)
	return tp != "" && tp == tc
}

// noteProbeFP records that a canonical probe for type t returned a real-looking
// page with fingerprint fp.
func (st *scanState) noteProbeFP(fp uint64, t DocType) {
	st.mu.Lock()
	defer st.mu.Unlock()
	m := st.fpTypes[fp]
	if m == nil {
		m = map[DocType]bool{}
		st.fpTypes[fp] = m
	}
	m[t] = true
}

// catchAll reports whether a fingerprint was served by probes of >= 2 types, or
// equals the homepage's own.
func (st *scanState) catchAll(fp uint64) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return fp == st.homeFP || len(st.fpTypes[fp]) >= 2
}

func bodyHash(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}

// view returns the (memoised) analysis of a body. A site that answers all
// canonical probes with the same SPA shell is analysed once, not once per probe.
func (st *scanState) view(body string) *bodyView {
	k := bodyHash(body)
	st.mu.Lock()
	v, ok := st.views[k]
	st.mu.Unlock()
	if ok {
		return v
	}
	v = analyzeBody(body)
	st.mu.Lock()
	st.views[k] = v
	st.mu.Unlock()
	return v
}

// sameAsHomepage reports whether a probe body is byte-identical to the
// homepage (a catch-all that serves the homepage for every path): it carries no
// evidence about the guessed path.
func (st *scanState) sameAsHomepage(body string) bool {
	return len(body) > minRealDocBytes && bodyHash(body) == st.homeHash
}

// noteStatus records a rate-limit/bot signal. It returns true when the status
// is one (429 or 503), in which case the response says nothing about the path.
func (st *scanState) noteStatus(status int) bool {
	if status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable {
		st.rateLimited.Store(true)
		return true
	}
	return false
}

// isChallengeStatus reports a status that means "the target refused to show us
// this page" rather than "the page does not exist": the link then stays in the
// result as link_unverified instead of being discarded.
func isChallengeStatus(status int) bool {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout,
		http.StatusTooManyRequests, http.StatusServiceUnavailable:
		return true
	}
	return status >= 500
}

// verifyLink GET-verifies one link-discovered document to capture status,
// freshness and content confidence. A footer link is strong location evidence,
// so an unconfirmed real page keeps at least its link-confidence floor; a link
// to a verified-dead page is flagged rejected; a link the target refused to
// serve (403/429/503/challenge, or a fetch failure) stays as link_unverified.
func verifyLink(ctx context.Context, client *http.Client, st *scanState, t DocType, hit DocFinding, now time.Time) DocFinding {
	unverified := func(why string) DocFinding {
		// Could not verify (fetch noise / proxy timeout / bot challenge). The
		// link itself is location evidence, so fall back to its link-confidence
		// floor rather than always collapsing to low. A footer link whose path
		// AND anchor text both name the type (floor=medium) stays medium even when
		// its body can't be fetched — the common production case behind the fleet
		// fetch proxy.
		if confidenceRank(hit.Confidence) < confidenceRank(hit.LinkConfidence) {
			hit.Confidence = hit.LinkConfidence
		}
		if hit.Confidence == "" {
			hit.Confidence = ConfLow
		}
		hit.Evidence = append(hit.Evidence, "link_unverified:"+why+":floor="+hit.LinkConfidence)
		return hit
	}
	pm, perr := probeVerify(ctx, client, hit.URL)
	if perr != nil || pm.status == 0 {
		return unverified("fetch_failed")
	}
	staleness, isoDate := stalenessOf(pm.lastMod, now)
	hit.Status = pm.status
	hit.ContentLen = pm.contentLen
	hit.LastModified = isoDate
	hit.Staleness = staleness
	if st.noteStatus(pm.status) || isChallengeStatus(pm.status) {
		hit.Status = 0 // we never saw the page; do not report the block status as the document's
		return unverified("http_" + strconv.Itoa(pm.status))
	}
	if pm.status >= 200 && pm.status < 400 {
		view := st.view(pm.body)
		if view.challenge {
			// The target answered 2xx with a bot-challenge page: we never saw the
			// document. Same treatment as a 403/429: keep the link, unverified.
			st.rateLimited.Store(true)
			hit.Status = 0
			return unverified("bot_challenge")
		}
		vr := classifyView(view, pm.status, t)
		if vr.IsReal && view.fp == st.homeFP && !view.pdf && confidenceRank(hit.LinkConfidence) < confidenceRank(ConfMedium) {
			// A weakly-evidenced link whose target renders exactly like the
			// homepage is the homepage (or a catch-all), not a document.
			vr.IsReal = false
			vr.Evidence = []string{"rejected:same_as_homepage"}
		}
		if !vr.IsReal {
			// A link to a soft-404 / a page that announces itself as another
			// document: the site references it, but the document is not there.
			hit.Confidence = ConfNone
			hit.Evidence = append([]string{"link_target_" + strings.TrimPrefix(firstEvidence(vr.Evidence), "rejected:")}, vr.Evidence...)
			hit.rejected = true
			return hit
		}
		if vr.Title != "" {
			hit.Title = vr.Title
		}
		if hasEvidenceTok(vr.Evidence, "pdf_document") && confidenceRank(hit.LinkConfidence) < confidenceRank(ConfMedium) &&
			!matchTextOrScript(titleRegexFor(t), scriptRegexFor(t), hit.LinkText) {
			// A PDF carries no type information of its own; with only a weak link
			// signal (anchor text "Sales and delivery terms" on a ToS slot) it is
			// not evidence of the document type.
			hit.Confidence = ConfLow
			hit.Evidence = append(hit.Evidence, "page_exists_unconfirmed", "pdf_weak_link")
			return hit
		}
		hit.Confidence = vr.Confidence
		// An explicit link to a real page is location evidence: hold it to at
		// least its link-confidence floor (medium when path AND anchor text
		// agree; low for a single-signal match).
		if confidenceRank(hit.Confidence) < confidenceRank(hit.LinkConfidence) {
			hit.Confidence = hit.LinkConfidence
		}
		hit.Evidence = append(hit.Evidence, vr.Evidence...)
		if t == DocImprint {
			// Verified real above — capture the body for extractImprintFields.
			hit.Body = pm.body
		}
		return hit
	}
	// 404/410 and other 4xx: the link is dead.
	hit.Confidence = ConfNone
	hit.Evidence = append([]string{"link_target_http_" + strconv.Itoa(pm.status)}, hit.Evidence...)
	hit.rejected = true
	return hit
}

func firstEvidence(ev []string) string {
	if len(ev) == 0 {
		return "unverified"
	}
	return ev[0]
}

func discardStandaloneDisclaimer(merged map[DocType]DocFinding) {
	if _, ok := merged[DocDisclaimer]; !ok {
		return
	}
	for t := range merged {
		if t != DocDisclaimer && !hubDocTypes[t] {
			return
		}
	}
	delete(merged, DocDisclaimer)
}
