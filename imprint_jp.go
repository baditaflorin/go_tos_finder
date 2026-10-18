package main

import (
	"html"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Japanese e-commerce disclosure pages often use key/value tables whose
// labels are governed by the Specified Commercial Transactions Act. The
// common extractor relies on Latin legal-form suffixes and cannot recognize
// these label/value rows.
var japaneseAddressEvidenceRE = regexp.MustCompile(`[0-9０-９]|〒|丁目|番地|番|号`)

var japaneseCompanyLabels = []string{
	"運営会社", "販売業者", "販売事業者", "事業者名", "会社名", "商号",
}

var japaneseAddressLabels = []string{"所在地", "住所", "本店所在地"}

var japaneseImprintLabels = []string{
	"運営会社", "販売業者", "販売事業者", "事業者名", "会社名", "商号",
	"所在地", "住所", "本店所在地",
	"代表者氏名", "代表者", "責任者", "電話番号", "メールアドレス", "お問い合わせ",
	"商品代金", "商品代金以外", "支払方法", "お支払い方法", "商品の引渡時期", "返品特約",
}

// normalizeJapaneseDigits converts the fullwidth digits commonly used in
// Japanese legal disclosures to ASCII digits. The invoice registration
// prefix may also be rendered as fullwidth T/t; normalize it to ASCII T.
func normalizeJapaneseDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= '０' && r <= '９':
			b.WriteRune('0' + (r - '０'))
		case r == 'Ｔ' || r == 'ｔ':
			b.WriteByte('T')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func extractJapaneseImprintFields(text string) (name, address string) {
	lines := strings.Split(text, "\n")
	name = japaneseLabeledValue(lines, japaneseCompanyLabels, 1)
	if !validJapaneseCompanyValue(name) {
		name = ""
	}
	address = japaneseLabeledValue(lines, japaneseAddressLabels, 4)
	if !validJapaneseAddressValue(address) {
		address = ""
	}
	return name, address
}

// japaneseImprintCandidate uses the operator/company row already understood
// by the JP disclosure parser as an entity anchor for nearby identifiers.
// This lets the ordinary candidate proximity logic attach 法人番号 and
// invoice-registration evidence even when the company name has no Latin
// legal-form suffix.
func japaneseImprintCandidate(text string) (imprintCandidate, int, bool) {
	lines := strings.Split(text, "\n")
	name := japaneseLabeledValue(lines, japaneseCompanyLabels, 1)
	if !validJapaneseCompanyValue(name) {
		return imprintCandidate{}, 0, false
	}
	offset := strings.Index(text, name)
	if offset < 0 {
		return imprintCandidate{}, 0, false
	}
	address := japaneseLabeledValue(lines, japaneseAddressLabels, 4)
	if !validJapaneseAddressValue(address) {
		address = ""
	}
	return imprintCandidate{
		Name:       name,
		Country:    "JP",
		Confidence: "high",
		Source:     "imprint_label",
		Address:    address,
	}, offset, true
}

func japaneseLabeledValue(lines, labels []string, maxLines int) string {
	for i, raw := range lines {
		line := strings.TrimSpace(html.UnescapeString(raw))
		if line == "" {
			continue
		}
		for _, label := range labels {
			value, matched := japaneseLabelRemainder(line, label)
			if !matched {
				continue
			}
			parts := make([]string, 0, maxLines)
			if value != "" {
				parts = append(parts, value)
			}
			for j := i + 1; j < len(lines) && len(parts) < maxLines; j++ {
				next := strings.TrimSpace(html.UnescapeString(lines[j]))
				if next == "" {
					continue
				}
				if isJapaneseImprintLabel(next) {
					break
				}
				parts = append(parts, next)
			}
			return strings.TrimSpace(strings.Join(parts, " "))
		}
	}
	return ""
}

func japaneseLabelRemainder(line, label string) (string, bool) {
	if !strings.HasPrefix(line, label) {
		return "", false
	}
	if len(line) == len(label) {
		return "", true
	}
	rest := line[len(label):]
	r, size := firstRune(rest)
	if r != ':' && r != '：' && !unicode.IsSpace(r) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimLeft(rest[size:], ":： \t")), true
}

func isJapaneseImprintLabel(line string) bool {
	for _, label := range japaneseImprintLabels {
		if _, ok := japaneseLabelRemainder(line, label); ok {
			return true
		}
	}
	return false
}

func validJapaneseCompanyValue(value string) bool {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) < 2 || len(runes) > 80 || strings.ContainsAny(value, "@|<>/\\") {
		return false
	}
	hasLetter := false
	for _, r := range runes {
		if unicode.IsControl(r) {
			return false
		}
		if unicode.IsLetter(r) {
			hasLetter = true
		}
	}
	return hasLetter
}

func validJapaneseAddressValue(value string) bool {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) < 4 || len(runes) > 240 || strings.ContainsAny(value, "@|<>\\") {
		return false
	}
	return japaneseAddressEvidenceRE.MatchString(value)
}

func firstRune(s string) (rune, int) {
	return utf8.DecodeRuneInString(s)
}
