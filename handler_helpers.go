package main

import (
	"context"
	"github.com/baditaflorin/go-common/safehttp"
	"golang.org/x/sync/errgroup"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

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

	if _, err := safehttp.CheckURL(r.Context(), u.String()); err != nil {
		// Distinguish "target unreachable" (NXDOMAIN / no records — the target
		// simply does not resolve) from a genuine policy reject (private IP,
		// invalid scheme). The former is data, not a service error: record it
		// as unreachable (404) so domainscope doesn't log upstream_error
		// for every dead domain. Only an actual SSRF/scheme block stays 400.
		if isUnreachableCheckError(err) {
			writeUnreachable(w, resp, err)
			return
		}
		writeJSON(w, http.StatusBadRequest, addError(resp, err))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), requestTimeout+probeTimeout+10*time.Second)
	defer cancel()

	resp.ResolvedIP = firstResolvedIP(ctx, u.Hostname())

	client := newClient()
	html, finalURL, err := fetchPageRetry(ctx, client, u.String())
	if err != nil {
		// Homepage could not be fetched after retries — the target is
		// unreachable, not an internal fault. Classified via meshresult so the
		// HTTP status follows the fleet contract (404/504/502, never false-OK).
		writeUnreachable(w, resp, err)
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
			// False-positive guard for canonical probes: a guessed path that
			// merely "exists" (low, page_exists_unconfirmed) but carries no
			// type-specific vocabulary is almost always an unrelated page (or a
			// large wildcard catch-all that slipped past the soft-404 gate) —
			// e.g. /shipping or /refunds 200ing on a site that sells nothing.
			// Unlike an explicit link, a probe has no independent location
			// evidence, so reject it. Link-discovered hits are handled in
			// Phase 3 and are never subject to this guard.
			if confidenceRank(vr.Confidence) <= confidenceRank(ConfLow) && !bodyHasTypeSignal(pm.body, cand.DocType) {
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
	// Write verified results into a SEPARATE map. Ranging over linkHits while
	// the goroutines below write back into it is a data race (the range read is
	// unsynchronised against the locked writes); collecting into linkResults and
	// merging after Wait() removes it. Pre-seed with the discovered hits so a
	// goroutine that the errgroup limiter never started (ctx cancelled) still
	// contributes its link-only evidence.
	linkResults := make(map[DocType]DocFinding, len(linkHits))
	for t, hit := range linkHits {
		linkResults[t] = hit
	}

	for t, hit := range linkHits {
		t, hit := t, hit
		g2.Go(func() error {
			pm, perr := probeVerify(g2ctx, probeClient, hit.URL)
			if perr != nil || pm.status == 0 {
				// Could not verify (fetch noise / proxy timeout). The link
				// itself is location evidence, so fall back to its
				// link-confidence floor rather than always collapsing to low.
				// A footer link whose path AND anchor text both name the type
				// (floor=medium) stays medium even when its body can't be
				// fetched — this is the common production case behind the
				// fleet fetch proxy.
				if confidenceRank(hit.Confidence) < confidenceRank(hit.LinkConfidence) {
					hit.Confidence = hit.LinkConfidence
				}
				if hit.Confidence == "" {
					hit.Confidence = ConfLow
				}
				hit.Evidence = append(hit.Evidence, "link_unverified:floor="+hit.LinkConfidence)
				linkMu.Lock()
				linkResults[t] = hit
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
					// An explicit link to a real page is location evidence: hold
					// it to at least its link-confidence floor (medium when path
					// AND anchor text agree; low for a single-signal match). This
					// replaces the old blanket "every link is at least medium",
					// which over-promoted single-signal links.
					if confidenceRank(hit.Confidence) < confidenceRank(hit.LinkConfidence) {
						hit.Confidence = hit.LinkConfidence
					}
					hit.Evidence = append(hit.Evidence, vr.Evidence...)
				}
			} else {
				hit.Confidence = ConfLow
			}
			linkMu.Lock()
			linkResults[t] = hit
			linkMu.Unlock()
			return nil
		})
	}
	_ = g2.Wait()
	linkHits = linkResults

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
