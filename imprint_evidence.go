package main

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// ImprintEvidence reports observations, not verified claims about the site's
// operator. URL applies to every observation; extraction is limited to the
// selected, already verified imprint page. Existing scalar fields are unchanged.
type ImprintEvidence struct {
	URL                    string                       `json:"url"`
	FetchedURL             string                       `json:"fetched_url,omitempty"`
	SourceLimitReached     bool                         `json:"source_limit_reached,omitempty"`
	CacheReportedTruncated bool                         `json:"cache_reported_truncated,omitempty"`
	Observations           []ImprintEvidenceObservation `json:"observations"`
	Truncated              bool                         `json:"truncated,omitempty"`
	OmittedSensitive       int                          `json:"omitted_sensitive,omitempty"`
	UnparseableJSONLD      int                          `json:"unparseable_json_ld,omitempty"`
}

// JSON-LD paths are RFC 6901 JSON Pointers within the zero-based JSON-LD
// script index. ObjectPath identifies the source object, not a verified legal
// entity. AddressPath groups only components in the same source address.
// Heuristic observations have no object/path association: extractors can use
// proximity, and even hCard fields can come from different parts of a page.
type ImprintEvidenceObservation struct {
	Field           string  `json:"field"`
	Value           string  `json:"value"`
	NormalizedValue string  `json:"normalized_value,omitempty"`
	Method          string  `json:"method"`
	Basis           string  `json:"basis"`
	Role            string  `json:"role,omitempty"`
	Component       string  `json:"component,omitempty"`
	ScriptIndex     *int    `json:"script_index,omitempty"`
	SourcePath      *string `json:"source_path,omitempty"`
	ObjectPath      *string `json:"object_path,omitempty"`
	AddressPath     *string `json:"address_path,omitempty"`
}

// The document fetch reads at most verifyBodyBytes (256 KiB) on the wire;
// decoding can expand that into a longer UTF-8 string. These additional
// limits bound path amplification and deeply nested input. A caller
// can see any loss due to those limits in Truncated; sensitive-looking values
// are counted separately and never included in this evidence object.
const (
	maxImprintObservations       = 2048
	maxImprintEvidenceBytes      = 768 * 1024
	maxImprintEvidenceValueBytes = 4096
	maxImprintEvidencePathBytes  = 1024
	maxImprintEvidenceDepth      = 32
)

// Do not deliberately copy contact/identifier payloads into the new field
// stream. This is a conservative filter for mixed text, not a claim to detect
// every possible private value mislabeled as a name or address by a website.
var imprintEvidenceSensitiveRE = regexp.MustCompile(`(?i)(?:mailto:|tel:|\b(?:e-?mail|telephone|phone|fax|mobile|tel\.?|VAT|TVA|SIRET|SIREN|RCS|tax\s*(?:id|number)|registration\s*(?:id|number)|UID)\s*[:#.]\s*\S|\b(?:VAT|TVA|SIRET|SIREN|RCS|UID|tax\s*(?:id|number)|registration\s*(?:id|number))\b\s*[:#.-]?\s*(?:[A-Z]{1,3}\s*)?\d{4,}|(?:法人番号|登録番号)\s*[:：]?\s*\d{4,}|\+\d[\d\s()./\-]{6,}\d)`)

type imprintEvidenceBuilder struct {
	evidence ImprintEvidence
	bytes    int
}

var imprintEvidenceNumericPayloadRE = regexp.MustCompile(`(?:\b\d{8,}\b|\b\d{2,4}[ \t/().-]+\d{2,4}[ \t/().-]+\d{2,4}(?:[ \t/().-]+\d{2,4})?\b)`)

func sensitiveImprintEvidenceValue(value string) bool {
	return imprintEmailRE.MatchString(value) || imprintPhoneRE.MatchString(value) || imprintEvidenceSensitiveRE.MatchString(value) || imprintEvidenceNumericPayloadRE.MatchString(value)
}

func (b *imprintEvidenceBuilder) add(o ImprintEvidenceObservation) {
	o.Value = strings.TrimSpace(o.Value)
	if o.Value == "" {
		return
	}
	if sensitiveImprintEvidenceValue(o.Value) {
		b.evidence.OmittedSensitive++
		return
	}
	pathBytes := 0
	for _, p := range []*string{o.SourcePath, o.ObjectPath, o.AddressPath} {
		if p != nil {
			if len(*p) > maxImprintEvidencePathBytes {
				b.evidence.Truncated = true
				return
			}
			pathBytes += len(*p)
		}
	}
	size := len(o.Value) + len(o.NormalizedValue) + len(o.Field) + len(o.Method) + len(o.Basis) + len(o.Role) + len(o.Component) + pathBytes
	if len(o.Value) > maxImprintEvidenceValueBytes || len(b.evidence.Observations) >= maxImprintObservations || b.bytes+size > maxImprintEvidenceBytes {
		b.evidence.Truncated = true
		return
	}
	b.bytes += size
	b.evidence.Observations = append(b.evidence.Observations, o)
}

func evidenceString(s string) *string { return &s }
func evidenceInt(i int) *int          { return &i }

func collectImprintEvidence(body, pageURL string, candidates []imprintCandidate) *ImprintEvidence {
	b := &imprintEvidenceBuilder{evidence: ImprintEvidence{URL: pageURL, Observations: []ImprintEvidenceObservation{}}}
	b.collectJSONLD(body)
	b.collectLabeledHostingProviders(body)
	// Capture the existing heuristic observations before normalization can
	// discard providers or merge different sources. They are independent
	// observations: there is intentionally no candidate/subject ID linking
	// a guessed name, address, or country.
	for _, c := range candidates {
		if c.Source == "json_ld" { // structural values are collected below
			continue
		}
		if name, _, ok := normalizeLegalName(c.Name, c.Suffix); ok {
			b.add(ImprintEvidenceObservation{Field: "name", Value: name, Method: c.Source, Basis: "heuristic"})
		}
		if c.Address != "" {
			b.add(ImprintEvidenceObservation{Field: "address", Value: c.Address, Method: c.Source, Basis: "heuristic", Component: "full"})
		}
		if c.Country != "" {
			b.add(ImprintEvidenceObservation{Field: "country", Value: c.Country, Method: c.Source, Basis: "heuristic"})
		}
	}
	if len(b.evidence.Observations) == 0 && !b.evidence.Truncated && b.evidence.OmittedSensitive == 0 && b.evidence.UnparseableJSONLD == 0 {
		return nil
	}
	return &b.evidence
}

func (b *imprintEvidenceBuilder) collectJSONLD(body string) {
	z := html.NewTokenizer(strings.NewReader(body))
	index := 0
	active := false
	var script strings.Builder
	flush := func() {
		if !active {
			return
		}
		var root interface{}
		if json.Unmarshal([]byte(script.String()), &root) != nil {
			b.evidence.UnparseableJSONLD++
		} else {
			b.visitJSONLD(root, "", index, 0)
		}
		index++
		active = false
		script.Reset()
	}
	for {
		switch z.Next() {
		case html.ErrorToken:
			flush()
			return
		case html.StartTagToken:
			t := z.Token()
			if t.Data != "script" {
				continue
			}
			for _, a := range t.Attr {
				if strings.EqualFold(a.Key, "type") && strings.EqualFold(a.Val, "application/ld+json") {
					active = true
					break
				}
			}
		case html.TextToken:
			if active {
				script.Write(z.Text())
			}
		case html.EndTagToken:
			if z.Token().Data == "script" {
				flush()
			}
		}
	}
}

func (b *imprintEvidenceBuilder) visitJSONLD(v interface{}, path string, script, depth int) {
	if depth > maxImprintEvidenceDepth || len(path) > maxImprintEvidencePathBytes {
		b.evidence.Truncated = true
		return
	}
	switch x := v.(type) {
	case []interface{}:
		for i, child := range x {
			b.visitJSONLD(child, fmt.Sprintf("%s/%d", path, i), script, depth+1)
		}
	case map[string]interface{}:
		if graph, ok := x["@graph"]; ok {
			b.visitJSONLD(graph, path+"/@graph", script, depth+1)
		}
		if !isOrgType(stringifyType(x["@type"])) {
			return
		}
		for _, f := range []struct{ key, field string }{{"legalName", "legal_name"}, {"name", "name"}} {
			if value, ok := x[f.key]; ok {
				b.jsonText(value, path+"/"+f.key, path, "", script, depth+1, f.field, "")
			}
		}
		if value, ok := x["address"]; ok {
			b.jsonAddress(value, path+"/address", path, script, depth+1)
		}
	}
}

func (b *imprintEvidenceBuilder) jsonAddress(v interface{}, path, object string, script, depth int) {
	if depth > maxImprintEvidenceDepth || len(path) > maxImprintEvidencePathBytes {
		b.evidence.Truncated = true
		return
	}
	switch x := v.(type) {
	case string:
		b.jsonText(x, path, object, path, script, depth+1, "address", "full")
	case []interface{}:
		for i, item := range x {
			b.jsonAddress(item, fmt.Sprintf("%s/%d", path, i), object, script, depth+1)
		}
	case map[string]interface{}:
		for _, f := range []struct{ key, field, component string }{
			{"streetAddress", "address", "street_address"}, {"postOfficeBoxNumber", "address", "post_office_box"},
			{"postalCode", "address", "postal_code"}, {"addressLocality", "address", "locality"},
			{"addressRegion", "address", "region"}, {"addressCountry", "country", ""},
		} {
			if value, ok := x[f.key]; ok {
				b.jsonText(value, path+"/"+f.key, object, path, script, depth+1, f.field, f.component)
			}
		}
	}
}

func (b *imprintEvidenceBuilder) jsonText(v interface{}, path, object, address string, script, depth int, field, component string) {
	if depth > maxImprintEvidenceDepth || len(path) > maxImprintEvidencePathBytes {
		b.evidence.Truncated = true
		return
	}
	switch x := v.(type) {
	case string:
		o := ImprintEvidenceObservation{Field: field, Value: x, Method: "json_ld", Basis: "explicit", Component: component, ScriptIndex: evidenceInt(script), SourcePath: evidenceString(path), ObjectPath: evidenceString(object)}
		if address != "" {
			o.AddressPath = evidenceString(address)
		}
		if field == "country" {
			o.NormalizedValue = normalizeCountry(x)
		}
		b.add(o)
	case []interface{}:
		for i, item := range x {
			b.jsonText(item, fmt.Sprintf("%s/%d", path, i), object, address, script, depth+1, field, component)
		}
	case map[string]interface{}:
		// Language-value objects and Schema.org Country objects can both
		// carry independently stated strings. No other keys are copied.
		for _, key := range []string{"@value", "name"} {
			if item, ok := x[key]; ok {
				b.jsonText(item, path+"/"+key, object, address, script, depth+1, field, component)
			}
		}
	}
}

// A historical first-party French legal-notice fixture places a separately
// identified host under an explicit Hébergeur heading. The existing suffix
// scanner misses SASU, and its normalizer intentionally removes providers.
// Retain this narrowly shaped section as evidence only; it cannot win the
// site's scalar legal name. Roles describe the source section, not proof of an
// operator relationship or a verified grouping of every field in that section.
var imprintProviderHeadingRE = regexp.MustCompile(`(?i)^h[ée]bergeur(?:\s+du\s+site)?\s*:?$`)
var imprintProviderSASUNameRE = regexp.MustCompile(`(?i)(?:^|\s)S\.?A\.?S\.?U\.?$`)
var imprintProviderAddressRE = regexp.MustCompile(`(?i)^adresse\s+du\s+si[èe]ge\s*:\s*(.+)$`)
var imprintProviderClientRE = regexp.MustCompile(`(?i)^\s*(?:client|customer|utilisateur|abonn[ée])\b`)

// Process each element's style once in the DOM walk. Oversized styles are
// skipped instead of repeatedly scanning attacker-controlled ancestor text.
func imprintHiddenElement(n *html.Node) (hidden, truncated bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, "hidden") || (strings.EqualFold(a.Key, "aria-hidden") && strings.EqualFold(strings.TrimSpace(a.Val), "true")) {
			return true, false
		}
		if strings.EqualFold(a.Key, "style") {
			if len(a.Val) > 4096 {
				return true, true
			}
			style := strings.ToLower(strings.Join(strings.Fields(a.Val), ""))
			if strings.Contains(style, "display:none") || strings.Contains(style, "visibility:hidden") {
				return true, false
			}
		}
	}
	return false, false
}

func imprintSectionLines(root *html.Node) ([]string, bool) {
	type item struct {
		node  *html.Node
		depth int
	}
	var stack []item
	for c := root.LastChild; c != nil; c = c.PrevSibling {
		stack = append(stack, item{c, 1})
	}
	var lines []string
	var line strings.Builder
	visited := 0
	flush := func() {
		if s := strings.TrimSpace(line.String()); s != "" {
			lines = append(lines, s)
		}
		line.Reset()
	}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		visited++
		if visited > 256 || x.depth > 16 || line.Len() > 4096 {
			return nil, true
		}
		switch x.node.Type {
		case html.TextNode:
			if len(x.node.Data) > 4096-line.Len() {
				return nil, true
			}
			line.WriteString(x.node.Data)
		case html.ElementNode:
			hidden, truncated := imprintHiddenElement(x.node)
			if truncated {
				return nil, true
			}
			if hidden {
				continue
			}
			switch x.node.Data {
			case "br":
				flush()
				continue
			case "span", "strong", "b", "em", "i": // inline text only
			default:
				return nil, false
			}
			for c := x.node.LastChild; c != nil; c = c.PrevSibling {
				stack = append(stack, item{c, x.depth + 1})
			}
		}
	}
	flush()
	return lines, false
}

func (b *imprintEvidenceBuilder) collectLabeledHostingProviders(body string) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return
	}
	type item struct {
		node  *html.Node
		depth int
	}
	stack := []item{{doc, 0}}
	for len(stack) > 0 {
		x := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := x.node
		if x.depth > maxImprintEvidenceDepth {
			b.evidence.Truncated = true
			continue
		}
		if n.Type == html.ElementNode {
			hidden, truncated := imprintHiddenElement(n)
			if truncated {
				b.evidence.Truncated = true
			}
			if hidden {
				continue
			}
		}
		for c := n.LastChild; c != nil; c = c.PrevSibling {
			stack = append(stack, item{c, x.depth + 1})
		}
		if n.Type == html.ElementNode && n.Data == "p" {
			b.collectExplicitAddressLines(n)
		}
		if n.Type != html.ElementNode || (n.Data != "h2" && n.Data != "h3" && n.Data != "h4") {
			continue
		}
		heading, tooLarge := imprintSectionLines(n)
		if tooLarge {
			b.evidence.Truncated = true
		}
		if len(heading) != 1 || !imprintProviderHeadingRE.MatchString(heading[0]) {
			continue
		}
		paragraph := n.NextSibling
		for paragraph != nil && (paragraph.Type == html.CommentNode || (paragraph.Type == html.TextNode && strings.TrimSpace(paragraph.Data) == "")) {
			paragraph = paragraph.NextSibling
		}
		if paragraph == nil || paragraph.Type != html.ElementNode || paragraph.Data != "p" {
			continue
		}
		hidden, truncated := imprintHiddenElement(paragraph)
		if truncated {
			b.evidence.Truncated = true
		}
		if hidden {
			continue
		}
		lines, tooLarge := imprintSectionLines(paragraph)
		if tooLarge {
			b.evidence.Truncated = true
		}
		if len(lines) == 0 {
			continue
		}
		name := strings.TrimSpace(lines[0])
		if strings.ContainsAny(name, ":\n\r") || !validLabeledName(name) {
			continue
		}
		if _, _, _, ok := detectSuffix(name); !ok && !imprintProviderSASUNameRE.MatchString(name) {
			continue
		}
		b.add(ImprintEvidenceObservation{Field: "name", Value: name, Method: "imprint_provider_section", Basis: "heuristic", Role: "hosting_provider"})
		for _, line := range lines[1:] {
			if imprintProviderClientRE.MatchString(line) {
				break
			}
			if m := imprintProviderAddressRE.FindStringSubmatch(strings.TrimSpace(line)); len(m) == 2 {
				b.add(ImprintEvidenceObservation{Field: "address", Value: m[1], Method: "imprint_provider_section", Basis: "heuristic", Role: "hosting_provider", Component: "full"})
			}
		}
	}
}

// A current first-party Austrian imprint explicitly labels a visible postal
// address, but the name-based candidate scanner misses it on the full page.
// Capture individual lines after a direct bold Adresse label without attaching
// them to the selected name, JSON-LD object, or an inferred country. Only
// direct text and br siblings are followed; contact-like, non-postal and
// unsupported markup end the observation window.
func (b *imprintEvidenceBuilder) collectExplicitAddressLines(paragraph *html.Node) {
	for label := paragraph.FirstChild; label != nil; label = label.NextSibling {
		if label.Type != html.ElementNode || (label.Data != "strong" && label.Data != "b") {
			continue
		}
		hidden, truncated := imprintHiddenElement(label)
		if truncated {
			b.evidence.Truncated = true
		}
		if hidden {
			continue
		}
		text, truncated := imprintSectionLines(label)
		if truncated {
			b.evidence.Truncated = true
		}
		if len(text) != 1 || !strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(text[0]), ":"), "Adresse") {
			continue
		}
		visited := 0
	addressLines:
		for sibling := label.NextSibling; sibling != nil; sibling = sibling.NextSibling {
			visited++
			if visited > 32 {
				b.evidence.Truncated = true
				break
			}
			switch sibling.Type {
			case html.CommentNode:
				continue
			case html.ElementNode:
				if sibling.Data == "br" {
					continue
				}
				break addressLines
			case html.TextNode:
				value := strings.TrimSpace(sibling.Data)
				if value == "" {
					continue
				}
				if len(value) > 512 {
					b.evidence.Truncated = true
					break addressLines
				}
				if sensitiveImprintEvidenceValue(value) {
					b.evidence.OmittedSensitive++
					break addressLines
				}
				if !looksAddressLine(value) {
					break addressLines
				}
				b.add(ImprintEvidenceObservation{Field: "address", Value: value, Method: "imprint_address_label", Basis: "heuristic", Component: "address_line"})
			default:
				break addressLines
			}
		}
	}
}
