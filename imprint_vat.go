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
		// Optional space: real evidence, sgermobil.si's real Splošni pogoji
		// page writes its own domestic "Davčna številka" (tax number) as
		// "SI 11597267" — space-separated, unlike the bare-adjacent form
		// this pattern originally required. Same class of fix as the
		// BE/FR space-tolerance fixes above (round 7).
		{"VAT", "SI", regexp.MustCompile(`\bSI\s?\d{8}\b`)},
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
		// Hungary — Adószám (the domestic tax number), 11 digits written
		// "########-#-##" (8-digit core + 1-digit VAT-status code + 2-digit
		// county code). Treated as VAT-equivalent (see isVATLikeIdentifierKind
		// and validateIdentifier below) since the 8-digit core IS the same
		// number the "HU"-prefixed EU VAT pattern above matches, just
		// written domestically without the country prefix — same
		// architecture as Poland's NIP/Portugal's NIPC. Real evidence:
		// szatmari-izek.shop.hu's real Impresszum page writes "Adószám:
		// 13495413-2-15"; huAdoszamValid (imprint_checksum.go) confirmed
		// against this value AND a second real value on the same page
		// (23495919-2-41, a different company mentioned further down).
		{"Adószám", "HU", regexp.MustCompile(`(?i)\bAdószám\s*[:#]?\s*\d{8}-\d-\d{2}\b`)},
		// Hungary — Cégjegyzékszám (the company-register number), written
		// as county(2)-court(2)-serial(6) digits, space- or dash-separated
		// (both forms confirmed real on the same szatmari-izek.shop.hu
		// page: "15 09 069897" for the page's own entity, "01-09-968314"
		// for a different company mentioned further down) — same class of
		// register-only identifier as Czechia's IČO/Luxembourg's RCS above.
		{"Cégjegyzékszám", "HU", regexp.MustCompile(`(?i)\bCégjegyzékszám\s*[:#]?\s*\d{2}[\s-]?\d{2}[\s-]?\d{6}\b`)},
		// Greece — ΑΦΜ (Αριθμός Φορολογικού Μητρώου, the domestic tax
		// registry number), 9 digits with no separators. Treated as
		// VAT-equivalent (see isVATLikeIdentifierKind and validateIdentifier
		// below) since it IS the same 9-digit body the "EL"-prefixed EU VAT
		// pattern above matches, just written domestically without the
		// country prefix — same architecture as Hungary's Adószám. Real
		// evidence: thikishop.gr's real Όροι Χρήσης page writes "ΑΦΜ:
		// 800617296"; grVATValid (imprint_checksum.go) confirmed against
		// this real value.
		//
		// Deliberately NO leading `\b` here (unlike every ASCII-anchored
		// pattern elsewhere in this table): Go's RE2 `\b`/`\w` are
		// ASCII-only — they do NOT recognise Greek (or any non-ASCII)
		// letters as "word" characters at all, so `\bΑΦΜ` can never match
		// ANYTHING (there's no ASCII word/non-word transition immediately
		// before a Greek letter, ever). Every other non-ASCII-labelled
		// pattern in this table (IČO, Adószám, Cégjegyzékszám) happens to
		// start/end on a plain ASCII character, so this RE2 gotcha never
		// surfaced before this round. The trailing `\b` after `\d{9}` is
		// still safe — digits ARE in RE2's ASCII \w class. Real-world
		// false-positive risk from dropping the leading boundary is low:
		// "αφμ" is an unusual consonant run in Greek and, even if it did
		// appear inside an unrelated word, would ALSO need to be
		// immediately followed by exactly 9 digits to match at all.
		{"ΑΦΜ", "EL", regexp.MustCompile(`(?i)ΑΦΜ\s*[:#]?\s*\d{9}\b`)},
		// Greece — Γ.Ε.ΜΗ. (Γενικό Εμπορικό Μητρώο, the General Commercial
		// Registry number), a variable-length digit run (commonly 12
		// digits) optionally preceded by "Αρ." (Greek "No."). Register-only
		// — no dedicated checksum algorithm confirmed this round (same
		// discipline as Ireland's CRO/Luxembourg's RCS: format-matching +
		// Register wiring, no invented checksum). Real evidence:
		// thikishop.gr's real page writes "Αρ. Γ.Ε.ΜΗ.: 132389607000".
		//
		// Same RE2 fix as ΑΦΜ just above: no leading `\b` (Greek "Α"/"Γ"
		// would silently never match one). This page's own real markup is
		// "<strong>Αρ. Γ.Ε.ΜΗ.:</strong> 132389607000" — the tag boundary
		// between "</strong>" and the value inserts a stripTagsLines
		// newline right before the digits, which the existing `\s*` right
		// after the label already bridges fine (a bare newline, not the
		// literal-un-decoded "&nbsp;" some other real pages in this
		// codebase have used at the same spot — checked byte-for-byte
		// against the actual fetched page here, not assumed).
		{"Γ.Ε.ΜΗ.", "GR", regexp.MustCompile(`(?i)(?:Αρ\.\s*)?Γ\.?\s?Ε\.?\s?ΜΗ\.?\s*[:#]?\s*\d{6,15}\b`)},
		// Bulgaria — ЕИК (Единен идентификационен код, the Unified
		// Identification Code / BULSTAT), 9 digits with no separators.
		// Treated as VAT-equivalent (see isVATLikeIdentifierKind and
		// validateIdentifier below) since it IS the same 9-digit body the
		// "BG"-prefixed EU VAT pattern above matches (BG201845795 = "BG" +
		// this page's own ЕИК 201845795), same architecture as Hungary's
		// Adószám/Greece's ΑΦΜ. Real evidence: cressi.bg's real Общи
		// условия page writes "ЕИК 201845795"; bgEIKValid
		// (imprint_checksum.go) confirmed against this real value.
		//
		// Same RE2 fix as round 18's Greek patterns: no leading `\b` — RE2
		// never recognises a Cyrillic letter as a "word" character either,
		// so `\bЕИК` would silently never match anything, for the exact
		// same reason `\bΑΦΜ` never matched Greek text.
		{"ЕИК", "BG", regexp.MustCompile(`(?i)ЕИК\s*[:#]?\s*\d{9}\b`)},
		// Croatia — OIB (Osobni identifikacijski broj, the personal/legal
		// identification number used for both individuals and companies),
		// 11 digits with no separators. Treated as VAT-equivalent (see
		// isVATLikeIdentifierKind and validateIdentifier below) since it IS
		// the same 11-digit body the "HR"-prefixed EU VAT pattern above
		// matches, same architecture as Bulgaria's ЕИК/Greece's ΑΦΜ. Real
		// evidence: mako.hr's real Opći uvjeti page writes "OIB:
		// 31448356613"; hrOIBValid (imprint_checksum.go) confirmed against
		// this real value via the published ISO 7064 MOD 11-10 algorithm.
		{"OIB", "HR", regexp.MustCompile(`\bOIB\s*[:#]?\s*\d{11}\b`)},
		// Croatia — MBS (Matični broj subjekta, the court-register entry
		// number), a variable-length digit run. Register-only — no
		// dedicated checksum algorithm confirmed this round (same
		// discipline as Ireland's CRO/Luxembourg's RCS/Greece's Γ.Ε.ΜΗ.:
		// format-matching + Register wiring, no invented checksum). Real
		// evidence: mako.hr's real page writes "MBS: 030013817".
		{"MBS", "HR", regexp.MustCompile(`\bMBS\s*[:#]?\s*\d{5,15}\b`)},
		// Slovenia — Matična številka (the company registration number),
		// typically 10 digits. Register-only — no dedicated checksum
		// algorithm confirmed this round (same discipline as Ireland's
		// CRO/Croatia's MBS: format-matching + Register wiring, no
		// invented checksum). Real evidence: sgermobil.si's real Splošni
		// pogoji page writes "Matična številka: 2153254000". Slovenia's
		// own tax number ("Davčna številka") needs no separate pattern
		// here — its real form ("SI 11597267") is already the same shape
		// the "SI"-prefixed EU VAT pattern above matches (with the space
		// tolerance also fixed this round), so it's picked up as a plain
		// "VAT" Kind hit without a dedicated domestic pattern.
		{"Matična številka", "SI", regexp.MustCompile(`\bMatična\s+številka\s*[:#]?\s*\d{7,12}\b`)},
		// Cyprus — "HE" (the Registrar of Companies' own file-number
		// prefix, e.g. "HE 141156"). Register-only — no dedicated checksum
		// algorithm confirmed this round (same discipline as Ireland's
		// CRO/Croatia's MBS: format-matching + Register wiring, no
		// invented checksum). Case-SENSITIVE deliberately (no `(?i)`):
		// lowercase "he" is an ordinary, extremely common English word/
		// pronoun, so folding case here would risk matching unrelated
		// prose; the real label is always capitalised. Real evidence:
		// epic.com.cy's real eStore Terms & Conditions page writes
		// "registration number HE 141156".
		{"HE", "CY", regexp.MustCompile(`\bHE\s?\d{4,7}\b`)},
		// Cyprus — the domestic VAT number, written in real Cypriot
		// English-language legal text WITHOUT the "CY" country prefix the
		// EU VAT pattern above requires (unlike most other domestic forms
		// in this table, which are anchored by a native-language label,
		// this one is anchored by the English phrase "VAT ... number"
		// since Cyprus's business-facing legal text is very commonly
		// English, a legacy of its common-law tradition). Kind is
		// deliberately "VAT Number", NOT the bare "VAT" Kind other
		// entries use: this match necessarily swallows the anchoring
		// label text too (RE2 has no lookbehind to strip it before
		// matching), and reusing "VAT" here would corrupt
		// cleanIdentifierValue's generic onlyAlnum() cleanup for every
		// OTHER country's already-clean "VAT" hits — so this Kind gets
		// its own dedicated cleaner (imprint_checksum.go) that trims to
		// just the trailing 9-character code, the same trailing-slice
		// pattern CIF/PartitaIVA already use for their own labelled
		// matches. The 8-digit+letter shape (with no leading country
		// code) is itself distinctively Cypriot among this table's VAT
		// formats — no other member state's VAT is this exact shape — so
		// requiring "VAT ... number" nearby is enough of a gate without
		// needing a Cyprus-specific word in the label itself. Real
		// evidence: epic.com.cy's real page writes "VAT registration
		// number 10141156Y".
		{"VAT Number", "CY", regexp.MustCompile(`(?i)\bVAT\s+(?:registration\s+)?number\s*[:#]?\s*\d{8}[A-Z]\b`)},
		// Lithuania — Įmonės kodas (the legal-entity code), 9 digits.
		// Register-only, NOT VAT-equivalent: unlike Bulgaria's ЕИК/
		// Greece's ΑΦΜ/Croatia's OIB, this is a genuinely SEPARATE number
		// from Lithuania's own PVM kodas (VAT code) — real evidence,
		// grupinispirkimas.lt's real page has Įmonės kodas 302983374
		// alongside a completely different PVM kodas LT100007453916 (the
		// existing "LT"-prefixed EU VAT pattern above already matches the
		// PVM kodas form on its own, no new pattern needed for it).
		// ltCompanyCodeValid (imprint_checksum.go) confirmed against this
		// value AND a second real value independently found the same
		// day (302662379, bigbox.lt).
		//
		// Deliberately NO leading `\b` (same RE2 gotcha rounds 18/19
		// found for Greek/Cyrillic — it's not script-specific: "Į" is
		// Latin Extended-A, U+012E, and RE2's `\b` is ASCII-only
		// regardless of Unicode block). Verified directly this round
		// rather than assumed: a leading `\b` here matches nothing at
		// all, confirmed before shipping.
		//
		// Optional literal "&nbsp;" tolerated between the label's colon
		// and the digits: this real page's own markup is "Įmonės
		// kodas:&nbsp;<i>302983374 </i>" — the tag boundary right after
		// "&nbsp;" inserts a stripTagsLines newline, which `\s*` alone
		// bridges fine, but the literal 6-character "&nbsp;" TEXT itself
		// (deliberately never decoded — see trimLeadingTrailingNBSP's doc
		// comment) sits between the colon and that newline and is not
		// whitespace, so `\s*` alone couldn't bridge it.
		{"Įmonės kodas", "LT", regexp.MustCompile(`(?i)Įmonės\s+kodas\s*[:#]?\s*(?:&nbsp;)?\s*\d{9}\b`)},
		// Latvia — Reģistrācijas numurs (the registration number), 11
		// digits. Treated as VAT-equivalent (see isVATLikeIdentifierKind
		// and validateIdentifier below), UNLIKE Lithuania's Įmonės kodas
		// just above: real evidence, gatavosana.lv's real Juridiskā
		// informācija page has "Reģistrācijas numurs: 40103719642" right
		// alongside "PVN maksātāja numurs: LV40103719642" — the SAME
		// 11-digit body, just with the "LV" VAT-country prefix added, same
		// architecture as Bulgaria's ЕИК/Greece's ΑΦΜ/Croatia's OIB. No
		// checksum implemented (stays format_valid — this round lacked
		// confidence in the published Latvian algorithm to verify safely
		// against the single real value available; several plausible
		// weight sequences from general knowledge were tried by hand
		// against 40103719642 and none matched, so nothing was guessed).
		// Both boundary characters here are plain ASCII ("R"/digit), so
		// (unlike Lithuania's "Į"-leading pattern) the leading `\b` is
		// safe — verified directly, not assumed.
		{"Reģistrācijas numurs", "LV", regexp.MustCompile(`(?i)\bReģistrācijas\s+numurs\s*[:#]?\s*\d{11}\b`)},
		// Estonia — registrikood (the company registry code), 8 digits.
		// Register-only, NOT VAT-equivalent: this is a genuinely SEPARATE
		// number from Estonia's own KMKR nr (VAT number) — real evidence,
		// voluaed.ee's real Müügitingimused page has "registrikood
		// 12178134" (8 digits) alongside a completely different "KMKR nr:
		// EE101490959" (a different 9-digit body under the existing
		// "EE"-prefixed EU VAT pattern above, no new pattern needed for
		// it) — same shape as Lithuania's Įmonės kodas/Latvia's PVN split,
		// not Bulgaria/Greece/Croatia's shared-body architecture. No
		// dedicated checksum algorithm confirmed this round (same
		// discipline as Ireland's CRO/Croatia's MBS: format-matching +
		// Register wiring, no invented checksum). Both boundary
		// characters are plain ASCII, so no RE2 `\b` workaround is needed
		// — verified directly.
		{"registrikood", "EE", regexp.MustCompile(`(?i)\bregistrikood\s*[:#]?\s*\d{8}\b`)},
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
			// m is kept as the RAW match (byte-identical to `text`)
			// deliberately — extractImprintText's proximity-attachment
			// step (imprint_jsonld.go) does `strings.Index(text, id.Value)`
			// to find where this identifier sits on the page, which
			// requires an exact substring match against the ORIGINAL text.
			// Any embedded whitespace/newline is cleaned up later, only at
			// display time (formatRegister) — see its doc comment.
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
