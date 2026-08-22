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
