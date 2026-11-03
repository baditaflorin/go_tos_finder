package main

import "testing"

// Every case below is a string the extractor really returned for a live site in the 2026-11 audit
// (720 imprint pages, each extracted name judged against the page text), with what the page names.

func TestNormalizeLegalNameCleansRealAuditCases(t *testing.T) {
	for _, tc := range []struct{ in, suffix, want string }{
		{"Website Owner: Manfredo Music SL", "SL", "Manfredo Music SL"},                                                                                      // nachoguitars.com
		{"Denominación social: VOLTERETA FUSIÓN, S.L.", "S.L.", "VOLTERETA FUSIÓN, S.L."},                                                                    // volteretarestaurante.com
		{"Responsable del sitio web: WIKI RENT A CAR, S.L.", "S.L.", "WIKI RENT A CAR, S.L."},                                                                // wikirentacar.es
		{"The exploitation rights belong to: Drone Rescue Systems GmbH", "GmbH", "Drone Rescue Systems GmbH"},                                                // dronerescue.com
		{"Su denominación social es: MOVILIDAPP IBERICA, S.L.", "S.L.", "MOVILIDAPP IBERICA, S.L."},                                                          // appyuntamiento.es
		{"Propiedad de WEB 365 SRL", "SRL", "WEB 365 SRL"},                                                                                                   // esdregiondemurcia.es
		{"Alle Rechte vorbehalten VIER PFOTEN - Stiftung", "Stiftung", "VIER PFOTEN - Stiftung"},                                                             // vier-pfoten.at
		{"TM & ©️2026 Sony Interactive Entertainment Inc.", "Inc.", "Sony Interactive Entertainment Inc."},                                                   // fanatec.com
		{"Alle rettigheter © Framsenteret Drift AS", "AS", "Framsenteret Drift AS"},                                                                          // framsenteret.no
		{"Vlasnik autorskih prava © avaz-roto press d.o.o.", "d.o.o.", "avaz-roto press d.o.o."},                                                             // avaz.ba
		{"Le site Selina est édité par Truchot SARL", "SARL", "Truchot SARL"},                                                                                // selina.press
		{"Ingenieros.es pertenece a PLATAFORMA DE INGENIERIA Y DESARROLLOS TECNOLÓGICOS SL", "SL", "PLATAFORMA DE INGENIERIA Y DESARROLLOS TECNOLÓGICOS SL"}, // ingenieros.es
		{"Está publicado por VEOLIA SPAIN SL", "SL", "VEOLIA SPAIN SL"},                                                                                      // veolia.es
		{"la société française Collective SAS", "SAS", "Collective SAS"},                                                                                     // collective.work
		{"der Selfio SE", "SE", "Selfio SE"},                                                                                                                 // selfio.de
		{"by SmartTube Co., Ltd.", "Co., Ltd.", "SmartTube Co., Ltd."},                                                                                       // smarttube.kr
		{"Aviso legal - Datan S.R.L.", "S.R.L.", "Datan S.R.L."},                                                                                             // datan.com.ar
		{"Zweigniederlassung der Valora Schweiz AG", "AG", "Valora Schweiz AG"},                                                                              // bob.ch
		{"IRTECH GmbH – App-Entwicklung & Software-Architektur", "GmbH", "IRTECH GmbH"},                                                                      // ir-tech.ch
		{"SA CONFORT DECOR au capital de 2 903 000 €", "SA", "SA CONFORT DECOR"},                                                                             // 4-pieds.com
		{"ラッコ株式会社（Rakko, Inc.", "株式会社", "ラッコ株式会社"},                                                                                                           // rakko.inc: the alias was cut at the legal form
		{"Müller und Meier GmbH", "GmbH", "Müller und Meier GmbH"},                                                                                           // unchanged
		{"BQ Redovisning & Rådgivning AB", "AB", "BQ Redovisning & Rådgivning AB"},                                                                           // unchanged
		{"42talents GmbH", "GmbH", "42talents GmbH"},                                                                                                         // digits are part of the name
	} {
		got, _, ok := normalizeLegalName(tc.in, tc.suffix)
		if !ok || got != tc.want {
			t.Errorf("normalizeLegalName(%q) = %q, %v; want %q", tc.in, got, ok, tc.want)
		}
	}
}

func TestNormalizeLegalNameRejectsLabelsAndHeadings(t *testing.T) {
	for _, tc := range []struct{ in, suffix string }{
		{"License Number: DMCC", "DMCC"},                   // threefold.me
		{"Company Links", "Company"},                       // biohithealthcare.co.uk
		{"Private Limited Company by Shares (Ltd)", "Ltd"}, // famous-people.de
		{"Handelsregister: HRA 27182 HB", "HB"},            // alex-das-schiff.de
		{"Propriétaire : SAS", "SAS"},                      // frederic-cassel.com
		{"Group co., ltd", "co., ltd"},                     // bloom-flowers.jp
		{"Company number: 16627865", "Company"},            // coinspice.io
	} {
		if got, _, ok := normalizeLegalName(tc.in, tc.suffix); ok {
			t.Errorf("normalizeLegalName(%q) = %q, true; want rejected", tc.in, got)
		}
	}
}

func TestTrimAtConjunctionKeepsHyphenCompound(t *testing.T) {
	// spectronet.de / muk.de: "Musik- und Kongreßhallen" is one name, not two entities.
	in := "Lübecker Musik- und Kongreßhallen GmbH"
	if got := trimAtConjunction(in, "GmbH"); got != in {
		t.Errorf("trimAtConjunction(%q) = %q, want unchanged", in, got)
	}
	// the real two-entity case this function exists for still cuts.
	if got := trimAtConjunction("YOOX and Meta Platforms Ireland Limited", "Limited"); got != "Meta Platforms Ireland Limited" {
		t.Errorf("trimAtConjunction YOOX/Meta = %q", got)
	}
}

func TestExtractEntityAroundKeepsDottedInitialsAndLeadingDigits(t *testing.T) {
	// radfak.de: "D.A.S." is an abbreviation, its period does not end a sentence.
	if got := extractEntityAround("Impressum\nD.A.S. SoftwareBüro GmbH, Musterstraße 1", "GmbH"); got != "D.A.S. SoftwareBüro GmbH" {
		t.Errorf("dotted initials: got %q", got)
	}
	// 42talents.com / 321go.es / 1global.com: digits glued to letters belong to the name.
	for in, want := range map[string]string{
		"Impressum\n42talents GmbH, Hauptstraße 5":    "42talents GmbH",
		"Impressum\n1GLOBAL Holdings B.V., Amsterdam": "1GLOBAL Holdings B.V.",
	} {
		suffix := "GmbH"
		if want == "1GLOBAL Holdings B.V." {
			suffix = "B.V."
		}
		if got := extractEntityAround(in, suffix); got != want {
			t.Errorf("extractEntityAround(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeCandidatesDropsHostingCreditsAndProviders(t *testing.T) {
	body := "<p>Éditeur du site : Info Région Editions SAS</p>\n<p>Hébergeur : OVH SAS</p>"
	in := []imprintCandidate{
		{Name: "Info Région Editions SAS", Suffix: "SAS", Source: "imprint_text"},
		{Name: "OVH SAS", Suffix: "SAS", Source: "imprint_text"},
	}
	got := normalizeCandidates(in, body, "https://jds.fr/mentions-legales")
	if len(got) != 1 || got[0].Name != "Info Région Editions SAS" {
		t.Fatalf("got %+v, want only the publisher", got)
	}
	// a provider is a normal operator on its own domain.
	own := normalizeCandidates([]imprintCandidate{{Name: "Webflow, Inc.", Suffix: "Inc.", Source: "json_ld"}}, "", "https://webflow.com/legal")
	if len(own) != 1 {
		t.Errorf("Webflow on webflow.com must be kept, got %+v", own)
	}
	other := normalizeCandidates([]imprintCandidate{{Name: "Webflow, Inc.", Suffix: "Inc.", Source: "imprint_text"}}, "", "https://forumeuropeendebioethique.eu/mentions")
	if len(other) != 0 {
		t.Errorf("Webflow on a third-party site must be dropped, got %+v", other)
	}
}

func TestNormalizeCandidatesKeepsIdentifiersOfRejectedNames(t *testing.T) {
	// proxima.ie: "Company Registration No: 613314" is not a name but its register number is the page's.
	in := []imprintCandidate{{Name: "Company Registration No: 613314", Suffix: "Company", Source: "imprint_text", Register: "CRO Company Registration No: 613314"}}
	got := normalizeCandidates(in, "", "https://proxima.ie/company-registered-address/")
	if len(got) != 1 || !got[0].IdentifierOnly {
		t.Fatalf("got %+v, want the candidate kept as identifier-only", got)
	}
	if best := bestImprintCandidate(got); best != nil {
		t.Errorf("an identifier-only candidate must never win, got %+v", best)
	}
}
