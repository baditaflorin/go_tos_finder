package main

import (
	"regexp"
	"strings"
	"unicode"
)

// Country refinement from signals other than the legal-form suffix. A suffix is a weak, shared
// signal: "GmbH"/"AG" are German, Austrian and Swiss, "s.r.o." Czech and Slovak, "S.A.S." French and
// Colombian, "Ltd" is spoken in a dozen countries, and "Inc."/"Company" appear everywhere. A 2026-11
// check on 1,025 sites whose operator is known from the company register (21 registries) found the
// suffix guess wrong for most .ch (52 of 60), .sk, .com.co and .com.au sites, and for Japanese pages
// whose English footer says "Inc.". The site's own country-code domain, its Japanese or Korean script
// and its +CC phone numbers say more, so they decide when the suffix could have been used there.

// nationalTLD extends ccTLD with the country-code domains that are used nationally and the
// second-level registrations where the bare ccTLD is also used as a generic domain (.co is Colombia
// only as com.co / net.co / ...).
var nationalTLD = map[string]string{
	"ar": "AR", "pe": "PE", "cl": "CL", "mx": "MX", "ve": "VE", "uy": "UY", "ec": "EC", "py": "PY",
	"bo": "BO", "cr": "CR", "il": "IL", "za": "ZA", "sg": "SG", "hk": "HK", "ae": "AE", "sa": "SA",
	"eg": "EG", "ma": "MA", "tn": "TN", "ng": "NG", "ke": "KE", "pk": "PK", "bd": "BD", "lk": "LK",
	"kz": "KZ", "by": "BY", "rs": "RS", "ba": "BA", "mk": "MK", "al": "AL", "li": "LI", "is": "IS",
}

var secondLevelNational = map[string]string{"co": "CO"}

// hostCountry is the country a hostname's ccTLD implies, "" for generic and borrowed TLDs.
func hostCountry(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return ""
	}
	last := parts[len(parts)-1]
	if cc := ccTLD[last]; cc != "" {
		return cc
	}
	if cc := nationalTLD[last]; cc != "" {
		return cc
	}
	if cc := secondLevelNational[last]; cc != "" && len(parts) >= 3 {
		switch parts[len(parts)-2] {
		case "com", "net", "org", "edu", "gov", "mil":
			return cc
		}
	}
	return ""
}

// nativeForms lists, per normalised legal form, the countries where it is a native form. A form
// that is not listed is treated as specific to the country the suffix table gives.
var nativeForms = map[string][]string{
	"gmbh": {"DE", "AT", "CH", "LI"}, "ag": {"DE", "AT", "CH", "LI"}, "gmbhcokg": {"DE", "AT", "CH"},
	"kg": {"DE", "AT"}, "ev": {"DE", "AT"}, "stiftung": {"DE", "AT", "CH", "LI"}, "verein": {"DE", "AT", "CH"},
	"sa":  {"FR", "CH", "BE", "LU", "ES", "PT", "RO", "GR", "PL", "CO", "PE", "BR", "AR", "CL", "MX", "UY", "EC", "VE"},
	"sas": {"FR", "CO", "IT", "BE"}, "sarl": {"FR", "CH", "LU", "BE", "MA", "TN"}, "sàrl": {"CH", "LU", "FR", "BE"},
	"srl": {"IT", "RO", "BE", "AR", "PE", "CO", "CL", "UY", "ES"}, "sl": {"ES"}, "slu": {"ES"},
	"sro": {"CZ", "SK"}, "spolsro": {"CZ", "SK"}, "as": {"NO", "DK", "SK", "CZ", "EE"}, "ab": {"SE", "FI"},
	"nv": {"NL", "BE", "CW"}, "bv": {"NL", "BE"}, "doo": {"SI", "HR", "RS", "BA", "ME", "MK"},
	"ltd":     {"GB", "IE", "AU", "NZ", "ZA", "HK", "SG", "IN", "CY", "MT", "IL", "CA", "KE", "NG", "PK", "BD", "LK", "TH", "MY", "JP", "KR", "CN", "TW"},
	"limited": {"GB", "IE", "AU", "NZ", "ZA", "HK", "SG", "IN", "CY", "MT", "IL", "CA", "KE", "NG", "PK", "BD", "LK", "TH", "MY", "JP", "KR", "CN", "TW"},
	"ptyltd":  {"AU", "ZA"}, "ltda": {"BR", "PT", "CL", "CO", "PE", "UY", "EC", "VE", "MX"},
	"sasu": {"FR"}, "snc": {"FR", "IT", "ES"}, "sci": {"FR"},
}

// weakForms carry no country information: any national domain beats them.
var weakForms = map[string]bool{
	"inc": true, "incorporated": true, "corp": true, "corporation": true, "co": true, "company": true,
	"llc": true, "lp": true, "llp": true, "pc": true, "plc": false,
}

func formKey(suffix string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(suffix) {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formNativeIn(suffix, cc string) bool {
	key := formKey(suffix)
	if weakForms[key] {
		return true
	}
	for _, c := range nativeForms[key] {
		if c == cc {
			return true
		}
	}
	return false
}

// phoneCodes maps a +CC calling code to a country (shared +1 and +7 are left out).
var phoneCodes = map[string]string{
	"41": "CH", "43": "AT", "49": "DE", "33": "FR", "34": "ES", "39": "IT", "31": "NL", "32": "BE", "352": "LU",
	"48": "PL", "420": "CZ", "421": "SK", "36": "HU", "40": "RO", "359": "BG", "30": "GR", "351": "PT", "46": "SE",
	"47": "NO", "45": "DK", "358": "FI", "353": "IE", "44": "GB", "372": "EE", "371": "LV", "370": "LT",
	"386": "SI", "385": "HR", "381": "RS", "380": "UA", "81": "JP", "82": "KR", "86": "CN", "886": "TW",
	"852": "HK", "65": "SG", "60": "MY", "66": "TH", "84": "VN", "62": "ID", "91": "IN", "61": "AU", "64": "NZ",
	"27": "ZA", "55": "BR", "54": "AR", "56": "CL", "57": "CO", "51": "PE", "52": "MX", "58": "VE", "598": "UY",
	"593": "EC", "90": "TR", "972": "IL", "971": "AE", "966": "SA", "20": "EG", "212": "MA",
}

var phoneRE = regexp.MustCompile(`(?:\+|\b00)\s?\(?(\d{1,3})\)?[\s.\-/(]*\d`)

// phoneCountry returns the country whose +CC calling code appears most on the page, "" without a clear winner.
func phoneCountry(text string) string {
	counts := map[string]int{}
	for _, m := range phoneRE.FindAllStringSubmatch(text, 40) {
		digits := m[1]
		// calling codes are 1-3 digits: try the longest match first
		for l := len(digits); l >= 1; l-- {
			if cc := phoneCodes[digits[:l]]; cc != "" {
				counts[cc]++
				break
			}
		}
	}
	best, bestN, second := "", 0, 0
	for cc, n := range counts {
		if n > bestN {
			best, second, bestN = cc, bestN, n
		} else if n > second {
			second = n
		}
	}
	if bestN > second {
		return best
	}
	return ""
}

// scriptCountry is JP for a page written in Japanese kana and KR for Hangul. It needs enough
// characters that a stray word in a footer cannot decide.
func scriptCountry(text string) string {
	kana, hangul := 0, 0
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.In(r, unicode.Hangul):
			hangul++
		}
	}
	switch {
	case kana >= 30 && kana > hangul:
		return "JP"
	case hangul >= 30:
		return "KR"
	}
	return ""
}

// refineCountry replaces a country that only the legal form suggested when a stronger local signal
// disagrees and the form could equally be native there. identifierCountry is true when a VAT prefix
// or a single-country register number already fixed the country; that is never overridden.
func refineCountry(country, suffix string, identifierCountry bool, hostname, text string) string {
	if identifierCountry {
		return country
	}
	if sc := scriptCountry(text); sc != "" && sc != country {
		// Kana or Hangul text: the page is Japanese or Korean, and an English "Inc."/"Ltd." footer
		// is that company's English name.
		if country == "" || formNativeIn(suffix, sc) || country == "US" || country == "GB" {
			return sc
		}
	}
	evidence := hostCountry(hostname)
	if evidence == "" {
		evidence = phoneCountry(text)
	}
	if evidence == "" || evidence == country {
		return country
	}
	// DE versus AT is decided by refineDEATCountry from the register type and the postcode shape,
	// which outrank the domain (a German GmbH with an HRB number on an .at host stays German).
	if (country == "DE" || country == "AT") && (evidence == "DE" || evidence == "AT") {
		return country
	}
	if country == "" || formNativeIn(suffix, evidence) {
		return evidence
	}
	return country
}
