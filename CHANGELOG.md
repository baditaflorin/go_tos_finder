# Changelog

All notable changes to this service are recorded here, newest first.

## 1.4.2 — 2026-08-04

### Fixed (real production verification-budget bug)

- **Explicit legal-document links inherited an expired canonical-probe
  deadline.** The finder first uses a bounded, speculative canonical-path
  probe phase, then verifies legal-document links actually exposed by the
  homepage. Both phases shared the same 10-second context, so a slow 30-path
  canonical pass could leave every footer/page-link verification immediately
  cancelled. In a deterministic fresh replay of 500 production domains, 386
  reached the canonical-probe cap and 22 exposed legal-document links that
  remained `link_unverified` for this reason. The link-verification phase now
  receives its own bounded deadline; explicit links remain subject to the
  existing soft-404 and vocabulary checks. The regression test forces the
  canonical phase to time out and proves the exposed privacy link is then
  content-verified rather than silently downgraded.

## 1.4.1 — 2026-07-31

### Fixed (real production bug, found via a fresh live sample during a genuine TRL re-audit)

- **Fallback-fetch User-Agent stripped, causing false "blocked"/"unreachable"
  results.** `newClient()` builds its `*http.Client` via
  `fleetfetch.NewHTTPClient`, whose `RoundTripper` deliberately strips the
  caller's per-request `User-Agent` (and `Accept-Encoding`) before handing
  headers to the shared fleet fetch cache — a real cache-hit-rate win when
  the fetch is cache-mediated, since the cache's own crawler UA is what
  actually reaches origin. But that same stripped header set is also what
  reaches fleetfetch's *direct* fallback fetch — the path taken whenever the
  shared cache is unreachable or (via `WithFallbackOnTimeout`, which this
  service enables) answers too slowly — and nothing else set a User-Agent on
  that path, since go-common's own default fallback client
  (`safehttp.NewClient(safehttp.WithTimeout(...))`) has no UA configured
  either. The request that's supposed to be a faithful, honestly-identified
  direct fetch silently went out as Go's bare `Go-http-client/1.1` instead.
  Verified live and reproducible against a real production sample domain:
  `chronicpies.com` (redirects to `chronicco.com`, a WordPress "Coming Soon"
  placeholder behind Cloudflare) returned HTTP 403 — classified
  `status:"blocked"` — to the bare-Go-UA request, but HTTP 200 (real content)
  to the byte-identical request carrying this service's own UA. Since this
  fallback path is exactly what runs whenever the shared cache is degraded —
  the one moment a caller most needs a correct direct fetch — the stripped
  UA was silently inflating false `blocked`/`unreachable` verdicts during
  cache outages/timeouts specifically. `newClient()` now supplies fleetfetch
  an explicit fallback client
  (`fleetfetch.WithFallbackClient`) built with `safehttp.WithUserAgent`,
  `safehttp.WithoutFetchCache`, and `safehttp.WithForceHTTP2` — restoring
  the correct UA on the fallback path (measured before/after: `chronicpies.com`
  blocked→no_data, `fixmycity.de` unreachable→ok/2 real documents found, same
  live re-run), while leaving the common cache-mediated path — and its
  shared-cache hit rate for every other fleet service — untouched.
  `WithoutFetchCache` additionally stops this fallback client from consulting
  the process-wide default fetch delegate a fleet deployment installs
  whenever `FLEET_FETCH_CACHE_URL` is set, which would otherwise route this
  "direct" fallback right back through the same (degraded) shared cache
  instead of genuinely reaching origin.

## 1.4.0 — 2026-07-29

### Fixed (real production false negatives, found via a live 1000-domain sample)

- **Homepage bot-block / WAF-challenge false negative.** The homepage fetch
  only treated HTTP >=500 as an error; a Cloudflare "Just a moment..." JS
  challenge, a stock Apache/Nginx 403 stub, or a generic WAF "Access Denied"
  page all return HTTP 200/403 with a real (non-empty) body, so the
  interstitial was fed straight into link scanning — which correctly found
  zero legal-document links in it — and the target was recorded as
  verdict:"none", indistinguishable from a domain that genuinely has no legal
  pages. A live re-fetch of 202 real "no_data" production domains found 15
  (~7%) were actually bot-blocked, not policy-free. New `isHomepageBlocked`
  (patterns_block.go) recognises the closed set of real interstitial
  signatures observed and reports these honestly as `status:"blocked"` /
  `result:"error"` / `verdict:"unknown"` (HTTP 502) instead of a false
  no-data claim.
- **Non-UTF-8 charset corruption.** Response bodies were read as raw bytes
  and treated as UTF-8 everywhere (every pattern regex, and encoding/json on
  the way out), with no charset detection at all. Real sites in production
  still declare `charset=iso-8859-1` / `windows-1252` (older European sites)
  or Shift_JIS/EUC-KR/GBK (older Japanese/Korean/Chinese sites); reading those
  bytes as UTF-8 silently breaks every non-ASCII title/link-text match,
  including the CJK/Cyrillic/Greek/Arabic script matchers added specifically
  for geo coverage. `decodeToUTF8` (charset.go) now transcodes to UTF-8 when
  the origin explicitly declares (and `golang.org/x/net/html/charset` is
  *certain* of) a non-UTF-8 encoding; an undeclared/ambiguous body is left
  byte-for-byte unchanged, so this cannot introduce new corruption on
  already-UTF-8 pages.
- **German "Datenschutzerklärung" title under-scored.** The privacy-policy
  `titleRegex` and canonical-probe false-positive guard matched bare
  `\bdatenschutz\b`, which cannot match inside the standard German compound
  title "Datenschutzerklärung" (no word boundary mid-compound). A real German
  privacy page titled exactly this way — even with a perfectly UTF-8 body —
  fell through to low/medium confidence, and could be rejected outright by
  the canonical-probe FP guard if the body used only German legal vocabulary.
  Found while writing the charset-fix test fixture; fixed independently of
  encoding in both `patterns.go` and `verify.go`.

## 1.3.5 — 2026-07-17

### Changed
- Make ToS finder selftest deterministic
- fix: reject third-party and soft-404 legal docs
- fix: reduce detector false positives

## 1.3.4 — 2026-07-05

### Fixed
- Retry the pre-fetch SSRF/DNS reachability check (safehttp.CheckURL) once on a transient DNS lookup timeout, mirroring the existing homepage-fetch retry. Live domains (including stripe.com/github.com) were intermittently failing this single-shot 3s DNS check and getting permanently recorded as unreachable.

## 1.3.3 — 2026-06-08

### Changed
- R5: adopt go-common/meshresult (honest status) (#6)

## 1.3.2 — 2026-06-08

### Changed
- TRL2: FP reduction + honest error taxonomy + retry + real /selftest (#5)
- chore(deps): bump github.com/baditaflorin/go-common to v0.63.0
- chore(deps): bump github.com/baditaflorin/go-common to v0.62.0
- refactor: split handler.go via fleet-runner split --auto (#4)
- refactor: split patterns.go via fleet-runner split --auto (#3)
- chore(deps): bump github.com/baditaflorin/go-common@v0.59.0 (fleet-runner rollout) (#2)
- docs: sync CLAUDE.md from services-registry (repo targeting, v0.55.0 selftest cache-bypass, CHANGELOG convention)
- chore(deps): bump github.com/baditaflorin/go-common to v0.54.0
- chore(deps): bump github.com/baditaflorin/go-common to v0.47.2
- Fix version-validation hook to be portable across BSD/GNU sed (#1)
- fix(fetch): restore corrupted handler.go and add WithFallbackOnTimeout
- docs(CLAUDE.md): warn host_port collisions clobber the colliding live service
