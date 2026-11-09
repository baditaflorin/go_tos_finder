package main

import (
	"strings"
	"testing"
)

// The production imprint for ap-dichtungstechnik.com places the house number
// on an Allee line, followed by a postal line and encoded-dash Tel./Fax lines.
// The previous projection lost the street and appended both contact lines.
func TestImprintAddressKeepsAlleeAndExcludesEncodedContactLines(t *testing.T) {
	const page = `<!doctype html><html lang="de"><head><title>Impressum</title></head><body>
<p><strong>Achim Pellen</strong><br>Dichtungstechnik GmbH<br>
Brüsseler Allee 19a<br>41812 Erkelenz</p>
<p>Tel.: +49 &#8211; (0)- 24 31 &#8211; 97 45 835<br>
Fax: +49 &#8211; (0)- 24 31 &#8211; 97 45 839</p>
</body></html>`
	const source = "https://ap-dichtungstechnik.com/impressum/"
	im := extractImprintFields(source, page, "ap-dichtungstechnik.com")
	want := "Brüsseler Allee 19a, 41812 Erkelenz"
	if im.Address != want {
		t.Fatalf("address = %q, want %q", im.Address, want)
	}
	if im.Evidence == nil || im.Evidence.URL != source {
		t.Fatalf("source evidence missing: %+v", im.Evidence)
	}
	found := false
	for _, o := range im.Evidence.Observations {
		if o.Field == "address" && o.Value == want {
			found = true
		}
		if o.Field == "address" && (strings.Contains(o.Value, "Tel.") || strings.Contains(o.Value, "Fax:")) {
			t.Errorf("contact text leaked into address evidence: %q", o.Value)
		}
	}
	if !found {
		t.Error("complete address was not retained in page evidence")
	}
}

func TestImprintAddressSeparatesMultilingualContactLabels(t *testing.T) {
	for _, label := range []string{"Tel.", "Telefon", "Phone", "Téléphone", "Teléfono", "Telefone", "Faks", "Τηλέφωνο", "Телефон", "電話", "电话"} {
		t.Run(label, func(t *testing.T) {
			text := "Example GmbH\n123 Main Street, 10115 Berlin\n" + label + ": +49 &#8211; 30 1234 5678\nFax: +49 &#8211; 30 9876 5432"
			if got := extractAddressNearEntity(text, "Example GmbH"); got != "123 Main Street, 10115 Berlin" {
				t.Errorf("address = %q", got)
			}
		})
	}
	text := "Example GmbH\n123 Main Street, 10115 Berlin, Tel.: +49 &#8211; 30 1234 5678"
	if got := extractAddressNearEntity(text, "Example GmbH"); got != "123 Main Street, 10115 Berlin" {
		t.Errorf("inline contact: address = %q", got)
	}
	text = "Example GmbH\nTelefon: +49 &#8211; 30 1234 5678\nBrüsseler Allee 19a\n41812 Erkelenz"
	if got := extractAddressNearEntity(text, "Example GmbH"); got != "Brüsseler Allee 19a, 41812 Erkelenz" {
		t.Errorf("contact before address: address = %q", got)
	}
}

func TestAlleeStreetMarkerRequiresHouseNumber(t *testing.T) {
	if !looksAddressLine("Brüsseler Allee 19a") {
		t.Error("German Allee street with a house number was missed")
	}
	if looksAddressLine("Unsere Allee ist täglich bis 19 Uhr geöffnet") {
		t.Error("ordinary prose about an Allee was treated as an address")
	}
	if !looksAddressLine("Faxon Street 12") || !looksAddressLine("Telefonstraße 4") {
		t.Error("ordinary street names must remain address candidates")
	}
}

func TestAddressEvidenceRejectsMultilingualContactPayloads(t *testing.T) {
	var b imprintEvidenceBuilder
	for _, value := range []string{"Τηλέφωνο: +30 210 123 4567", "Телефон: +7 495 123 4567", "電話：03-1234-5678", "电话：010-1234-5678"} {
		b.add(ImprintEvidenceObservation{Field: "address", Value: value})
	}
	b.add(ImprintEvidenceObservation{Field: "address", Value: "Telefonstraße 4"})
	if b.evidence.OmittedSensitive != 4 || len(b.evidence.Observations) != 1 || b.evidence.Observations[0].Value != "Telefonstraße 4" {
		t.Errorf("contact evidence filtering = %+v", b.evidence)
	}
}
