package main

import (
	htmlstd "html"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// linkScan scans rendered HTML and returns the best DocFinding per type from
// in-page <a href> links. Footer links are preferred when both header and
// footer link variants match the same doc type.
func linkScan(html string, base *url.URL) map[DocType]DocFinding {
	out := make(map[DocType]DocFinding)
	anchors := extractAnchors(html)
	footerStart, footerEnd := footerBounds(html)
	var scriptTargets map[string]*url.URL

	for _, a := range anchors {
		linkText := strings.TrimSpace(stripTags(a.Inner))
		if len(linkText) > 80 {
			linkText = linkText[:80]
		}
		rawHref := htmlstd.UnescapeString(strings.TrimSpace(a.Href))
		abs := ""
		scripted := false
		if strings.HasPrefix(strings.ToLower(rawHref), "javascript:") {
			if scriptTargets == nil {
				scriptTargets = staticScriptLegalURLs(html)
			}
			abs, scripted = staticScriptLegalTarget(scriptTargets, rawHref, linkText)
			if !scripted {
				continue
			}
		} else {
			abs = absolutize(rawHref, base)
		}
		if abs == "" {
			continue
		}
		u, err := url.Parse(abs)
		if err != nil {
			continue
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			continue
		}
		// Platform-wide or social-network legal links in a footer are not this
		// target's documents. Keep the origin and its subdomains only.
		if !sameSiteHost(u, base) && !scripted {
			continue
		}
		// A link to the site root is never a legal document (budongsam.com's
		// "Copyright" link pointed at "/", and the homepage then verified as a
		// "copyright policy" on its generic legal vocabulary).
		if (u.Path == "" || u.Path == "/") && u.RawQuery == "" {
			continue
		}
		dt, how, floor := classifyLinkEvidence(u.Path, linkText)
		if dt == "" {
			continue
		}
		source := "page_link"
		if a.Offset >= footerStart && a.Offset <= footerEnd {
			source = "footer_link"
		}
		hit := DocFinding{
			Type:           dt,
			URL:            stripFragment(u),
			Source:         source,
			LinkText:       linkText,
			Match:          how,
			LinkConfidence: floor,
		}
		if cur, exists := out[dt]; exists {
			// Prefer footer over page; otherwise keep the first match.
			if cur.Source != "footer_link" && source == "footer_link" {
				out[dt] = hit
			}
			continue
		}
		out[dt] = hit
	}
	return out
}

var (
	jsLegalCallRE          = regexp.MustCompile(`(?is)^\s*javascript:\s*([a-z_$][a-z0-9_$]*)\s*\(\s*\)\s*;?\s*$`)
	jsFunctionDefinitionRE = regexp.MustCompile(`(?s)\bfunction\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(\s*\)\s*\{([^{}]{0,1000})\}`)
	jsURLLiteralRE         = regexp.MustCompile(`(?is)["'](https?://[^"'\s]+)["']`)
)

// staticScriptLegalURLs indexes zero-argument page functions that contain one
// unambiguous, absolute HTTP(S) URL literal.
func staticScriptLegalURLs(pageHTML string) map[string]*url.URL {
	urls := make(map[string]*url.URL)
	for _, function := range jsFunctionDefinitionRE.FindAllStringSubmatch(pageHTML, -1) {
		if len(function) != 3 {
			continue
		}
		literals := jsURLLiteralRE.FindAllStringSubmatch(function[2], -1)
		if len(literals) != 1 || len(literals[0]) != 2 {
			continue
		}
		u, err := url.Parse(strings.TrimSpace(literals[0][1]))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			continue
		}
		urls[function[1]] = u
	}
	return urls
}

// staticScriptLegalTarget resolves the narrow, common case where a first-party
// legal link calls one of those functions. It deliberately does not execute
// JavaScript. The destination path and first-party link text must independently
// identify the same specific document type before an off-site URL is admitted;
// normal safehttp checks still protect the subsequent fetch.
func staticScriptLegalTarget(functionURLs map[string]*url.URL, href, linkText string) (string, bool) {
	call := jsLegalCallRE.FindStringSubmatch(href)
	if len(call) != 2 {
		return "", false
	}
	u := functionURLs[call[1]]
	if u == nil {
		return "", false
	}
	pathType, _, _ := classifyLinkEvidence(u.Path, "")
	textType, _, _ := classifyLinkEvidence("", linkText)
	if pathType == "" || pathType != textType || hubDocTypes[pathType] {
		return "", false
	}
	copyURL := *u
	return stripFragment(&copyURL), true
}

// sameSiteHost reports whether u belongs to the same site as base: the same
// registrable domain (eTLD+1, public-suffix aware), so example.com <-> www.
// example.com <-> legal.example.com match, while platform-wide or social-network
// legal links in a footer (policies.google.com, facebook.com/policy) and sibling
// tenants of a multi-tenant suffix (foo.myshopify.com vs bar.myshopify.com, both
// in the public-suffix list) do not. The previous check only accepted base's own
// host and its subdomains, so a site that fetched as https://www.x.com lost every
// footer link that pointed at the apex https://x.com/privacy.
func sameSiteHost(u, base *url.URL) bool {
	if u == nil || base == nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	baseHost := strings.ToLower(strings.TrimSuffix(base.Hostname(), "."))
	if host == "" || baseHost == "" {
		return false
	}
	if host == baseHost || strings.HasSuffix(host, "."+baseHost) {
		return true
	}
	a, errA := publicsuffix.EffectiveTLDPlusOne(host)
	b, errB := publicsuffix.EffectiveTLDPlusOne(baseHost)
	return errA == nil && errB == nil && a == b
}

func stripFragment(u *url.URL) string {
	u.Fragment = ""
	return u.String()
}

// extractAnchors yields <a href=...>...</a> blocks with their byte offset in
// the source HTML. Uses a regex; tolerant of malformed markup. The Inner
// capture is shallow — it'll include nested tags, which stripTags removes
// downstream.
type anchor struct {
	Href   string
	Inner  string
	Offset int
}

// anchorRE is the reference definition of what an anchor is. Production uses the
// linear-time scanAnchors below (the regexp ran at ~1 MB/s on a 4 MB homepage);
// the two are kept equivalent by TestScanAnchorsMatchesRegexp.
var anchorRE = regexp.MustCompile(`(?is)<a\b[^>]*?\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))[^>]*>(.*?)</a>`)

func extractAnchors(html string) []anchor { return scanAnchors(html) }

func isWordByte(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isSpaceByte(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// scanAnchors is the single-pass equivalent of anchorRE.FindAllStringSubmatchIndex.
func scanAnchors(src string) []anchor {
	var out []anchor
	i := 0
	for {
		p := indexFold(src, "<a", i)
		if p < 0 {
			return out
		}
		q := p + 2
		if q >= len(src) {
			return out
		}
		if isWordByte(src[q]) { // \b after "a"
			i = q
			continue
		}
		gt := strings.IndexByte(src[q:], '>')
		if gt < 0 {
			return out
		}
		regionEnd := q + gt
		// Find "\bhref\s*=\s*" before the first '>'.
		href, valEnd, ok := "", 0, false
		for h := q; ; {
			h = indexFold(src[:regionEnd], "href", h)
			if h < 0 {
				break
			}
			if h > 0 && isWordByte(src[h-1]) { // \b before href
				h += 4
				continue
			}
			k := h + 4
			for k < len(src) && isSpaceByte(src[k]) {
				k++
			}
			if k >= len(src) || src[k] != '=' {
				h += 4
				continue
			}
			k++
			for k < len(src) && isSpaceByte(src[k]) {
				k++
			}
			if k >= len(src) {
				break
			}
			switch c := src[k]; {
			case c == '"' || c == '\'':
				e := strings.IndexByte(src[k+1:], c)
				if e < 0 {
					// unterminated quote: the regexp alternative fails; try the
					// unquoted branch exactly as the regexp would.
					e2 := k
					for e2 < len(src) && !isSpaceByte(src[e2]) && src[e2] != '>' {
						e2++
					}
					if e2 == k {
						h += 4
						continue
					}
					href, valEnd, ok = src[k:e2], e2, true
				} else {
					href, valEnd, ok = src[k+1:k+1+e], k+1+e+1, true
				}
			default:
				e2 := k
				for e2 < len(src) && !isSpaceByte(src[e2]) && src[e2] != '>' {
					e2++
				}
				if e2 == k {
					h += 4
					continue
				}
				href, valEnd, ok = src[k:e2], e2, true
			}
			break
		}
		if !ok {
			i = q
			continue
		}
		g := strings.IndexByte(src[valEnd:], '>')
		if g < 0 {
			return out
		}
		innerStart := valEnd + g + 1
		closeAt := indexFold(src, "</a>", innerStart)
		if closeAt < 0 {
			return out
		}
		out = append(out, anchor{Href: cleanHref(href), Inner: src[innerStart:closeAt], Offset: p})
		i = closeAt + 4
	}
}

// cleanHref normalises an href value: surrounding/escaped quotes left by HTML
// that sits inside a JS/JSON string (`href=\"https://x/privacy\"`) are removed
// — they previously produced fetch URLs ending in %22 — and entities decoded.
func cleanHref(h string) string {
	h = strings.TrimSpace(h)
	if strings.IndexByte(h, '\\') >= 0 {
		h = strings.ReplaceAll(h, `\/`, `/`)
		h = strings.TrimLeft(h, `\"'`)
		if i := strings.IndexAny(h, `\"'`); i >= 0 {
			h = h[:i]
		}
	}
	h = strings.Trim(h, `"'`)
	if strings.IndexByte(h, '&') >= 0 {
		h = htmlstd.UnescapeString(h)
	}
	return strings.TrimSpace(h)
}

// footerBounds locates the byte range of the last <footer>...</footer> in the
// HTML, falling back to the bottom 30% of the document if no <footer> exists.
func footerBounds(src string) (int, int) {
	idx := -1
	for _, tag := range []string{"<footer", "<FOOTER", "<Footer"} {
		if k := strings.LastIndex(src, tag); k > idx {
			idx = k
		}
	}
	if idx >= 0 {
		end := indexFold(src, "</footer>", idx)
		if end < 0 {
			return idx, len(src)
		}
		return idx, end + len("</footer>")
	}
	// Fallback: anything beyond 70% of the page treated as "footer-ish".
	return (len(src) * 7) / 10, len(src)
}

// absolutize resolves a possibly-relative URL against `base`.
func absolutize(href string, base *url.URL) string {
	href = strings.TrimSpace(href)
	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") ||
		strings.HasPrefix(href, "mailto:") || strings.HasPrefix(href, "tel:") {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}
