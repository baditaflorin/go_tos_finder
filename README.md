# go_tos_finder

Locates legal documents (Terms of Service, Privacy Policy, Cookie Policy,
DMCA, Acceptable Use, GDPR/DPA, Imprint, Sub-processors, SLA, Refund/Shipping,
etc.) on a target site and reports URLs plus freshness metadata.

This is a discovery tool — it does **not** assess compliance. For that, see
[`go_gdpr_compliance`](https://github.com/baditaflorin/go_gdpr_compliance).

## API

```
GET /t/{token}/go_tos_finder?target=https://example.com
```

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
curl 'http://localhost:8316/t/default_token/?target=https://github.com'
```
