package main

import "testing"

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
