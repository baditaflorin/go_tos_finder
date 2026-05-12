package main

import (
	"net/url"
	"regexp"
	"strings"
)

// linkScan scans rendered HTML and returns the best DocFinding per type from
// in-page <a href> links. Footer links are preferred when both header and
// footer link variants match the same doc type.
func linkScan(html string, base *url.URL) map[DocType]DocFinding {
	out := make(map[DocType]DocFinding)
	anchors := extractAnchors(html)
	footerStart, footerEnd := footerBounds(html)

	for _, a := range anchors {
		abs := absolutize(a.Href, base)
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
		linkText := strings.TrimSpace(stripTags(a.Inner))
		if len(linkText) > 80 {
			linkText = linkText[:80]
		}
		dt, how := classifyLink(u.Path, linkText)
		if dt == "" {
			continue
		}
		source := "page_link"
		if a.Offset >= footerStart && a.Offset <= footerEnd {
			source = "footer_link"
		}
		hit := DocFinding{
			Type:     dt,
			URL:      stripFragment(u),
			Source:   source,
			LinkText: linkText,
			Match:    how,
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

var anchorRE = regexp.MustCompile(`(?is)<a\b[^>]*?\bhref\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))[^>]*>(.*?)</a>`)

func extractAnchors(html string) []anchor {
	matches := anchorRE.FindAllStringSubmatchIndex(html, -1)
	out := make([]anchor, 0, len(matches))
	for _, m := range matches {
		// m: [start, end, g1s, g1e, g2s, g2e, g3s, g3e, g4s, g4e]
		var href, inner string
		switch {
		case m[2] != -1:
			href = html[m[2]:m[3]]
		case m[4] != -1:
			href = html[m[4]:m[5]]
		case m[6] != -1:
			href = html[m[6]:m[7]]
		}
		if m[8] != -1 {
			inner = html[m[8]:m[9]]
		}
		out = append(out, anchor{
			Href:   strings.TrimSpace(href),
			Inner:  inner,
			Offset: m[0],
		})
	}
	return out
}

// footerBounds locates the byte range of the last <footer>...</footer> in the
// HTML, falling back to the bottom 30% of the document if no <footer> exists.
func footerBounds(html string) (int, int) {
	low := strings.ToLower(html)
	if idx := strings.LastIndex(low, "<footer"); idx >= 0 {
		end := strings.Index(low[idx:], "</footer>")
		if end < 0 {
			return idx, len(html)
		}
		return idx, idx + end + len("</footer>")
	}
	// Fallback: anything beyond 70% of the page treated as "footer-ish".
	return (len(html) * 7) / 10, len(html)
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

// stripTags returns text content with HTML tags removed. Quick and lossy.
var tagRE = regexp.MustCompile(`<[^>]+>`)
var wsRE = regexp.MustCompile(`\s+`)

func stripTags(html string) string {
	s := tagRE.ReplaceAllString(html, " ")
	s = wsRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
