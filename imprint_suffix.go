package main

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Vendored (trimmed) from go_legal_entity's entity_suffix.go / entity.go /
// entity_detect.go / entity_contains.go / entity_is.go (v1.10.14) — a
// real-data-tested legal-entity-suffix table used to anchor "which line names
// the company" during imprint text extraction.
//
// TRIMMED, deliberately: the upstream table also carries native-script
// (Cyrillic/Han/Hangul/Greek/Thai) suffixes and "prefix-form" jurisdictions
// (the legal form precedes the name — Russian "ООО «Яндекс»", Indonesian
// "PT Telkom", Vietnamese "Công ty TNHH ...") which need extra
// script-detection and reversed-extraction machinery upstream carries
// (isGluedScript, isPrefixForm, extractEntityAfterPrefix). This pass's
// rulesets (eu_baseline / de_tmg / at_ecg_medieng — see imprint.go) are all
// Latin-script, name-then-suffix jurisdictions, so that machinery is left out
// here rather than carried in unused; a future pass adding non-EU/non-Latin
// imprint rulesets should port it from go_legal_entity rather than
// reinventing it. All Latin-script, name-follows-suffix entries from the
// upstream table are kept (minus the two Vietnamese prefix-form entries,
// which the trimmed extractor here cannot correctly anchor).

// suffixEntry maps a legal-entity suffix (as it tends to appear in text) to
// its likely jurisdiction and confidence. Ordering only matters via
// suffixOrder, sorted by descending length so "GmbH & Co. KG" matches before
// "GmbH" and "Pte Ltd" before "Ltd".
type suffixEntry struct {
	suffix     string
	country    string // ISO-3166-1 alpha-2 inferred jurisdiction (best-effort)
	confidence string // high|medium|low
}

// suffixTable — see the file-level comment above for what was trimmed from
// go_legal_entity's version and why.
var suffixTable = []suffixEntry{
	// United States
	{"Inc.", "US", "medium"},
	{"Inc", "US", "low"},
	{"LLC", "US", "high"},
	{"L.L.C.", "US", "high"},
	{"PLLC", "US", "high"},
	{"P.C.", "US", "medium"},
	{"Corp.", "US", "medium"},
	{"Corporation", "US", "medium"},
	{"Co.", "US", "low"},
	{"Company", "US", "low"},
	{"LLLP", "US", "high"},
	{"LP", "US", "medium"},
	{"LLP", "US", "low"},

	// United Kingdom
	{"Ltd", "GB", "medium"},
	{"Ltd.", "GB", "medium"},
	{"Limited", "GB", "medium"},
	{"PLC", "GB", "high"},
	{"plc", "GB", "high"},
	// All-lowercase "ltd" — same shape as the "PLC"/"plc" pair just above
	// (this table already accepts both cases for that one). Real
	// evidence, round 23: epic.com.cy's real eStore Terms & Conditions
	// page stylises its own name as all-lowercase "epic ltd" throughout
	// (a brand-identity choice, not a jurisdiction signal — Cyprus is
	// itself a common-law "Ltd" jurisdiction like the existing GB tag
	// already covers, disambiguated from GB by ground-truth identifier
	// evidence — HE/VAT Number, imprint_vat.go — the same way the "Ltd"
	// entry's own GB-or-elsewhere ambiguity is already resolved
	// elsewhere in this table).
	{"ltd", "GB", "medium"},

	// Germany
	{"GmbH & Co. KG", "DE", "high"},
	{"GmbH", "DE", "high"},
	{"AG", "DE", "high"},
	{"KG", "DE", "high"},
	{"OHG", "DE", "high"},
	{"UG (haftungsbeschränkt)", "DE", "high"},
	{"e.V.", "DE", "high"},
	{"Stiftung", "DE", "medium"},
	{"SE", "EU", "medium"},

	// France
	{"SARL", "FR", "high"},
	{"SAS", "FR", "high"},
	{"S.A.S.", "FR", "high"},
	{"SA", "FR", "low"},
	{"S.A.", "FR", "low"},
	{"SCI", "FR", "high"},
	{"EURL", "FR", "high"},
	{"SNC", "FR", "high"},
	{"SCS", "FR", "high"},
	{"SCOP", "FR", "high"},

	// Netherlands
	{"B.V.", "NL", "high"},
	{"BV", "NL", "medium"},
	{"N.V.", "NL", "high"},
	{"NV", "NL", "medium"},
	{"V.O.F.", "NL", "high"},
	{"C.V.", "NL", "medium"},
	{"Coöperatie", "NL", "high"},
	{"Stichting", "NL", "high"},

	// Italy
	{"S.r.l.", "IT", "high"},
	{"Srl", "IT", "high"},
	// "S. R. L." (each letter separately dotted and spaced, all caps) is a
	// distinct real-world stylization from "S.r.l." — common on official
	// pages and in ALL-CAPS structured data/branding. Without its own
	// entry it matched nothing at all (imprint_suffix.go's suffixTable is
	// case-sensitive), or — worse — an all-caps "S.R.L." with NO internal
	// spaces would match Romania's own-name "S.R.L." entry below, giving a
	// genuinely Italian company the wrong country. Real evidence:
	// simanova.it's real note legali visibly reads "SIMANOVA S. R. L.".
	{"S. R. L.", "IT", "high"},
	{"S.p.A.", "IT", "high"},
	{"SpA", "IT", "medium"},
	{"S.n.c.", "IT", "high"},
	{"S.a.s.", "IT", "high"},

	// Spain
	{"S.L.U.", "ES", "high"},
	{"S.A.U.", "ES", "high"},
	{"S.L.", "ES", "high"},
	{"S.L", "ES", "medium"},
	// Undotted "SL" — a common real-world stylization with no entry at
	// all before this fix. Real evidence: eurostarshotels.com's real,
	// official aviso legal reads "EUROSTARS HOTEL COMPANY SL" (no periods
	// anywhere in the suffix).
	{"SL", "ES", "medium"},
	{"S. Coop.", "ES", "high"},

	// Poland
	{"Sp. z o.o.", "PL", "high"},
	{"sp. z o.o.", "PL", "high"},
	{"S.A. (Polska)", "PL", "medium"},
	{"Sp.j.", "PL", "high"},
	{"Sp.k.", "PL", "high"},

	// Sweden
	{"AB", "SE", "medium"},
	{"HB", "SE", "high"},
	{"KB", "SE", "high"},

	// Australia
	{"Pty Ltd", "AU", "high"},
	{"Pty. Ltd.", "AU", "high"},
	{"Pty Limited", "AU", "high"},

	// India
	{"Pvt Ltd", "IN", "high"},
	{"Pvt. Ltd.", "IN", "high"},
	{"Private Limited", "IN", "high"},

	// Singapore
	{"Pte Ltd", "SG", "high"},
	{"Pte. Ltd.", "SG", "high"},

	// Japan (Latin transliteration only — see file-level comment)
	{"K.K.", "JP", "high"},

	// Romania
	{"S.R.L.", "RO", "low"},

	// Ireland
	{"DAC", "IE", "high"},

	// Hong Kong / China (Latin form only)
	{"Co., Ltd.", "CN", "low"},

	// Belgium
	{"BVBA", "BE", "high"},
	{"SPRL", "BE", "high"},
	{"SRL", "BE", "low"},
	{"NV/SA", "BE", "high"},

	// Switzerland / Liechtenstein
	{"Sàrl", "CH", "high"},
	{"S.à r.l.", "LU", "high"},
	{"Sagl", "CH", "high"},
	{"GmbH (Schweiz)", "CH", "medium"},

	// Austria
	{"GesmbH", "AT", "high"},
	{"Ges.m.b.H.", "AT", "high"},

	// Denmark
	{"ApS", "DK", "high"},
	{"A/S", "DK", "high"},
	{"I/S", "DK", "high"},
	{"K/S", "DK", "high"},
	{"P/S", "DK", "high"},

	// Norway
	{"AS", "NO", "medium"},
	{"ASA", "NO", "high"},
	{"ANS", "NO", "high"},

	// Finland
	{"Oy", "FI", "high"},
	{"Oyj", "FI", "high"},
	{"Ky", "FI", "medium"},

	// Czechia / Slovakia
	{"s.r.o.", "CZ", "high"},
	{"a.s.", "CZ", "medium"},
	{"spol. s r.o.", "CZ", "high"},
	{"k.s.", "CZ", "medium"},
	{"v.o.s.", "CZ", "high"},

	// Hungary
	{"Kft.", "HU", "high"},
	{"Zrt.", "HU", "high"},
	{"Nyrt.", "HU", "high"},
	{"Bt.", "HU", "medium"},

	// Portugal / Brazil
	{"Lda.", "PT", "high"},
	{"Lda", "PT", "high"},
	{"Unipessoal Lda", "PT", "high"},
	{"Ltda.", "BR", "high"},
	{"Ltda", "BR", "high"},
	{"S.A.", "BR", "low"},
	{"EIRELI", "BR", "high"},
	{"MEI", "BR", "medium"},

	// Mexico / LatAm — compound forms (longest, ranked first).
	{"S.A.P.I. de C.V.", "MX", "high"},
	{"S. de R.L. de C.V.", "MX", "high"},
	{"S.A.B. de C.V.", "MX", "high"},
	{"S.A. de C.V.", "MX", "high"},
	{"S.A.B.", "MX", "high"},
	{"S. de R.L.", "MX", "high"},
	{"S.A.C.", "PE", "high"},
	{"S.A.S.", "CO", "low"},
	{"C.A.", "VE", "medium"},
	{"S.A. de C.V. de R.L.", "MX", "medium"},

	// Indonesia — "Tbk" is the public-company marker (matched as a trailing
	// token; the "PT" prefix marker itself is a prefix form, not included).
	{"(Persero) Tbk", "ID", "high"},
	{"Tbk", "ID", "high"},
	{"Perseroan Terbatas", "ID", "high"},

	// Malaysia
	{"Sdn Bhd", "MY", "high"},
	{"Sdn. Bhd.", "MY", "high"},
	{"Berhad", "MY", "high"},
	{"Bhd", "MY", "medium"},

	// Gulf / Middle East — public joint-stock companies.
	{"P.J.S.C.", "AE", "high"},
	{"PJSC", "AE", "high"},
	{"P.S.C.", "AE", "high"},
	{"PSC", "AE", "medium"},
	{"P.J.S.C", "AE", "medium"},
	{"W.L.L.", "BH", "high"},
	{"WLL", "QA", "high"},
	{"L.L.C. (UAE)", "AE", "medium"},

	// Turkey
	{"A.Ş.", "TR", "high"},
	{"Ltd. Şti.", "TR", "high"},
	{"Anonim Şirketi", "TR", "high"},

	// Russia / CIS — Latin transliterations (native Cyrillic forms are out of
	// scope for this trimmed table — see file-level comment).
	{"OOO", "RU", "high"},
	{"OAO", "RU", "high"},
	{"ZAO", "RU", "high"},
	{"PAO", "RU", "high"},

	// Bulgaria, round 19 — NATIVE Cyrillic forms (no prior BG suffix
	// entries existed at all, Latin or otherwise). Real evidence:
	// cressi.bg's real Общи условия page names "Кеме ЕООД" — confirmed via
	// this ONE fetched page, so only "ЕООД" is real-evidence-confirmed
	// this round. "ООД"/"АД"/"ЕАД" are added alongside it on the same
	// well-established-sibling-forms basis used for Greece's native forms
	// in round 18 — zero collision risk (Bulgarian Cyrillic letter
	// sequences here don't match any Russian Cyrillic entry this table
	// might one day gain: "ЕООД"/"ООД"/"АД"/"ЕАД" are distinct sequences
	// from "ООО"/"ОАО"/"ЗАО"/"ПАО" above, letter-for-letter).
	{"ЕООД", "BG", "high"},
	{"ООД", "BG", "high"},
	{"АД", "BG", "medium"},
	{"ЕАД", "BG", "high"},

	// Croatia, round 20 — no prior HR suffix entries existed at all. Real
	// evidence: mako.hr's real Opći uvjeti page names "Mako d.o.o.". "j.d.o.o."
	// (simple LLC, a reduced-capital variant introduced 2012) and "d.d."
	// (joint-stock company) added alongside it on the same
	// well-established-sibling-forms basis used for Greece/Bulgaria in
	// rounds 18-19.
	//
	// Known, accepted collision risk (same class already tolerated
	// elsewhere in this table for "S.A."/France-Romania and
	// "S.R.L."/Italy-Romania): "d.o.o." is shared, letter-for-letter, by
	// EVERY former-Yugoslav-republic legal system — Slovenia writes its
	// own "družba z omejeno odgovornostjo" the identical way. Slovenia is
	// a later target in this same real-evidence series and has no entry
	// here yet, so this table has no ACTUAL collision to resolve today —
	// but ground-truth identifier evidence (OIB/MBS, imprint_vat.go) is
	// wired below via singleCountryIdentifierKind precisely so a future
	// Slovenian real-evidence round can self-heal the same way the
	// existing S.A./S.R.L. collisions already do, rather than needing
	// suffixTable itself to somehow encode the ambiguity.
	{"d.o.o.", "HR", "high"},
	{"j.d.o.o.", "HR", "high"},
	{"d.d.", "HR", "medium"},

	// Greece (Latin transliteration; native Greek forms out of scope here)
	{"A.E.", "GR", "high"},
	{"E.P.E.", "GR", "high"},
	{"O.E.", "GR", "medium"},
	{"I.K.E.", "GR", "high"},
	// Greece, round 18 — the ACTUAL native Greek Unicode forms real Greek
	// pages use (Greek Iota/Kappa/Epsilon/Alpha/Phi/... are DIFFERENT
	// Unicode code points from the visually-similar Latin letters above,
	// e.g. Greek "Ι" U+0399 vs Latin "I" U+0049 — a plain-text match on the
	// Latin entries above can never match this real page's own text). Real
	// evidence: thikishop.gr's real Όροι Χρήσης page names "MASTER
	// ACCESSORIES Ι.Κ.Ε." — confirmed via this ONE fetched page, so only
	// "Ι.Κ.Ε." is real-evidence-confirmed this round. "Α.Ε."/"Ε.Π.Ε."/"Ο.Ε."
	// are added alongside it on the same basis as the Latin quartet just
	// above (the same four well-established Greek legal forms, just their
	// native-script spelling) rather than left half-fixed — zero collision
	// risk (Greek script appears nowhere else in this table) and a
	// trivially-verifiable alphabet mapping, not speculative business data.
	{"Ι.Κ.Ε.", "GR", "high"},
	{"Α.Ε.", "GR", "high"},
	{"Ε.Π.Ε.", "GR", "high"},
	{"Ο.Ε.", "GR", "medium"},

	// Estonia / Latvia / Lithuania
	{"OÜ", "EE", "high"},
	{"SIA", "LV", "high"},
	{"UAB", "LT", "high"},
	{"AB (Lietuva)", "LT", "medium"},

	// Canada
	{"ULC", "CA", "high"},
	{"Ltée", "CA", "high"},
	{"Inc. (Canada)", "CA", "medium"},

	// South Africa
	{"(Pty) Ltd", "ZA", "high"},
	{"(Pty) Ltd.", "ZA", "high"},
	{"Ltd (RF)", "ZA", "medium"},

	// United Arab Emirates / Gulf free-zone
	{"FZ-LLC", "AE", "high"},
	{"FZE", "AE", "high"},
	{"DMCC", "AE", "high"},

	// Ireland extra
	{"Teoranta", "IE", "high"},
	{"Teo.", "IE", "high"},

	// Romania extra
	{"S.A. (România)", "RO", "medium"},
	{"PFA", "RO", "high"},
}

// suffixOrder is suffixTable sorted by descending raw-suffix length, so
// "GmbH & Co. KG" matches before "GmbH" and "Pte Ltd" before "Ltd".
var suffixOrder []suffixEntry

func init() {
	suffixOrder = append(suffixOrder, suffixTable...)
	sort.SliceStable(suffixOrder, func(i, j int) bool {
		return len(suffixOrder[i].suffix) > len(suffixOrder[j].suffix)
	})
}

// detectSuffix searches `text` for any known legal-entity suffix and returns
// the first (longest) hit.
func detectSuffix(text string) (suffix, country, confidence string, ok bool) {
	for _, e := range suffixOrder {
		if containsSuffix(text, e.suffix) {
			return e.suffix, e.country, e.confidence, true
		}
	}
	return "", "", "", false
}

// containsSuffix returns true if text contains the suffix as a token (loose
// word boundary) — avoids e.g. matching "Inc." inside "Inca". Also rejects
// suffixes preceded by '(' — that shape is almost always a parenthetical
// aside like "Equity crowdfunding (Limited availability)", not a corporate
// suffix. Boundary detection is Unicode-aware: an accented Latin letter
// immediately adjacent to the match is treated as a continuing word
// character.
func containsSuffix(text, suffix string) bool {
	idx := 0
	for {
		j := strings.Index(text[idx:], suffix)
		if j < 0 {
			return false
		}
		start := idx + j
		end := start + len(suffix)
		if end < len(text) {
			if r, _ := utf8.DecodeRuneInString(text[end:]); isWordRune(r) {
				idx = end
				continue
			}
		}
		if start > 0 {
			r := prevRune(text, start)
			if isWordRune(r) || r == '(' {
				idx = end
				continue
			}
		}
		return true
	}
}

// isWordRune reports whether r is a letter or digit in any script.
func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// prevRune decodes the rune ending immediately before byte offset pos.
func prevRune(s string, pos int) rune {
	r, _ := utf8.DecodeLastRuneInString(s[:pos])
	return r
}

// suffixHasSlash reports whether a legal-form suffix legitimately contains a
// '/' (A/S, K/S, I/S, P/S, NV/SA) — relaxes the slash-reject in
// cleanCandidateName for Danish/Norwegian/Belgian names.
func suffixHasSlash(suffix string) bool {
	return strings.Contains(suffix, "/")
}
