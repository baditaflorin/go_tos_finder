package main

import (
	"encoding/json"
	"strings"

	"golang.org/x/net/html"
)

// Vendored (adapted) from go_legal_entity's extract.go / extract_extract.go /
// extract_country.go / handler_merge.go (v1.10.14) — the structured-data
// candidate chain (Schema.org JSON-LD Organization, hCard/vCard
// microformats, og:site_name, footer copyright lines) plus the
// suffix-anchored plain-text line-scan, in source-priority order:
// json_ld > imprint_text > imprint_label > hcard > copyright_footer >
// og_site_name (see candidateSourceRank in imprint.go). imprint_label is a
// go_tos_finder-native addition, not ported from upstream — see
// extractImprintText's doc comment for why. go_tos_finder extracts from a single
// already-fetched, already-verified imprint page body, so the upstream
// multi-page orchestration (analyzeWith, mergeCandidates across many pages)
// is not ported — only the per-page extractors.

// imprintCandidate describes one extracted entity-identity candidate before
// selection. Trimmed from go_legal_entity's candidateFinding: FoundOn
// (multi-page provenance) isn't meaningful for a single-page extraction, and
// Script (writing system) isn't tracked since the trimmed suffixTable
// (imprint_suffix.go) is Latin-only.
type imprintCandidate struct {
	Name        string
	Suffix      string
	Country     string // ISO-3166-1 alpha-2, best-effort
	Confidence  string // country-inference confidence: high|medium|low
	Source      string // json_ld | hcard | og_site_name | copyright_footer | imprint_text | imprint_label
	Register    string
	VAT         string
	Address     string
	Identifiers []identifierHit
}

// extractImprintCandidates runs the full extraction chain against an
// imprint page's body.
func extractImprintCandidates(body, pageURL string) []imprintCandidate {
	var out []imprintCandidate
	if doc, err := html.Parse(strings.NewReader(body)); err == nil {
		out = append(out, extractJSONLD(doc, pageURL)...)
		out = append(out, extractHCard(doc, pageURL)...)
		out = append(out, extractOG(doc, pageURL)...)
	}
	out = append(out, extractCopyrightFooter(body, pageURL)...)
	out = append(out, extractImprintText(body, pageURL)...)
	return out
}

// --- JSON-LD -----------------------------------------------------------

func extractJSONLD(doc *html.Node, pageURL string) []imprintCandidate {
	var out []imprintCandidate
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" {
			isLD := false
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "type") && strings.EqualFold(a.Val, "application/ld+json") {
					isLD = true
					break
				}
			}
			if isLD && n.FirstChild != nil {
				txt := collectText(n)
				out = append(out, parseLDBlob(txt, pageURL)...)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func parseLDBlob(blob, pageURL string) []imprintCandidate {
	blob = strings.TrimSpace(blob)
	if blob == "" {
		return nil
	}
	// JSON-LD may be a single object, an array, or have @graph.
	var raw interface{}
	if err := json.Unmarshal([]byte(blob), &raw); err != nil {
		return nil
	}
	var out []imprintCandidate
	var visit func(v interface{})
	visit = func(v interface{}) {
		switch t := v.(type) {
		case []interface{}:
			for _, e := range t {
				visit(e)
			}
		case map[string]interface{}:
			if graph, ok := t["@graph"]; ok {
				visit(graph)
			}
			typ := stringifyType(t["@type"])
			if isOrgType(typ) {
				cand := orgFromMap(t, pageURL)
				if cand.Name != "" {
					out = append(out, cand)
				}
			}
		}
	}
	visit(raw)
	return out
}

func stringifyType(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []interface{}:
		var parts []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	}
	return ""
}

// localBusinessSubtypes are Schema.org LocalBusiness subtypes — plus a
// curated set of common second-level leaf types nested one level deeper
// (e.g. Hotel under LodgingBusiness, Attorney under LegalService,
// Restaurant under FoodEstablishment) — that a real controller entity
// commonly declares in JSON-LD instead of the generic
// "Organization"/"LocalBusiness" type. Schema.org's actual type hierarchy
// is e.g. Hotel -> LodgingBusiness -> LocalBusiness -> Organization, but
// isOrgType does flat string equality, not hierarchy-aware matching, so
// every concrete subtype actually seen in the wild has to be listed here
// explicitly.
//
// Found via live verification against a real Austrian hotel's Impressum
// page (schema.org @type: "Hotel") — not a hypothetical gap: this service
// silently produced zero JSON-LD fields for that real, live page before
// this fix (see imprint_extract_real_evidence_test.go). Deliberately not
// exhaustive (schema.org's LocalBusiness tree keeps growing, and native
// non-Latin business types are out of scope the same way imprint_suffix.go
// is Latin-only) — this is a pragmatic, broad-but-curated list of the
// common cases, not a full hierarchy engine.
var localBusinessSubtypes = map[string]bool{
	// Direct schema.org LocalBusiness subtypes (~30, per schema.org's own
	// LocalBusiness page).
	"animalshelter": true, "automotivebusiness": true, "childcare": true,
	"dentist": true, "drycleaningorlaundry": true, "emergencyservice": true,
	"employmentagency": true, "entertainmentbusiness": true,
	"financialservice": true, "foodestablishment": true,
	"governmentoffice": true, "healthandbeautybusiness": true,
	"homeandconstructionbusiness": true, "internetcafe": true,
	"legalservice": true, "library": true, "lodgingbusiness": true,
	"medicalbusiness": true, "professionalservice": true,
	"radiostation": true, "realestateagent": true, "recyclingcenter": true,
	"selfstorage": true, "shoppingcenter": true,
	"sportsactivitylocation": true, "store": true, "televisionstation": true,
	"touristinformationcenter": true, "travelagency": true,

	// Common second-level leaves seen in real-world JSON-LD — sites
	// overwhelmingly declare the specific leaf type directly rather than
	// the intermediate subtype above it.
	"hotel": true, "motel": true, "resort": true, "bedandbreakfast": true,
	"hostel": true, "campground": true, // LodgingBusiness
	"restaurant": true, "bakery": true, "barorpub": true,
	"cafeorcoffeeshop": true, "fastfoodrestaurant": true,
	"icecreamshop": true, "winery": true, // FoodEstablishment
	"attorney": true, "notary": true, // LegalService
	"autodealer": true, "autorepair": true, "autorental": true,
	"autobodyshop": true, "autopartsstore": true, "autowash": true,
	"gasstation": true, "motorcycledealer": true,
	"motorcyclerepair":  true, // AutomotiveBusiness
	"accountingservice": true, "automatedteller": true, "bank": true,
	"insuranceagency": true, // FinancialService
	"physician":       true, "hospital": true, "medicalclinic": true,
	"optician": true, "dermatology": true, // MedicalBusiness-adjacent
	"bookstore": true, "clothingstore": true, "computerstore": true,
	"conveniencestore": true, "departmentstore": true,
	"electronicsstore": true, "florist": true, "furniturestore": true,
	"gardenstore": true, "grocerystore": true, "hardwarestore": true,
	"homegoodsstore": true, "jewelrystore": true, "liquorstore": true,
	"mobilephonestore": true, "movierentalstore": true, "musicstore": true,
	"outletstore": true, "pawnshop": true, "petstore": true,
	"pharmacy": true, "shoestore": true, "sportinggoodsstore": true,
	"tirestore": true, "toystore": true, "bikestore": true, // Store
}

func isOrgType(s string) bool {
	for _, raw := range strings.Split(s, ",") {
		t := strings.ToLower(strings.TrimSpace(raw))
		if i := strings.LastIndex(t, "/"); i >= 0 {
			t = t[i+1:]
		}
		if i := strings.LastIndex(t, "#"); i >= 0 {
			t = t[i+1:]
		}
		switch t {
		case "organization", "corporation", "localbusiness", "ngo", "educationalorganization", "governmentorganization", "performinggroup":
			return true
		}
		if localBusinessSubtypes[t] {
			return true
		}
	}
	return false
}

func orgFromMap(m map[string]interface{}, pageURL string) imprintCandidate {
	c := imprintCandidate{Source: "json_ld"}
	legalName := false
	if s, _ := m["legalName"].(string); s != "" {
		c.Name = strings.TrimSpace(s)
		legalName = c.Name != ""
	}
	if c.Name == "" {
		if s, _ := m["name"].(string); s != "" {
			c.Name = strings.TrimSpace(s)
		}
	}
	if s, _ := m["vatID"].(string); s != "" {
		c.VAT = s
		c.Identifiers = append(c.Identifiers, identifierHit{
			Kind: "VAT", Value: s, Country: vatCountryHint(s),
			Validation: string(validateIdentifier("VAT", s, vatCountryHint(s))),
		})
	}
	if s, _ := m["taxID"].(string); s != "" {
		c.Register = s
		c.Identifiers = append(c.Identifiers, identifierHit{Kind: "taxID", Value: s})
	}
	// address.addressCountry can be a string or an object with "@value"/"name".
	explicitCountry := ""
	if addr, ok := m["address"]; ok {
		c.Address = addressTextFrom(addr)
		if addrMap, ok := addr.(map[string]interface{}); ok {
			explicitCountry = countryFromAddr(addrMap["addressCountry"])
		}
	}
	if c.Name != "" {
		if suf, cc, conf, ok := detectSuffix(c.Name); ok {
			c.Suffix = suf
			if explicitCountry == "" {
				c.Country = cc
				c.Confidence = conf
			}
		}
	}
	if c.Country == "" && explicitCountry != "" {
		c.Country = explicitCountry
		c.Confidence = "high"
	}
	// A generic schema.org Organization.name is usually a brand/website
	// title. Keep it only when it carries independent legal evidence (a
	// form suffix, identifier, or registered address); legalName itself is
	// an explicit claim.
	if !legalName && c.Name != "" && c.Suffix == "" && c.Register == "" && c.VAT == "" && c.Address == "" {
		c.Name = ""
	}
	return c
}

func addressTextFrom(v interface{}) string {
	switch t := v.(type) {
	case string:
		return collapseSpaces(t)
	case map[string]interface{}:
		parts := []string{}
		for _, key := range []string{"streetAddress", "postOfficeBoxNumber", "postalCode", "addressLocality", "addressRegion", "addressCountry"} {
			if s := countryFieldString(t[key]); s != "" {
				parts = append(parts, s)
			}
		}
		return collapseSpaces(strings.Join(parts, ", "))
	case []interface{}:
		for _, item := range t {
			if s := addressTextFrom(item); s != "" {
				return s
			}
		}
	}
	return ""
}

func countryFromAddr(v interface{}) string {
	switch t := v.(type) {
	case string:
		return normalizeCountry(t)
	case map[string]interface{}:
		if s, _ := t["@value"].(string); s != "" {
			return normalizeCountry(s)
		}
		if s, _ := t["name"].(string); s != "" {
			return normalizeCountry(s)
		}
	}
	return ""
}

func countryFieldString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case map[string]interface{}:
		if s, _ := t["name"].(string); s != "" {
			return strings.TrimSpace(s)
		}
		if s, _ := t["@value"].(string); s != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// --- hCard / vCard -------------------------------------------------------

func hasClass(cls, want string) bool {
	for _, t := range strings.Fields(cls) {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}

func extractHCard(doc *html.Node, pageURL string) []imprintCandidate {
	var out []imprintCandidate
	var org, country, address string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			class := ""
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "class") {
					class = a.Val
				}
			}
			if hasClass(class, "org") && org == "" {
				org = strings.TrimSpace(collectText(n))
			}
			if hasClass(class, "country-name") && country == "" {
				country = strings.TrimSpace(collectText(n))
			}
			if hasClass(class, "adr") && address == "" {
				address = collapseSpaces(collectText(n))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if org != "" {
		c := imprintCandidate{Name: org, Source: "hcard", Address: address}
		if country != "" {
			c.Country = normalizeCountry(country)
			c.Confidence = "high"
		}
		if suf, cc, conf, ok := detectSuffix(org); ok {
			c.Suffix = suf
			if c.Country == "" {
				c.Country = cc
				c.Confidence = conf
			}
		}
		out = append(out, c)
	}
	return out
}

// --- Open Graph ------------------------------------------------------------

func extractOG(doc *html.Node, pageURL string) []imprintCandidate {
	var siteName string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "meta" {
			var prop, content string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "property":
					prop = strings.ToLower(a.Val)
				case "name":
					if prop == "" {
						prop = strings.ToLower(a.Val)
					}
				case "content":
					content = a.Val
				}
			}
			if prop == "og:site_name" && content != "" && siteName == "" {
				siteName = strings.TrimSpace(content)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	if siteName == "" {
		return nil
	}
	c := imprintCandidate{Name: siteName, Source: "og_site_name", Confidence: "low"}
	if suf, cc, conf, ok := detectSuffix(siteName); ok {
		c.Suffix = suf
		c.Country = cc
		c.Confidence = conf
	}
	return []imprintCandidate{c}
}

// --- Footer copyright ------------------------------------------------------

func extractCopyrightFooter(body, pageURL string) []imprintCandidate {
	text := stripTagsLines(body)
	var out []imprintCandidate
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || len(line) > 140 {
			continue
		}
		if strings.Contains(line, "&quot;") || strings.Contains(line, "&nbsp;") ||
			strings.Contains(line, "&copy;") || strings.Contains(line, "&amp;") {
			continue
		}
		if !strings.Contains(line, "©") && !containsCI(line, "copyright") {
			continue
		}
		suf, cc, conf, ok := detectSuffix(line)
		if !ok {
			continue
		}
		name := extractEntityAround(line, suf)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, imprintCandidate{
			Name:       name,
			Suffix:     suf,
			Country:    cc,
			Confidence: conf,
			Source:     "copyright_footer",
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

// --- Imprint / Impressum text ---------------------------------------------

// extractImprintText line-scans for a legal-form suffix, extracts the entity
// name around it, the address on the following lines, and attaches nearby
// VAT/register identifiers within an ~800-byte proximity window (gated by
// country coherence — an identifier whose jurisdiction conflicts with the
// candidate's own is not attached). If that suffix-anchored scan finds
// nothing anywhere on the page, falls back to an explicit trading-name
// label (labeledNameRE — Firmenname:/Company name:/Unternehmen:/Inhaber:),
// tagged with Source "imprint_label" rather than "imprint_text" so
// candidateSourceRank can still prefer a real suffix match over it. This
// fallback exists because a legal-form suffix (GmbH/AG/Ltd/...) is not
// universal: a sole trader (Einzelunternehmen) legitimately has none at
// all, which the suffix scan alone can never find — confirmed via a real,
// live Austrian sole-trader Impressum (see
// imprint_extract_real_evidence_test.go).
func extractImprintText(body, pageURL string) []imprintCandidate {
	low := strings.ToLower(pageURL)
	isLegalPage := strings.Contains(low, "imprint") || strings.Contains(low, "impressum") ||
		strings.Contains(low, "legal") || strings.Contains(low, "colophon") ||
		// "colofon" (no 'h') is the actual Dutch spelling used by real
		// colofon pages (see patterns.go's DocImprint path list, which
		// already used the correct spelling) — this gate had drifted to the
		// English "colophon" only, so a Dutch imprint page with no
		// otherwise-recognisable VAT/register ID on it would silently skip
		// extraction. Real evidence: simpelbootverhuurutrecht.nl/colofon/.
		strings.Contains(low, "colofon") ||
		strings.Contains(low, "terms") || strings.Contains(low, "company") ||
		strings.Contains(low, "about")
	text := stripTagsLines(body)
	if !isLegalPage {
		// Still useful only if VAT/register IDs are present.
		ids := findIdentifiers(text, 3)
		if len(ids) == 0 {
			return nil
		}
	}
	var out []imprintCandidate
	// Track each candidate's byte offset so we only attach IDs that live
	// close to it — the first candidate must not vacuum up every ID on a
	// multi-entity imprint.
	candOffset := map[string]int{}
	seen := map[string]bool{}
	cursor := 0
	lines := strings.Split(text, "\n")
	// labelName/labelOffset track the best labeledNameRE fallback hit found
	// while scanning (see below) — only used if the suffix-anchored scan
	// above finds nothing anywhere on the page.
	var labelName string
	var labelOffset int
	for idx, raw := range lines {
		lineStart := cursor
		cursor += len(raw) + 1 // +1 for the '\n' separator
		line := strings.TrimSpace(raw)
		// Real evidence: eurostarshotels.com's real aviso legal states its
		// identity as one unbroken sentence — name, register entry, CIF,
		// and phone all in a single 172-character paragraph with no <br>
		// breaks at all (common in Spanish/civil-law legal-boilerplate
		// style). The original 140-char cap silently dropped it entirely
		// before ever reaching detectSuffix. Raised to 220 — still well
		// short of unrelated marketing prose, and hasProximityCorroboration
		// (imprint.go) already guards the false-positive risk this cap was
		// presumably meant to bound, by requiring a nearby address/
		// register/VAT before a plain-text suffix match can win at all.
		if line == "" || len(line) > 220 {
			continue
		}
		if strings.Contains(line, "&quot;") || strings.Contains(line, "&nbsp;") ||
			strings.Contains(line, "&copy;") || strings.Contains(line, "&amp;") {
			continue
		}
		// Sole-trader fallback (Bug 2): an explicit trading-name label
		// (Firmenname:/Company name:/Unternehmen:/Inhaber:) with no
		// legal-form suffix required at all — a sole trader
		// (Einzelunternehmen) legitimately has none. Tried on every
		// candidate line regardless of whether THIS line also carries a
		// suffix, but only actually used below if the suffix-anchored scan
		// comes up completely empty (a suffix match is a strictly stronger
		// signal than a bare label).
		if labelName == "" {
			if m := labeledNameRE.FindStringSubmatch(line); m != nil {
				cand := collapseSpaces(strings.TrimSpace(m[1]))
				if cand == "" {
					// Label alone on its own line: the value is on the
					// next non-empty line.
					for j := idx + 1; j < len(lines) && j <= idx+2; j++ {
						next := strings.TrimSpace(lines[j])
						if next != "" {
							cand = collapseSpaces(next)
							break
						}
					}
				}
				if cand != "" {
					if _, _, _, sufOK := detectSuffix(cand); !sufOK && validLabeledName(cand) {
						labelName = cand
						labelOffset = lineStart
					}
				}
			}
		}
		suf, cc, conf, ok := detectSuffix(line)
		if !ok {
			continue
		}
		name := extractEntityAround(line, suf)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, imprintCandidate{
			Name:       name,
			Suffix:     suf,
			Country:    cc,
			Confidence: conf,
			Source:     "imprint_text",
			Address:    extractAddressNearEntity(text, name),
		})
		candOffset[name] = lineStart
		if len(out) >= 5 {
			break
		}
	}
	if len(out) == 0 && labelName != "" {
		out = append(out, imprintCandidate{
			Name:    labelName,
			Source:  "imprint_label",
			Address: extractAddressNearEntity(text, labelName),
		})
		candOffset[labelName] = labelOffset
	} else if len(out) == 0 {
		if name := standaloneLegalFormCandidate(lines); name != "" {
			if offset := strings.Index(text, name); offset >= 0 {
				out = append(out, imprintCandidate{
					Name:    name,
					Source:  "imprint_label",
					Address: extractAddressNearEntity(text, name),
				})
				candOffset[name] = offset
			}
		}
	}
	const proximityBytes = 800
	ids := findIdentifiers(text, 5)
	if len(out) > 0 && len(ids) > 0 {
		for _, id := range ids {
			idOffset := strings.Index(text, id.Value)
			if idOffset < 0 {
				continue
			}
			best := -1
			bestDist := proximityBytes + 1
			for ci := range out {
				off, ok := candOffset[out[ci].Name]
				if !ok {
					continue
				}
				dist := idOffset - off
				if dist < 0 {
					dist = -dist
				}
				if dist < bestDist {
					bestDist = dist
					best = ci
				}
			}
			if best < 0 {
				continue
			}
			// Upstream (go_legal_entity) gates this attachment on a country
			// coherence check: reject when the identifier's jurisdiction
			// conflicts with the candidate's already-inferred country. That
			// makes sense for its multi-page/multi-entity crawl context,
			// but here it actively misfires: a shared multi-country legal
			// form (bare "GmbH" is DE-or-AT ambiguous — the trimmed
			// suffixTable only carries a DE entry, see imprint_suffix.go)
			// would cause the gate to reject a perfectly legitimate
			// Austrian VAT/FN attachment simply because the suffix guess
			// said "DE". An identifier's own jurisdiction prefix (ATU...,
			// a DE VAT, ...) is more specific and authoritative than a
			// suffix guess, so attachment is never gated on country here —
			// extractImprintFields (imprint.go) applies the real
			// precedence afterwards: the identifier's own country outranks
			// the candidate's suffix-inferred country when they disagree.
			switch id.Kind {
			case "HRB", "HRA", "Steuernummer", "USt-IdNr", "FN", "CompaniesHouse",
				"EIN", "ABN", "ACN", "CUI", "KvK", "SIRET", "SIREN", "REA", "Hoja",
				"KRS", "REGON":
				if out[best].Register == "" {
					out[best].Register = formatRegister(id.Kind, id.Value)
				}
				out[best].Identifiers = append(out[best].Identifiers, id)
			// "PartitaIVA" (Italy), "CIF" (Spain), and "NIP" (Poland) are
			// VAT-equivalent — literally what their labels mean ("Partita
			// IVA" = VAT number; Spain's CIF/NIF doubles as the domestic
			// VAT identifier under LSSICE; Poland's NIP IS the VAT number,
			// just written without the "PL" country prefix on a domestic
			// page) — not a separate company register number the way
			// KvK/SIRET/HRB/KRS are. Real evidence: all three were
			// previously matched by findIdentifiers (imprint_vat.go already
			// had patterns for the first two) but silently dropped here
			// since none of these Kinds appeared in EITHER switch case at
			// all — simanova.it's real Partita IVA, eurostarshotels.com's
			// real CIF, and neptun.orlen.pl's real NIP were all found and
			// then discarded before ever reaching the winning candidate.
			case "VAT", "PartitaIVA", "CIF", "NIP":
				if out[best].VAT == "" {
					out[best].VAT = cleanIdentifierValue(id.Kind, id.Value)
					if out[best].Country == "" {
						out[best].Country = id.Country
						out[best].Confidence = "high"
					}
				}
				out[best].Identifiers = append(out[best].Identifiers, id)
			}
		}
		for i := range out {
			out[i].Identifiers = mergeIdentifiers(out[i].Identifiers)
		}
	}
	return out
}

// mergeIdentifiers dedups identifierHit entries by kind+value.
func mergeIdentifiers(in []identifierHit) []identifierHit {
	seen := map[string]bool{}
	var out []identifierHit
	for _, id := range in {
		if id.Kind == "" || id.Value == "" {
			continue
		}
		key := id.Kind + "|" + id.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, id)
	}
	return out
}

// collectText concatenates the text-node content under an *html.Node.
func collectText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
