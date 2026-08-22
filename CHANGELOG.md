# Changelog

All notable changes to this service are recorded here, newest first.

## 1.5.2 — 2026-08-22

### Fixed

Found by running the shipped 1.5.1 imprint extractor against a real, live
Austrian imprint page again (hotelrose.at/impressum_de.html — the same
fixture the 1.5.1 pass used) and noticing a real, present VAT number was
silently dropped.

- **VAT dropped when the winning JSON-LD candidate lacks it but a same-page
  plain-text candidate found it.** The real page's JSON-LD block
  (`@type: "Hotel"`) has no `vatID` field at all; its visible text literally
  reads "UID-Nr.: ATU43951103" — a well-formed, valid Austrian VAT number
  that the plain-text `imprint_label` candidate (added in 1.5.1) DOES
  capture via proximity attachment. But `mergeImprintCandidates`
  (`imprint.go`) only merges same-page candidates on exact case-insensitive
  name equality, and the JSON-LD candidate's name
  (`"Aktivhotel zur Rose - Franz Holzmann"`) never matched the plain-text
  candidate's name (`"Aktivhotel Zur Rose"`) — different casing, different
  trailing content. Since JSON-LD outranks plain text in
  `bestImprintCandidate`, the JSON-LD candidate won with no VAT, and the
  VAT-bearing candidate was discarded outright. Worse: because
  `scoreImprint` only adds VAT to the completeness denominator when a VAT is
  actually present, the missing VAT didn't lower the score either — the page
  reported a deceptively perfect `completeness_score: 100` despite genuinely
  failing to surface a real, present VAT number.

  Fixed with a new `backfillWinnerIdentifiers` step (`imprint.go`): after
  `bestImprintCandidate` picks the winning candidate, a still-missing
  `Register`/`VAT` is backfilled from any OTHER candidate in the SAME
  single-page candidate list that has it — not gated on name match.
  `extractImprintFields` only ever runs on a single already-fetched,
  already-verified imprint page, so unlike go_legal_entity's original
  multi-page `mergeCandidates` (which needs strict name-matching to avoid
  conflating two different companies found on two different pages), a
  same-page identifier find is already itself the evidence of entity
  association. `mergeImprintCandidates`'s existing exact-name merge behaviour
  for genuinely-duplicate same-named candidates (e.g. JSON-LD + hCard both
  naming the identical company) is unchanged — this is an additional
  backfill step, not a loosening of that function.

New regression tests (`imprint_extract_real_evidence_test.go`): VAT/
`vat_validation`/`vat_valid` assertions added to the existing hotelrose.at
real-evidence test, plus two new tests isolating the backfill mechanism
directly (`TestBackfillWinnerIdentifiersHotelRoseRealEvidencePattern`,
`TestBackfillWinnerIdentifiersDoesNotBackfillFromUnrelatedName`).

## 1.5.1 — 2026-08-22

### Fixed

Real-world gaps found by manually comparing this service's imprint field
extraction (shipped in 1.5.0, same day) against actual live websites'
Impressum content, specifically hunting for false negatives/positives — not
a hypothetical review.

- **JSON-LD Organization-subtype matching missed schema.org `Hotel`**
  (`isOrgType`, `imprint_jsonld.go`). The accepted-type set was an
  exact-match list (`organization`/`corporation`/`localbusiness`/`ngo`/
  `educationalorganization`/`governmentorganization`/`performinggroup`) with
  no awareness of schema.org's real type hierarchy (`Hotel` ->
  `LodgingBusiness` -> `LocalBusiness` -> `Organization`). Found via a real
  Austrian hotel's Impressum page whose JSON-LD (`@type: "Hotel"`, carrying
  `name`/`address`/`telephone`/`email`) this service silently produced zero
  fields from. Broadened to ~60 common schema.org `LocalBusiness` subtypes
  and second-level leaves (`Restaurant`/`Store`/`ProfessionalService`/
  `MedicalBusiness`/`AutomotiveBusiness`/`FinancialService`/`Attorney`/
  `Dentist`/`RealEstateAgent`/... — a curated common-case list, not
  exhaustive).
- **Sole-trader imprints have no legal-form suffix to anchor on at all.**
  The suffix-anchored plain-text name scan (`extractImprintText`) required a
  GmbH/AG/Ltd-style suffix before it would identify a candidate name —
  systematically missing every sole-trader (Einzelunternehmen) imprint,
  which legitimately has none. Found on the same real Austrian hotel page,
  whose visible text reads "Firmenname: Aktivhotel Zur Rose" with no legal
  form anywhere on the page. Added a new fallback extraction path (source
  `imprint_label`, ranked below an actual suffix match) that recognises an
  explicit label (`Firmenname:`/`Company name:`/`Unternehmen:`/`Inhaber:`)
  anchored to the start of a line — so it does not fire on running prose
  that merely mentions the word.
- **The `responsible_person` checklist item couldn't be satisfied by a sole
  trader.** `responsiblePersonLabelRE` requires a distinct label
  (Geschäftsführer/vertretungsberechtigt/verantwortlich für den
  Inhalt/managing director/...), but a sole trader's imprint never has one —
  the proprietor's name IS the trading name. Confirmed against the same real
  page ("Aktivhotel zur Rose - Franz Holzmann", no separate label anywhere);
  Austria's ECG §5 / Mediengesetz §§24-25 don't require a separately
  labelled line when the proprietor's identity is already disclosed this
  way. Added an additional satisfaction path (the label-based match is
  unchanged for GmbH/AG-style entities): when no legal-form suffix was found
  at all, the extracted legal name is checked for an embedded two-plus-word
  Title-Case run that looks like a natural person's name.
- **Cloudflare's email-obfuscation markup defeated contact detection
  entirely.** Cloudflare's "Email Address Obfuscation" anti-scraping feature
  (`data-cfemail="<hex>"` / `/cdn-cgi/l/email-protection#<hex>`, a public,
  well-documented reversible single-byte-XOR encoding) was invisible to
  `hasImprintContact`'s plain-text email regex — a real, extremely common
  pattern across the web, verified live on a gambling-affiliate homepage
  using it to hide a real contact address. Added a decoder (the same
  algorithm Cloudflare's own injected browser JS uses to reconstruct the
  address) so a successfully-decoded email now counts toward the `contact`
  checklist item.
- **Register field now credits a court-only mention.** A named register
  court (`courtMention`) was previously only appended when a register NUMBER
  was already present; the real hotel page names "Firmengericht:
  Landesgericht Innsbruck" but cites no FN number at all. A court-only
  mention now satisfies the `register` checklist item on its own.

New regression tests (`imprint_extract_real_evidence_test.go`) use real
excerpts from the live pages that exposed each gap, plus a false-positive
stress test confirming a JSON-LD `WebPage.author` `Person` node (an SEO
byline seen on a real gambling-affiliate page) was already correctly
ignored.

## 1.5.0 — 2026-08-22

### Added

- **Structured imprint (Impressum) field extraction.** `documents[]` for
  `DocImprint` previously carried only a bare `imprint_present` boolean — a
  `merged[DocImprint]` map-presence check backed by verify.go's
  `docTypeSignalRE[DocImprint]` vocabulary sanity-check. That confirms a page
  plausibly IS an imprint page but says nothing about whether it discloses
  what EU e-Commerce Directive Art. 5 / Germany's TMG §5 (→DDG §5) / Austria's
  ECG §5 + Mediengesetz §§24-25 actually require: legal entity name, address,
  register number, VAT ID, responsible person, contact. The response now also
  carries a structured `imprint` object (`imprint_present` is unchanged, and
  is now derived from `imprint.present`) with: `legal_name` + `suffix`
  (legal-form, e.g. "GmbH"), `address`, `country` (ISO-3166-1 alpha-2),
  `register` (e.g. "HRB 12345, Amtsgericht Berlin" / "FN 123456a,
  Firmenbuchgericht Wien"), `vat` + `vat_validation` (real per-country
  check-digit validation, not just regex-shape matching), `responsible_person`,
  and `fields_found`/`fields_missing`/`completeness_score`/`ruleset` scored
  against one of three field checklists (`eu_baseline` / `de_tmg` /
  `at_ecg_medieng`) selected by the extracted country.
- Extraction runs on the imprint page's already-fetched, already-verified
  body (no extra HTTP round trip) via a priority chain — Schema.org JSON-LD
  `Organization` > a suffix-anchored plain-text line-scan > hCard/vCard
  microformats > footer copyright lines > `og:site_name` — vendored (and
  adapted) from the sibling `go_legal_entity` service's mature (TRL 6)
  field-extraction pipeline rather than re-derived from scratch: its
  27-EU-country+non-EU VAT/register identifier table (`imprint_vat.go`,
  extended here with an Austrian FN pattern the upstream table was missing),
  per-country VAT checksum algorithms (`imprint_checksum.go`), ISO-3166-1
  country normalisation (`imprint_country.go`), a trimmed Latin-script
  legal-entity-suffix table (`imprint_suffix.go`), and the
  false-positive-hardened candidate name/address cleanup
  (`imprint_name.go`, `imprint_jsonld.go`).
- New multi-language responsible-person regex
  (Geschäftsführer/vertretungsberechtigt/verantwortlich für den Inhalt for
  DE/AT, managing director/legal representative for EN, représentant légal
  for FR).

## 1.4.4 — 2026-08-20

DomainScope extraction fixes promoted; distinct tool_version for this production rollout.

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
