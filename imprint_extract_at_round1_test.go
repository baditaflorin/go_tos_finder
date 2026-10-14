package main

import (
	"strings"
	"testing"
)

// Real-evidence round for Austrian imprints (2026-10-14): 200 live .at domains re-enriched with the
// shipped code showed 45 Austrian GmbHs labelled DE (the bare "GmbH" suffix outranked everything) and 70
// of 105 Austrian-ruleset imprints without an address (addresses were only searched next to a detected
// legal-entity name). Fixtures are verbatim lines from the real pages: ages.at, herold.at, ams.at,
// alpenverein.at, bruckneruni.at.

func atPage(lines ...string) string {
	var b strings.Builder
	b.WriteString("<html><body>")
	for _, l := range lines {
		b.WriteString("<p>" + l + "</p>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func TestAustrianGmbHIsNotLabelledGerman(t *testing.T) {
	cases := []struct {
		host, url, wantAddr string
		body                string
	}{
		{"ages.at", "https://www.ages.at/impressum", "1220 Wien, Spargelfeldstraße 191", atPage(
			"Informationen gemäß § 5 ECG, § 25 Mediengesetz und § 14 UGB",
			"Anbieter dieser Webseite",
			"Österreichische Agentur für Gesundheit und Ernährungssicherheit GmbH, kurz AGES",
			"1220 Wien, Spargelfeldstraße 191",
			"Gesellschafter der AGES")},
		{"herold.at", "https://www.herold.at/impressum/", "Canettistraße 5, A-1100 Wien", atPage(
			"Diensteanbieter:", "HEROLD Business Data GmbH", "Anschrift und Kontakt:", "Canettistraße 5", "A-1100 Wien",
			"Tel.: +43 (0)5 0485-333", "E-Mail: kundenservice@herold.at")},
	}
	for _, c := range cases {
		im := extractImprintFields(c.url, c.body, c.host)
		if im.Country != "AT" || im.Ruleset != RulesetATECGMedienG {
			t.Errorf("%s: country=%q ruleset=%q, want AT / %s", c.host, im.Country, im.Ruleset, RulesetATECGMedienG)
		}
		if im.Address != c.wantAddr {
			t.Errorf("%s: address=%q, want %q", c.host, im.Address, c.wantAddr)
		}
	}
}

func TestGermanGmbHStaysGerman(t *testing.T) {
	body := atPage("Impressum", "Muster Handels GmbH", "Marienplatz 8, 80331 München", "Amtsgericht München, HRB 123456",
		"Geschäftsführer: Erika Mustermann", "E-Mail: info@muster-handel.de")
	im := extractImprintFields("https://muster-handel.de/impressum", body, "muster-handel.de")
	if im.Country != "DE" || im.Ruleset != RulesetDETMG {
		t.Errorf("German GmbH: country=%q ruleset=%q, want DE / %s", im.Country, im.Ruleset, RulesetDETMG)
	}
	// a German company on an .at domain keeps DE when its register and postcode are German
	im = extractImprintFields("https://beispiel.at/impressum", atPage("Impressum", "Beispiel Software GmbH", "Hauptstraße 5, 10115 Berlin", "Amtsgericht Charlottenburg, HRB 987654"), "beispiel.at")
	if im.Country != "DE" {
		t.Errorf("German GmbH on an .at host: country=%q, want DE (HRB + 5-digit PLZ outrank the TLD)", im.Country)
	}
}

func TestAustrianPostalAddressWithoutLegalEntity(t *testing.T) {
	cases := []struct {
		name, host, url, wantAddr, wantName string
		body                                string
	}{
		{"two lines with blank spacers (ams.at)", "ams.at", "https://www.ams.at/organisation/ueber-ams/impressum", "Treustraße 35-43, 1200 Wien", "",
			atPage("Arbeitsmarktservice", "Dienstleistungsunternehmen des öffentlichen Rechts", "Treustraße 35-43", "", "1200 Wien", "Telefon: +43 5 09 04 199")},
		{"labelled Anschrift and Medieninhaber (alpenverein.at)", "alpenverein.at", "https://www.alpenverein.at/portal/impressum/index.php", "Olympiastraße 37, 6020 Innsbruck", "Österreichischer Alpenverein",
			atPage("Impressum und Offenlegung nach §§ 24, 25 Mediengesetz (MedienG) iVm § 5 E-Commerce-Gesetz (ECG)", "Medieninhaber: Österreichischer Alpenverein",
				"Vereinssitz und Anschrift: Olympiastraße 37, 6020 Innsbruck", "Tel: +43 512 59547")},
		{"one line with country (bruckneruni.at)", "bruckneruni.at", "https://www.bruckneruni.ac.at/de/impressum", "Alice-Harnoncourt-Platz 1, 4040 Linz", "",
			atPage("Kontakt", "Anton Bruckner Privatuniversität", "Alice-Harnoncourt-Platz 1, 4040 Linz, Austria", "T +43 732 701000")},
	}
	for _, c := range cases {
		im := extractImprintFields(c.url, c.body, c.host)
		if im.Address != c.wantAddr {
			t.Errorf("%s: address=%q, want %q", c.name, im.Address, c.wantAddr)
		}
		if c.wantName != "" && im.LegalName != c.wantName {
			t.Errorf("%s: legal name=%q, want %q", c.name, im.LegalName, c.wantName)
		}
		if strings.HasSuffix(im.LegalName, ":") {
			t.Errorf("%s: legal name %q is a label", c.name, im.LegalName)
		}
	}
}

func TestATAddressFallbackIgnoresPlainText(t *testing.T) {
	// Austrian page with no postal address at all: the fallback must not invent one from prose.
	im := extractImprintFields("https://beispiel.at/impressum", atPage("Impressum", "Wir freuen uns auf Ihren Besuch im Jahr 2026 in Wien.", "Öffnungszeiten täglich 9 bis 17 Uhr"), "beispiel.at")
	if im.Address != "" {
		t.Errorf("invented address %q from prose", im.Address)
	}
}
