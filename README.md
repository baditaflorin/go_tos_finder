# go_tos_finder

Locates legal documents (Terms of Service, Privacy Policy, Cookie Policy,
DMCA, Acceptable Use, GDPR/DPA, Imprint, Sub-processors, SLA, Refund/Shipping,
etc.) on a target site and reports URLs plus freshness metadata.

This is a discovery tool — it does **not** assess compliance. For that, see
[`go_gdpr_compliance`](https://github.com/baditaflorin/go_gdpr_compliance).

## API

```
GET /?target=https://example.com
```

Authenticate via `Authorization: Bearer <key>`, `X-API-Key: <key>`, or
`?api_key=<key>` (browser/demo convenience — leaks the key into logs, avoid
for anything but manual testing). The legacy `/t/{token}/go_tos_finder?target=<url>`
path form is deprecated fleet-wide and no longer routable.

Response (truncated for brevity):

```json
{
  "tool": "go_tos_finder",
  "version": "1.0.0",
  "target": "https://github.com",
  "documents": [
    {
      "type": "terms_of_service",
      "url": "https://github.com/site/terms",
      "status": 200,
      "last_modified": "2024-03-15",
      "staleness": "current",
      "source": "footer_link",
      "link_text": "Terms"
    },
    {
      "type": "privacy_policy",
      "url": "https://github.com/site/privacy",
      "status": 200,
      "source": "footer_link"
    }
  ],
  "documents_found": 2,
  "documents_missing": ["cookie_policy", "acceptable_use", "dmca"],
  "imprint_present": false,
  "verdict": "partial"
}
```

## Imprint observations

When the selected imprint page has a body, `imprint.evidence` may contain its
URL and independent observations of names, address components and countries.
`evidence.url` is the selected link; `evidence.fetched_url`, when available,
is the final response URL of the body after redirects. A missing `fetched_url`
does not establish whether a redirect occurred.
The existing `imprint.legal_name`, `address`, `country`, identifiers and scoring
remain the selected scalar result; observations are not verified operator
claims and must not be merged into an entity merely because they share a page.

For JSON-LD, `script_index` counts `application/ld+json` scripts from zero and
`source_path` is an RFC 6901 JSON Pointer in that script. `object_path` marks
only the source object, and `address_path` groups only components from the same
address object or string. A `name` is not automatically a legal name. Country
values remain as stated; `normalized_value` is present only when recognized.
Text, Open Graph, copyright and hCard observations use their extractor's
`method` and `basis: "heuristic"`; they have no object or address association,
including when two fields happen to be near each other. They may name a host,
provider or unrelated party. An `imprint_provider_section` observation is
under an explicit hosting-provider heading; `role: "hosting_provider"` describes
that source section, not a verified legal owner or a cross-field entity match.
Direct postal-looking text lines after a bold `Adresse` label use
`method: "imprint_address_label"` and `component: "address_line"`; those lines
are separate observations and are not assigned to a subject. A country inferred
by a heuristic is not an explicit country statement. Existing heuristic extraction itself has limits,
including its existing candidate-count limits; this is not a full page archive.

The stream deliberately copies no identifier or contact properties. It also
omits values that look like mixed contact or identifier text and reports their
count in `omitted_sensitive`; the filter cannot recognize every maliciously
mislabelled value. The fetcher retains at most 256 KiB before character-set decoding, which can
expand the resulting UTF-8 body. `source_limit_reached: true` means the byte
limit was filled, so the source may be incomplete (including at an exact
boundary). The observations additionally have count, size and
depth limits, and `truncated: true` signals an output limit was reached; malformed JSON-LD scripts
are counted in `unparseable_json_ld`. Only the selected imprint page is
processed, and a missing body cannot yield evidence.

## Verdict

- `comprehensive` — ToS + Privacy + ≥1 other, or 5+ documents.
- `partial` — ≥2 documents found but ToS/Privacy missing.
- `sparse` — exactly 1 document found.
- `none` — no documents discovered.
- `unknown` — the homepage could not be verified (see `status`/`result` below);
  this is NOT a claim that the site has no legal documents.

## Status / result

`status` (and the fleet-canonical `result`) additionally distinguish *why* a
scan didn't produce documents:

- `ok` — homepage fetched and scanned; `documents`/`verdict` reflect real findings.
- `no_data` — homepage fetched and scanned; genuinely no legal document found.
- `unreachable` — the target could not be reached at all (NXDOMAIN, connect/TLS
  failure, ...).
- `blocked` — the homepage responded (HTTP < 500) but the body is a bot-block /
  WAF-challenge interstitial (Cloudflare JS challenge, a stock 403 stub, a
  generic "Access Denied" page) or an unusably empty stub, not real site
  content. Reported as HTTP 502 with `verdict:"unknown"` rather than a false
  `no_data` — we did not actually see the site, so "no legal documents found"
  would be a false claim of certainty.

## Flow

1. SSRF-guard the target URL (block private, loopback, link-local, CGNAT, ULA).
2. Fetch target HTML (capped to 4 MiB).
3. Scan all `<a href>` links for URL-path or link-text matches against a
   per-document-type pattern table (`patterns.go`). Footer links are preferred
   over header/page links.
4. For each document type still missing, probe a small set of canonical paths
   (HEAD; falls back to GET on 405/501). Capped at 30 probes total, with
   `errgroup.SetLimit(5)`.
5. For each discovered document, capture status, `Content-Length`, and parse
   `Last-Modified` into a `current`/`outdated` staleness flag
   (threshold: 2 years).

## Build

```
docker build -t ghcr.io/baditaflorin/go_tos_finder:1.0.0 .
```

## Run

```
docker compose up -d
```

The public demo key (`default_token`) shown against the hosted
`https://tos-finder.0crawl.com` endpoint has been sunset fleet-wide (it was
a security risk — a static, undifferentiated bypass in front of the
keystore) and no longer authenticates there. Against a locally-run
container (as above, no fleet gateway in front of it), this binary's own
built-in local-dev fallback key may still apply — verify against the
running container rather than assuming either way. For hosted access,
use a real fleet-issued key:

```
curl 'http://localhost:8316/?api_key=<your-key>&target=https://github.com'
```

Privacy phrase matching uses Unicode word boundaries and NFC normalization. An
explicit document title or H1 outranks a conflicting section heading; combined
titles and otherwise untyped pages with legal section headings remain supported.
