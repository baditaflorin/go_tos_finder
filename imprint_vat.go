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
		// Optional space after "BE": real evidence, factuo.be's real
		// mentions légales writes "TVA BE 0704742612" (space, no space
		// within the 10-digit body) — the original pattern required the
		// digits to butt directly against "BE" with no space at all.
		{"VAT", "BE", regexp.MustCompile(`\bBE\s?0\d{9}\b`)},
		{"VAT", "BG", regexp.MustCompile(`\bBG\d{9,10}\b`)},
		{"VAT", "CY", regexp.MustCompile(`\bCY\d{8}[A-Z]\b`)},
		{"VAT", "CZ", regexp.MustCompile(`\bCZ\d{8,10}\b`)},
		{"VAT", "DE", regexp.MustCompile(`\bDE\d{9}\b`)},
		{"VAT", "DK", regexp.MustCompile(`\bDK\d{8}\b`)},
		{"VAT", "EE", regexp.MustCompile(`\bEE\d{9}\b`)},
		{"VAT", "EL", regexp.MustCompile(`\bEL\d{9}\b`)},
		{"VAT", "ES", regexp.MustCompile(`\bES[A-Z0-9]\d{7}[A-Z0-9]\b`)},
		{"VAT", "FI", regexp.MustCompile(`\bFI\d{8}\b`)},
		// Optional spaces throughout: real evidence, factuo.be's real
		// mentions légales (naming its French hosting provider) writes
		// "FR 29 421 527 797" — check-chars and every digit group
		// space-separated, none of which the original contiguous pattern
		// allowed for.
		{"VAT", "FR", regexp.MustCompile(`\bFR\s?[A-Z0-9]{2}\s?\d{3}\s?\d{3}\s?\d{3}\b`)},
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
		// Case-insensitive: real evidence, humresto.fr's real mentions-légales
		// writes "N° Siret : 980 184 584 000 12" (mixed case, not "SIRET"),
		// which the previously case-sensitive pattern never matched at all —
		// only surfaced once RulesetFRLCEN started requiring "register" and
		// this real page's completeness score came up short. The trailing
		// alternation ("(?:\d{5}|\d{3}\s?\d{2})") accepts the SIRET's last
		// 5-digit NIC segment written either solid or as "3+2" — the same
		// real page groups the full 14 digits "980 184 584 000 12" (3-3-3-
		// 3-2), not the "3-3-3-5" grouping the pattern previously assumed.
		{"SIRET", "FR", regexp.MustCompile(`(?i)\bSIRET\s*[:#]?\s*\d{3}\s?\d{3}\s?\d{3}\s?(?:\d{5}|\d{3}\s?\d{2})\b`)},
		{"SIREN", "FR", regexp.MustCompile(`(?i)\bSIREN\s*[:#]?\s*\d{3}\s?\d{3}\s?\d{3}\b`)},
		// Norway — Organisasjonsnummer / MVA (9 digits, often "NO" + "MVA").
		// Case-insensitive: real evidence, japanphoto.no's real kjøpsvilkår
		// page opens with "...fra CEWE Norge AS med org.nr. 965 321 039..."
		// — ordinary mid-sentence Norwegian prose lower-cases "org.nr" (only
		// a sentence-initial or label form capitalizes it, e.g. the SAME
		// page's later "Org.nr: 965 321 039"), which the previously
		// case-sensitive pattern (requiring a literal capital "Org") never
		// matched at all. Same class of fix as round 7's BE/FR spacing gap.
		{"OrgNr", "NO", regexp.MustCompile(`(?i)\b(?:Org(?:anisasjonsnr|\.?\s?nr)?|MVA)\.?\s*[:#]?\s*(?:NO)?\s*\d{3}\s?\d{3}\s?\d{3}(?:\s?MVA)?\b`)},
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
		// Spain — Registro Mercantil "Hoja" number (e.g. "Hoja B-372183",
		// "hoja SE - 76.664"), LSSICE Art. 10's register-inscription
		// disclosure. A full citation also carries a tomo/folio/libro/
		// sección, but "Hoja <province-code>-<number>" is the one
		// consistently-present, uniquely-identifying part across every real
		// citation observed — real evidence: eurostarshotels.com's real
		// aviso legal ("Hoja B-372183") plus several other real Spanish
		// aviso-legal pages found during the same search (Sevilla "hoja SE
		// - 76.664", Málaga "Hoja MA-111580", another Barcelona filing
		// "hoja B-493395"). Not in the original go_legal_entity table at
		// all — a Spanish imprint's register disclosure was silently
		// unmodeled (only CIF/NIF, the tax ID, was covered).
		{"Hoja", "ES", regexp.MustCompile(`(?i)\bHoja\s*:?\s*[A-Z]{0,3}\s*[-.]?\s*\d[\d.]{2,9}\b`)},
		// Poland — NIP (tax ID, 10 digits), the domestic label-anchored
		// form written WITHOUT the "PL" country prefix the existing
		// VAT/PL pattern above requires (that form is for cross-border EU
		// VAT use; NIP on a domestic imprint never carries the prefix).
		// Same architecture as PartitaIVA (Italy) / CIF (Spain): a
		// label-anchored identifier the generic country-prefixed VAT
		// pattern cannot see at all. Accepts the optional dash-grouped
		// "###-###-##-##" form real evidence also showed alongside the
		// solid 10-digit form. Real evidence: neptun.orlen.pl's real "Dane
		// kontaktowe i rejestrowe" page ("NIP: 5252855028"); the dashed
		// form ("NIP: 583-001-89-31") was independently observed on
		// another real Polish company's registry-data page during the
		// same search.
		{"NIP", "PL", regexp.MustCompile(`\bNIP\s*[:#]?\s*\d{3}-?\d{3}-?\d{2}-?\d{2}\b`)},
		// Poland — KRS (Krajowy Rejestr Sądowy), the National Court
		// Register entry number — Poland's equivalent of Germany's HRB or
		// Italy's REA. Not in the original table at all. Real evidence:
		// neptun.orlen.pl ("KRS: 0000888254").
		{"KRS", "PL", regexp.MustCompile(`\bKRS\s*[:#]?\s*\d{10}\b`)},
		// Poland — REGON (statistical/business ID, 9 or 14 digits) —
		// commonly disclosed alongside NIP/KRS on a real Polish imprint,
		// though not independently required by law the way NIP/KRS are.
		// Real evidence: neptun.orlen.pl ("REGON: 388432405").
		{"REGON", "PL", regexp.MustCompile(`\bREGON\s*[:#]?\s*\d{9}(?:\d{5})?\b`)},
		// Portugal — NIPC (Número de Identificação de Pessoa Coletiva, 9
		// digits), the domestic label-anchored form written WITHOUT the
		// "PT" country prefix the existing VAT/PT pattern above requires
		// — same architecture as NIP (Poland) / PartitaIVA (Italy) / CIF
		// (Spain). Real evidence: urbana.com.pt's real "Ficha técnica"
		// page ("NIPC -508980186").
		{"NIPC", "PT", regexp.MustCompile(`\bNIPC\s*[:#-]?\s*\d{9}\b`)},
		// Ireland — CRO (Companies Registration Office) company number, the
		// register-number disclosure S.I. No. 68/2003 (European Communities
		// (Directive 2000/31/EC) Regulations 2003), reg. 6(a) requires
		// alongside name/address/contact. Distinct from the "CompaniesHouse"
		// pattern above: UK Companies House numbers are always 7-8 digits
		// (zero-padded), while a real Irish CRO number can be shorter — real
		// evidence: proxima.ie's real website footer ("Company Registration
		// No: 613314") is 6 digits, which the UK pattern's \d{7,8} cannot
		// match at all. Given its own Kind ("CRO", not folded into
		// "CompaniesHouse") so singleCountryIdentifierKind
		// (imprint_checksum.go) can correct a real, live country
		// mis-attribution: suffixTable (imprint_suffix.go) maps bare "Ltd"
		// only to GB, so this same real Irish company's Country came out
		// "GB" before this fix — an unambiguous domestic register-number
		// label is exactly the kind of ground-truth evidence this codebase
		// already uses to override a suffix guess (see the REA/Hoja/KvK/NIPC
		// precedent cited in imprint.go's extractImprintFields).
		{"CRO", "IE", regexp.MustCompile(`(?i)\b(?:CRO\s*(?:No\.?|Number|Reg(?:istration)?\.?(?:\s*(?:No\.?|Number))?)|Compan(?:y|ies)\s+Registration(?:\s+Office)?(?:\s+No\.?|\s+Number)?)\s*[:#]?\s*\d{5,7}\b`)},
		// Luxembourg — RCS (Registre de Commerce et des Sociétés) company
		// number, the register disclosure the Loi du 14 août 2000 relative
		// au commerce électronique (Luxembourg's Directive 2000/31/EC
		// transposition), Art. 4 requires alongside name/address/contact.
		// Requiring a LETTER immediately before the digits (Luxembourg's
		// shape: a single section letter — "B" for the commercial/companies
		// section — then 4-7 digits, e.g. "B258641") is what keeps this
		// pattern from colliding with France's differently-shaped RCS
		// citation (a pure 9-digit SIREN with no letter prefix, e.g. "RCS
		// Paris 123 456 789") — the two countries share the same register
		// name but not the same number shape. Real evidence: menu.lu's real
		// "Mentions légales" page ("RCS Luxembourg n&deg; B258641" — note
		// the literal, un-decoded "n&deg;" ordinal-sign entity, which
		// stripTagsLines never decodes).
		{"RCS", "LU", regexp.MustCompile(`(?i)\bRCS\s*(?:Luxembourg)?\s*(?:n(?:&deg;|°|º|o)?\.?)?\s*:?\s*[A-Z]\s?\d{4,7}\b`)},
		// Sweden — Organisationsnummer, the domestic company-register number
		// (10 digits, hyphenated 6-4) that doubles as the base of the
		// SE-prefixed VAT number above ("SE" + org.nr digits + "01") — the
		// register disclosure Lag (2002:562) om elektronisk handel och andra
		// informationssamhällets tjänster 8 § (Sweden's e-Commerce Directive
		// transposition — "namn... organisationsnummer... i förekommande
		// fall") requires alongside name/address/contact. Distinct
		// Kind name from the existing Norwegian "OrgNr" entry above (same
		// underlying word, "organisationsnummer" vs "organisasjonsnummer",
		// but a different digit shape — Norway's is 9 digits in 3-3-3
		// groups with no hyphen, Sweden's is 10 digits split 6-4 by a
		// hyphen) so singleCountryIdentifierKind (imprint_checksum.go) can
		// attribute it unambiguously to SE. Real evidence:
		// bqredovisning.se's real, live privacy-policy page ("BQ
		// Redovisning &amp; Rådgivning AB (org.nr 559086-2809)").
		{"Organisationsnummer", "SE", regexp.MustCompile(`(?i)\borg(?:anisationsnummer|\.?\s?nr)\.?\s*:?\s*\d{6}-\d{4}\b`)},
		// Denmark — CVR-nr. (Det Centrale Virksomhedsregister, the
		// Central Business Register number), the domestic label-anchored
		// form written WITHOUT the "DK" country prefix the existing VAT/DK
		// pattern above requires — same architecture as Sweden's
		// Organisationsnummer immediately above (an 8-digit body that
		// doubles as the base of the DK-prefixed VAT number: "DK" + this
		// CVR body has no extra check digit at all). The register
		// disclosure Lov om tjenester i informationssamfundet, herunder
		// visse aspekter af elektronisk handel (Denmark's e-Commerce
		// Directive transposition), § 7 requires "hvor det er relevant"
		// (where applicable) alongside name/address/contact — same
		// conditionality as Sweden's 8 §, so no dedicated Danish ruleset is
		// added (see the "no ruleset" note at this Kind's
		// singleCountryIdentifierKind entry). The label separator varies
		// across real pages ("CVR.nr." parenthetical form, "CVR-nr.:"
		// standalone form, "CVR nr." with no punctuation at all) hence the
		// permissive `[.\-\s]?` between "CVR" and "nr". Real evidence:
		// kims.dk's real, live handelsbetingelser page ("Orkla Snacks
		// Danmark A/S (CVR.nr. 15233877)"); the "CVR-nr.:" form was
		// independently observed on webshop.dn.dk's real handelsbetingelser
		// page during the same search ("CVR-nr.: 60804214") — both values
		// pass the DK weighted-mod-11 checksum below.
		{"CVR", "DK", regexp.MustCompile(`(?i)\bCVR[.\-\s]?nr\.?\s*:?\s*\d{8}\b`)},
		// Finland — Y-tunnus (Yritys- ja yhteisötunnus, the Finnish Business
		// ID), the domestic label-anchored form written WITHOUT the "FI"
		// country prefix the existing VAT/FI pattern above requires — same
		// architecture as Denmark's CVR-nr immediately above (an 8-digit body
		// that doubles as the base of the FI-prefixed VAT number: "FI" + this
		// Y-tunnus body with its hyphen removed has no extra check digit at
		// all). Written with a hyphen before the final check digit
		// ("1938183-5"), a different shape from the plain 8-digit VAT/FI
		// form. Real evidence: finnprotec.fi's real, live
		// toimitus-ja-sopimusehdot (delivery-and-contract-terms) page
		// ("Finnprotec Oy (Y-tunnus: 1938183-5)") — the value passes the FI
		// weighted-mod-11 checksum below once the hyphen is stripped.
		{"Y-tunnus", "FI", regexp.MustCompile(`(?i)\bY-tunnus\s*:?\s*\d{7}-\d\b`)},
		// Czechia — IČO (Identifikační číslo osoby, the Czech business ID),
		// an 8-digit domestic identifier with its own published weighted-
		// mod-11 checksum (czICOValid, imprint_checksum.go) — same class of
		// gap as CVR/Y-tunnus/CRO above: had no pattern at all before this,
		// so a real Czech imprint page's own identifier was never matched,
		// which (via extractImprintText's isLegalPage-or-identifiers gate)
		// silently skipped the ENTIRE suffix-anchored scan on any Czech
		// page whose URL doesn't happen to contain an English keyword like
		// "terms"/"legal" — "obchodni-podminky" (Czech for "terms and
		// conditions") does not. "IČ" (no trailing O) is accepted too — an
		// equally common older/short form of the same label. Real evidence:
		// onlineshop.cz's real Obchodní podmínky page writes "IČO: 247 17
		// 509" (3-2-3 grouped); the plain ungrouped 8-digit form is also
		// accepted since IČO is always exactly 8 digits regardless of how a
		// given page chooses to space it.
		{"IČO", "CZ", regexp.MustCompile(`(?i)\bIČO?\s*[:#]?\s*(?:\d{3}\s?\d{2}\s?\d{3}|\d{8})\b`)},
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
