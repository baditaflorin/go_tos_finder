package main

import (
	"strings"
	"testing"
)

func TestJapaneseSpecifiedCommercialTransactionFields(t *testing.T) {
	body := "<html><body><table><tr><th>運営会社</th><td>株式会社サンプル</td></tr>" +
		"<tr><th>所在地</th><td>〒160-0022 東京都新宿区新宿一丁目 2-3</td></tr>" +
		"<tr><th>代表者氏名</th><td>非対象</td></tr></table></body></html>"
	im := extractImprintFields("https://shop.example.jp/tokushoho", body, "shop.example.jp")
	if im.Country != "JP" {
		t.Fatalf("country = %q, want JP", im.Country)
	}
	if im.LegalName != "株式会社サンプル" {
		t.Errorf("legal_name = %q, want the value from the 運営会社 row", im.LegalName)
	}
	if im.Address != "〒160-0022 東京都新宿区新宿一丁目 2-3" {
		t.Errorf("address = %q, want the value from the 所在地 row", im.Address)
	}
}

func TestJapaneseDisclosureLabelsAreCountryGated(t *testing.T) {
	body := "<html><body><table><tr><th>運営会社</th><td>株式会社サンプル</td></tr>" +
		"<tr><th>所在地</th><td>〒160-0022 東京都新宿区新宿一丁目 2-3</td></tr></table></body></html>"
	im := extractImprintFields("https://shop.example.com/tokushoho", body, "shop.example.com")
	if im.LegalName != "" || im.Address != "" {
		t.Errorf("Japanese labels parsed outside JP: legal_name=%q address=%q", im.LegalName, im.Address)
	}
}

func TestJapaneseCorporateAndInvoiceIdentifiers(t *testing.T) {
	body := `<html><body>
<p>運営会社: 株式会社テスト商事</p>
<p>法人番号: 8700110005901</p>
<p>適格請求書発行事業者登録番号: T0000000000000</p>
</body></html>`
	im := extractImprintFields("https://shop.example.jp/tokushoho", body, "shop.example.jp")
	if im.Country != "JP" {
		t.Errorf("country = %q, want JP from Japanese government identifiers", im.Country)
	}
	if im.Register != "法人番号 8700110005901" {
		t.Errorf("register = %q, want normalized Corporate Number", im.Register)
	}
	if im.VAT != "T0000000000000" {
		t.Errorf("vat = %q, want the T-prefixed invoice issuer number", im.VAT)
	}
	if im.VATValidation != string(formatValid) {
		t.Errorf("vat_validation = %q, want format_valid (invoice T numbers do not all use the corporate checksum)", im.VATValidation)
	}
}

func TestJapaneseCorporateNumberChecksum(t *testing.T) {
	// Official NTA example: base 700110005901 yields 8700110005901.
	valid := findIdentifiers("法人番号: 8700110005901", 5)
	if len(valid) != 1 || valid[0].Kind != "法人番号" || valid[0].Validation != string(checksumValid) {
		t.Fatalf("valid Corporate Number hit = %+v, want one checksum_valid 法人番号", valid)
	}
	invalid := findIdentifiers("法人番号: 9700110005901", 5)
	if len(invalid) != 1 || invalid[0].Validation != string(checksumInvalid) {
		t.Errorf("mutated Corporate Number hit = %+v, want checksum_invalid", invalid)
	}
	fullwidth := findIdentifiers("法人番号：８７００１１０００５９０１", 5)
	if len(fullwidth) != 1 || fullwidth[0].Validation != string(checksumValid) {
		t.Errorf("fullwidth Corporate Number hit = %+v, want checksum_valid", fullwidth)
	}
}

func TestJapaneseInvoiceRegistrationRequiresContextAndShape(t *testing.T) {
	for _, text := range []string{
		"登録番号: Ｔ０００００００００００００",
		"適格請求書発行事業者: T1234567890123",
		"インボイス登録番号: T1234567890123",
	} {
		hits := findIdentifiers(text, 5)
		if len(hits) != 1 || hits[0].Kind != "InvoiceIssuerRegistration" || hits[0].Validation != string(formatValid) {
			t.Errorf("invoice registration hits for %q = %+v, want one format_valid hit", text, hits)
		}
	}
	for _, text := range []string{
		"T1234567890123",        // no Japanese invoice-registration context
		"登録番号: 1234567890123",   // generic 13-digit registration number, no T
		"法人番号: 12345678901234",  // 14 digits must not match a 13-digit shape
		"登録番号: T12345678901234", // same boundary for invoice IDs
	} {
		if hits := findIdentifiers(text, 5); len(hits) != 0 {
			t.Errorf("unexpected Japanese identifier hit for %q: %+v", text, hits)
		}
	}
}

func TestJapaneseIdentifiersCanAttachAcrossLongDisclosureSections(t *testing.T) {
	body := `<html><body>
<p>運営会社: 株式会社テスト商事</p>
<p>` + strings.Repeat("会社概要を確認してください。", 100) + `</p>
<p>法人番号: 8700110005901</p>
<p>適格請求書発行事業者登録番号: T0000000000000</p>
</body></html>`
	im := extractImprintFields("https://shop.example.jp/tokushoho", body, "shop.example.jp")
	if im.Register != "法人番号 8700110005901" {
		t.Errorf("register = %q, want the unique page-level Corporate Number", im.Register)
	}
	if im.VAT != "T0000000000000" || im.VATValidation != string(formatValid) {
		t.Errorf("invoice result = %q / %q, want unique format_valid invoice number", im.VAT, im.VATValidation)
	}
}

func TestJapanesePageLevelFallbackAbstainsOnMultipleNumbers(t *testing.T) {
	body := `<html><body>
<p>運営会社: 株式会社テスト商事</p>
<p>` + strings.Repeat("会社概要を確認してください。", 100) + `</p>
<p>法人番号: 8700110005901</p>
<p>法人番号: 5700110005903</p>
</body></html>`
	im := extractImprintFields("https://shop.example.jp/tokushoho", body, "shop.example.jp")
	if im.Register != "" {
		t.Errorf("register = %q, want empty when multiple distinct Corporate Numbers make page-level attribution ambiguous", im.Register)
	}
}
