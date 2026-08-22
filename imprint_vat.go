package main

import "regexp"

// This file is vendored (and lightly adapted) from go_legal_entity's vat.go
// (v1.10.14) — a mature, real-data-tested VAT/register-ID identifier table
// covering 27 EU member states plus common non-EU national identifiers
// (CompaniesHouse, EIN, ABN/ACN, SIRET/SIREN, KvK, CUI, ...). Rather than
// re-derive this from scratch, go_tos_finder reuses it as the identifier
// layer for structured imprint (Impressum) field extraction — see
// imprint.go's extractImprintFields.
//
// One real gap found while porting: the source table has no Austrian
// Firmenbuch company-register pattern (only German HRB/HRA/Steuernummer) —
// an odd omission given the table's VAT entry already covers Austrian VAT
// (ATU########). Added below as "FN" so an Austrian imprint's "FN 123456a"
// register number (ECG §5 / UGB §14 disclosure requirement) is actually
// captured, not silently dropped.

// vatPattern names a regex that identifies a tax/registration ID and the
// country it implies. We use Go's regexp (RE2), so no lookbehind — patterns
// are anchored to a literal prefix where possible.
type vatPattern struct {
	name    string
	country string
	re      *regexp.Regexp
}

// Compiled once at init.
var vatPatterns []vatPattern

func init() {
	vatPatterns = []vatPattern{
		// EU VAT numbers — common formats.
		{"VAT", "AT", regexp.MustCompile(`\bATU\d{8}\b`)},
		{"VAT", "BE", regexp.MustCompile(`\bBE0\d{9}\b`)},
		{"VAT", "BG", regexp.MustCompile(`\bBG\d{9,10}\b`)},
		{"VAT", "CY", regexp.MustCompile(`\bCY\d{8}[A-Z]\b`)},
		{"VAT", "CZ", regexp.MustCompile(`\bCZ\d{8,10}\b`)},
		{"VAT", "DE", regexp.MustCompile(`\bDE\d{9}\b`)},
		{"VAT", "DK", regexp.MustCompile(`\bDK\d{8}\b`)},
		{"VAT", "EE", regexp.MustCompile(`\bEE\d{9}\b`)},
		{"VAT", "EL", regexp.MustCompile(`\bEL\d{9}\b`)},
		{"VAT", "ES", regexp.MustCompile(`\bES[A-Z0-9]\d{7}[A-Z0-9]\b`)},
		{"VAT", "FI", regexp.MustCompile(`\bFI\d{8}\b`)},
		{"VAT", "FR", regexp.MustCompile(`\bFR[A-Z0-9]{2}\d{9}\b`)},
		{"VAT", "GB", regexp.MustCompile(`\bGB\d{9}\b`)},
		{"VAT", "HR", regexp.MustCompile(`\bHR\d{11}\b`)},
		{"VAT", "HU", regexp.MustCompile(`\bHU\d{8}\b`)},
		{"VAT", "IE", regexp.MustCompile(`\bIE\d{7}[A-Z]{1,2}\b`)},
		{"VAT", "IT", regexp.MustCompile(`\bIT\d{11}\b`)},
		{"VAT", "LT", regexp.MustCompile(`\bLT(\d{9}|\d{12})\b`)},
		{"VAT", "LU", regexp.MustCompile(`\bLU\d{8}\b`)},
		{"VAT", "LV", regexp.MustCompile(`\bLV\d{11}\b`)},
		{"VAT", "MT", regexp.MustCompile(`\bMT\d{8}\b`)},
		{"VAT", "NL", regexp.MustCompile(`\bNL\d{9}B\d{2}\b`)},
		{"VAT", "PL", regexp.MustCompile(`\bPL\d{10}\b`)},
		{"VAT", "PT", regexp.MustCompile(`\bPT\d{9}\b`)},
		{"VAT", "RO", regexp.MustCompile(`\bRO\d{2,10}\b`)},
		{"VAT", "SE", regexp.MustCompile(`\bSE\d{12}\b`)},
		{"VAT", "SI", regexp.MustCompile(`\bSI\d{8}\b`)},
		{"VAT", "SK", regexp.MustCompile(`\bSK\d{10}\b`)},

		// National identifiers.
		{"HRB", "DE", regexp.MustCompile(`\bHRB\s?\d{1,6}\b`)},
		{"HRA", "DE", regexp.MustCompile(`\bHRA\s?\d{1,6}\b`)},
		{"USt-IdNr", "DE", regexp.MustCompile(`USt[- ]?IdNr\.?\s*:?\s*DE\d{9}`)},
		{"Steuernummer", "DE", regexp.MustCompile(`Steuernummer\s*:?\s*[\d/]{10,15}`)},
		// Austria — Firmenbuch company-register number (ECG §5 / UGB §14):
		// "FN" followed by 4-6 digits and a single check letter, e.g.
		// "FN 123456a". Not present in the upstream go_legal_entity table —
		// added here since Austrian imprint completeness depends on it.
		{"FN", "AT", regexp.MustCompile(`\bFN\s?\d{4,6}\s?[a-zA-Z]\b`)},
		// CompaniesHouse: require the "Company No"/"Company Number"/"Registered
		// in England No" prefix. A bare 8-digit number is far too loose
		// (matched timestamps, postcodes, product IDs); the prefix is what
		// makes it a register hit. We capture the prefix as part of the
		// match string so downstream consumers can audit context.
		{"CompaniesHouse", "GB", regexp.MustCompile(`\b(?:Company\s*(?:No|Number)|Registered\s+(?:in\s+(?:England|Scotland|Wales|England\s+and\s+Wales)\s+)?(?:No|Number)|Registration\s+(?:No|Number))\.?\s*[:#]?\s*\d{7,8}\b`)},
		{"EIN", "US", regexp.MustCompile(`\bEIN\s*[:#]?\s*\d{2}-\d{7}\b`)},
		{"ABN", "AU", regexp.MustCompile(`\bABN\s*[:#]?\s*\d{2}\s?\d{3}\s?\d{3}\s?\d{3}\b`)},
		{"ACN", "AU", regexp.MustCompile(`\bACN\s*[:#]?\s*\d{3}\s?\d{3}\s?\d{3}\b`)},
		{"CUI", "RO", regexp.MustCompile(`\b(?:CUI|CIF)\s*[:#]?\s*(?:RO)?\d{2,10}\b`)},
		{"KvK", "NL", regexp.MustCompile(`\bKvK[- ]?(?:nr\.?)?\s*[:#]?\s*\d{8}\b`)},
		{"SIRET", "FR", regexp.MustCompile(`\bSIRET\s*[:#]?\s*\d{3}\s?\d{3}\s?\d{3}\s?\d{5}\b`)},
		{"SIREN", "FR", regexp.MustCompile(`\bSIREN\s*[:#]?\s*\d{3}\s?\d{3}\s?\d{3}\b`)},
		// Norway — Organisasjonsnummer / MVA (9 digits, often "NO" + "MVA").
		{"OrgNr", "NO", regexp.MustCompile(`\b(?:Org(?:anisasjonsnr|\.?\s?nr)?|MVA)\.?\s*[:#]?\s*(?:NO)?\s*\d{3}\s?\d{3}\s?\d{3}(?:\s?MVA)?\b`)},
		// Switzerland — UID (CHE-###.###.###).
		{"UID", "CH", regexp.MustCompile(`\bCHE[- ]?\d{3}\.?\d{3}\.?\d{3}(?:\s?(?:MWST|TVA|IVA|VAT))?\b`)},
		// Spain — CIF/NIF on the imprint (letter + 7 digits + control).
		{"CIF", "ES", regexp.MustCompile(`\b(?:CIF|NIF)\s*[:#]?\s*[A-HJ-NP-SUVW]\d{7}[0-9A-J]\b`)},
		// Italy — Codice Fiscale / Partita IVA (11 digits) with explicit label.
		{"PartitaIVA", "IT", regexp.MustCompile(`\b(?:Partita\s*IVA|P\.?\s?IVA|C\.?F\.?)\s*[:#]?\s*\d{11}\b`)},
		// Italy — REA (Repertorio Economico Amministrativo), the Chamber of
		// Commerce business-register number D.Lgs. 70/2003 Art. 7 requires
		// alongside the Partita IVA. Not in the original go_legal_entity
		// table at all — an Italian imprint's register disclosure was
		// silently unmodeled. Real evidence: simanova.it's real note legali
		// ("REA CE 362094"); the optional province-code and label variants
		// below ("Numero REA: (AL) 217552", "REA nr. MI - 2677988") were
		// observed across several other real Italian note-legali pages
		// during the same search.
		{"REA", "IT", regexp.MustCompile(`\b(?:Numero\s+)?REA\.?\s*(?:nr\.?)?\s*:?\s*\(?[A-Z]{0,2}\)?[\s.-]?\d{4,8}\b`)},
		// Brazil — CNPJ (##.###.###/####-##).
		{"CNPJ", "BR", regexp.MustCompile(`\b(?:CNPJ)?\s*[:#]?\s*\d{2}\.\d{3}\.\d{3}/\d{4}-\d{2}\b`)},
		// New Zealand — Company / NZBN (13 digits) with label.
		{"NZBN", "NZ", regexp.MustCompile(`\bNZBN\s*[:#]?\s*\d{13}\b`)},
	}
}

// identifierHit is one matched VAT/register identifier, carrying its own
// structural-validation verdict — the evidence-trail field that proves an
// identifier is structurally sound without an external registry call.
type identifierHit struct {
	Kind       string
	Value      string
	Country    string
	Validation string
}

// findIdentifiers scans text for VAT/register IDs. Returns up to `max` hits.
func findIdentifiers(text string, max int) []identifierHit {
	var hits []identifierHit
	seen := map[string]bool{}
	for _, p := range vatPatterns {
		for _, m := range p.re.FindAllString(text, -1) {
			key := p.name + ":" + m
			if seen[key] {
				continue
			}
			seen[key] = true
			hits = append(hits, identifierHit{
				Kind:       p.name,
				Value:      m,
				Country:    p.country,
				Validation: string(validateIdentifier(p.name, m, p.country)),
			})
			if len(hits) >= max {
				return hits
			}
		}
	}
	return hits
}
