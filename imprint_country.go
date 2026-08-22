package main

import (
	"strings"
	"unicode"
)

// Vendored (lightly trimmed) from go_legal_entity's country.go (v1.10.14).
// normalizeCountry never passes free text through: an unrecognised country
// string ("ESTONIA", mojibake) returns "" rather than leaking verbatim into
// the emitted `country` field — see imprint.go's extractImprintFields for how
// this feeds the eu_baseline/de_tmg/at_ecg_medieng ruleset selection.
//
// The upstream file also has reconcileCountryWithTLD / sharedMultiCountryForm
// / nationalSuffixCountry for cross-source candidate reconciliation
// (go_legal_entity merges candidates found on MANY pages). go_tos_finder
// extracts from a single already-verified imprint page, so that
// multi-source reconciliation layer is not needed here; tldCountryOf is kept
// as a plain signal for the VAT-prefix > suffix > TLD precedence in
// extractImprintFields.

// isoAlpha2 is the set of assignable ISO-3166-1 alpha-2 codes (plus the EU /
// UK exceptional reservations, and "EL" — the VAT-prefix form for Greece).
// Membership of this set is what makes a 2-letter token a *real* country code
// rather than free text that merely happens to be two letters long.
var isoAlpha2 = func() map[string]bool {
	codes := strings.Fields(`
		AD AE AF AG AI AL AM AO AQ AR AS AT AU AW AX AZ
		BA BB BD BE BF BG BH BI BJ BL BM BN BO BQ BR BS BT BV BW BY BZ
		CA CC CD CF CG CH CI CK CL CM CN CO CR CU CV CW CX CY CZ
		DE DJ DK DM DO DZ
		EC EE EG EH ER ES ET
		FI FJ FK FM FO FR
		GA GB GD GE GF GG GH GI GL GM GN GP GQ GR GS GT GU GW GY
		HK HM HN HR HT HU
		ID IE IL IM IN IO IQ IR IS IT
		JE JM JO JP
		KE KG KH KI KM KN KP KR KW KY KZ
		LA LB LC LI LK LR LS LT LU LV LY
		MA MC MD ME MF MG MH MK ML MM MN MO MP MQ MR MS MT MU MV MW MX MY MZ
		NA NC NE NF NG NI NL NO NP NR NU NZ
		OM
		PA PE PF PG PH PK PL PM PN PR PS PT PW PY
		QA
		RE RO RS RU RW
		SA SB SC SD SE SG SH SI SJ SK SL SM SN SO SR SS ST SV SX SY SZ
		TC TD TF TG TH TJ TK TL TM TN TO TR TT TV TW TZ
		UA UG UM US UY UZ
		VA VC VE VG VI VN VU
		WF WS
		YE YT
		ZA ZM ZW
		EU EL
	`)
	m := make(map[string]bool, len(codes))
	for _, c := range codes {
		m[c] = true
	}
	return m
}()

// countryNames maps free-text country names (English plus the most common
// native/endonym spellings seen in EU imprints) to ISO-3166-1 alpha-2. Keys
// are lowercased; the lookup strips diacritics/punctuation first so
// "españa", "ESPANA", "Spain." all hit the "spain" entry.
var countryNames = map[string]string{
	"united states": "US", "united states of america": "US", "usa": "US", "us": "US",
	"united kingdom": "GB", "uk": "GB", "great britain": "GB", "england": "GB",
	"scotland": "GB", "wales": "GB", "northern ireland": "GB",
	"germany": "DE", "deutschland": "DE", "allemagne": "DE",
	"france": "FR", "frankreich": "FR",
	"netherlands": "NL", "the netherlands": "NL", "nederland": "NL", "holland": "NL", "niederlande": "NL",
	"italy": "IT", "italia": "IT", "italien": "IT",
	"spain": "ES", "espana": "ES", "españa": "ES", "spanien": "ES",
	"poland": "PL", "polska": "PL", "polen": "PL",
	"sweden": "SE", "sverige": "SE", "schweden": "SE",
	"romania": "RO", "romania ": "RO", "rumania": "RO", "rumanien": "RO", "românia": "RO",
	"austria": "AT", "osterreich": "AT", "österreich": "AT", "autriche": "AT",
	"estonia": "EE", "eesti": "EE", "estland": "EE",
	"latvia": "LV", "latvija": "LV", "lettland": "LV",
	"lithuania": "LT", "lietuva": "LT", "litauen": "LT",
	"finland": "FI", "suomi": "FI", "finnland": "FI",
	"denmark": "DK", "danmark": "DK", "danemark": "DK", "dänemark": "DK",
	"norway": "NO", "norge": "NO", "norwegen": "NO",
	"belgium": "BE", "belgique": "BE", "belgie": "BE", "belgië": "BE", "belgien": "BE",
	"switzerland": "CH", "schweiz": "CH", "suisse": "CH", "svizzera": "CH", "helvetia": "CH",
	"ireland": "IE", "eire": "IE", "éire": "IE", "irland": "IE",
	"portugal": "PT",
	"greece":   "GR", "hellas": "GR", "ellada": "GR", "griechenland": "GR",
	"czech republic": "CZ", "czechia": "CZ", "cesko": "CZ", "Česko": "CZ",
	"slovakia": "SK", "slovensko": "SK",
	"slovenia": "SI", "slovenija": "SI",
	"hungary": "HU", "magyarorszag": "HU", "ungarn": "HU",
	"bulgaria": "BG", "balgariya": "BG", "bulgarien": "BG",
	"croatia": "HR", "hrvatska": "HR", "kroatien": "HR",
	"luxembourg": "LU", "luxemburg": "LU",
	"cyprus": "CY", "malta": "MT",
	"australia": "AU", "new zealand": "NZ",
	"canada": "CA", "mexico": "MX", "brazil": "BR", "brasil": "BR",
	"argentina": "AR", "chile": "CL", "colombia": "CO", "peru": "PE",
	"india": "IN", "singapore": "SG", "japan": "JP", "nippon": "JP",
	"china": "CN", "south korea": "KR", "korea": "KR", "republic of korea": "KR",
	"vietnam": "VN", "viet nam": "VN", "thailand": "TH", "indonesia": "ID",
	"malaysia": "MY", "philippines": "PH", "hong kong": "HK", "taiwan": "TW",
	"united arab emirates": "AE", "uae": "AE", "saudi arabia": "SA",
	"israel": "IL", "turkey": "TR", "turkiye": "TR", "türkiye": "TR",
	"south africa": "ZA", "egypt": "EG", "nigeria": "NG",
	"russia": "RU", "russian federation": "RU", "ukraine": "UA",
}

// normalizeCountry maps an arbitrary country string to an ISO-3166-1 alpha-2
// code, or returns "" when it cannot. It NEVER passes free text through. The
// procedure is:
//
//  1. strip surrounding whitespace and any non-ASCII bytes (mojibake / native
//     script we don't have an endonym entry for is dropped here);
//  2. an exact 2-letter token that is a real ISO code wins;
//  3. otherwise fold diacritics + drop punctuation and look up the name table;
//  4. anything still unresolved returns "".
func normalizeCountry(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// A clean 2-letter token: accept only if it is a real ISO alpha-2 code.
	if len(s) == 2 && isASCIIAlpha(s) {
		up := strings.ToUpper(s)
		if isoAlpha2[up] {
			return up
		}
		return ""
	}
	key := foldCountryKey(s)
	if key == "" {
		return ""
	}
	if cc, ok := countryNames[key]; ok {
		return cc
	}
	// A folded key that is itself a real 2-letter ISO code ("DE", "us").
	if len(key) == 2 && isoAlpha2[strings.ToUpper(key)] {
		return strings.ToUpper(key)
	}
	return ""
}

// foldCountryKey lowercases, folds common Latin diacritics to ASCII, drops any
// remaining non-ASCII byte, removes punctuation, and collapses whitespace —
// producing the lookup key for countryNames.
func foldCountryKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r == ' ' || r == '\t':
			b.WriteRune(' ')
		case r < 0x80:
			// ASCII punctuation/digits → drop (turns "u.s.a." into "usa").
		default:
			if folded := foldRune(r); folded != 0 {
				b.WriteRune(folded)
			}
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// foldRune maps a small set of accented Latin letters to their ASCII base.
// Returns 0 for runes that should be dropped (non-Latin scripts).
func foldRune(r rune) rune {
	switch r {
	case 'ä', 'à', 'á', 'â', 'ã', 'å', 'ā':
		return 'a'
	case 'é', 'è', 'ê', 'ë', 'ē':
		return 'e'
	case 'í', 'ì', 'î', 'ï':
		return 'i'
	case 'ö', 'ò', 'ó', 'ô', 'õ', 'ø':
		return 'o'
	case 'ü', 'ù', 'ú', 'û':
		return 'u'
	case 'ç', 'ć', 'č':
		return 'c'
	case 'ñ':
		return 'n'
	case 'ß':
		return 's'
	}
	if unicode.Is(unicode.Latin, r) {
		lr := unicode.ToLower(r)
		if lr >= 'a' && lr <= 'z' {
			return lr
		}
	}
	return 0
}

func isASCIIAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return true
}

// ccTLD maps a hostname's effective ccTLD to its ISO-3166-1 alpha-2 country.
// Only ccTLDs with an unambiguous national meaning are listed — gTLDs
// (.com/.net/.org/.io/.dev/…) and multi-country TLDs return "".
var ccTLD = map[string]string{
	"ro": "RO", "de": "DE", "fr": "FR", "it": "IT", "es": "ES", "pl": "PL",
	"nl": "NL", "be": "BE", "at": "AT", "ch": "CH", "se": "SE", "dk": "DK",
	"no": "NO", "fi": "FI", "pt": "PT", "ie": "IE", "cz": "CZ", "sk": "SK",
	"si": "SI", "hu": "HU", "bg": "BG", "hr": "HR", "lu": "LU", "lt": "LT",
	"lv": "LV", "ee": "EE", "gr": "GR", "cy": "CY", "mt": "MT",
	"uk": "GB", "au": "AU", "nz": "NZ", "ca": "CA", "jp": "JP", "br": "BR",
	"vn": "VN", "th": "TH", "id": "ID", "my": "MY", "ph": "PH", "in": "IN",
	"ru": "RU", "ua": "UA", "tr": "TR", "kr": "KR", "cn": "CN", "tw": "TW",
}

// tldCountryOf returns the ISO-2 country implied by a hostname's ccTLD, or ""
// for gTLDs / unknown TLDs. "falconvet.ro" → "RO"; "stripe.com" → "".
func tldCountryOf(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return ""
	}
	return ccTLD[parts[len(parts)-1]]
}
