package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func evidenceForTest(t *testing.T, pageURL, body string) (Imprint, map[string]interface{}, []map[string]interface{}) {
	t.Helper()
	u, err := url.Parse(pageURL)
	if err != nil {
		t.Fatal(err)
	}
	im := extractImprintFields(pageURL, body, u.Hostname())
	encoded, err := json.Marshal(im)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]interface{}
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	evidence, ok := object["evidence"].(map[string]interface{})
	if !ok {
		t.Fatal("expected additive imprint evidence")
	}
	var observations []map[string]interface{}
	for _, raw := range evidence["observations"].([]interface{}) {
		observations = append(observations, raw.(map[string]interface{}))
	}
	return im, evidence, observations
}

func observationForTest(t *testing.T, observations []map[string]interface{}, field, value, method string) map[string]interface{} {
	t.Helper()
	for _, o := range observations {
		if o["field"] == field && o["value"] == value && o["method"] == method {
			return o
		}
	}
	t.Fatalf("observation missing: field=%s method=%s", field, method)
	return nil
}

func TestImprintEvidenceRetainsStructuredAlternativesAndPaths(t *testing.T) {
	const body = `<script type="application/ld+json">{
	  "@graph":[
	    {"@type":"Organization","legalName":"Société Alpha SARL","name":"Alpha marque",
	     "address":[{"streetAddress":"10 Rue des Fleurs","postalCode":"75001","addressLocality":"Paris","addressCountry":{"name":"France"},"telephone":"PHONE_SECRET"},
	                {"streetAddress":"12 Calle Sol","addressCountry":{"@value":"España"}}, {"streetAddress":"TVA BE 0704742612"}],
	     "email":"EMAIL_SECRET","vatID":"VAT_SECRET","taxID":"TAX_SECRET"},
	    {"@type":"Organization","name":"Provider GmbH","address":{"streetAddress":"2 Other Road","addressCountry":"DE"}},
	    {"@type":"Person","name":"IGNORE_PERSON"}
	  ]}</script><script type="application/ld+json">{"@type":"Organization","name":"Other Kft.","address":{"streetAddress":"VAT: FR123456789"}}</script>`
	_, evidence, obs := evidenceForTest(t, "https://example.test/imprint", body)
	if evidence["url"] != "https://example.test/imprint" {
		t.Fatal("missing source URL")
	}
	checks := []struct {
		field, value, path, object, address string
		script                              float64
	}{
		{"legal_name", "Société Alpha SARL", "/@graph/0/legalName", "/@graph/0", "", 0},
		{"name", "Alpha marque", "/@graph/0/name", "/@graph/0", "", 0},
		{"address", "10 Rue des Fleurs", "/@graph/0/address/0/streetAddress", "/@graph/0", "/@graph/0/address/0", 0},
		{"country", "France", "/@graph/0/address/0/addressCountry/name", "/@graph/0", "/@graph/0/address/0", 0},
		{"address", "12 Calle Sol", "/@graph/0/address/1/streetAddress", "/@graph/0", "/@graph/0/address/1", 0},
		{"country", "España", "/@graph/0/address/1/addressCountry/@value", "/@graph/0", "/@graph/0/address/1", 0},
		{"name", "Provider GmbH", "/@graph/1/name", "/@graph/1", "", 0},
		{"address", "2 Other Road", "/@graph/1/address/streetAddress", "/@graph/1", "/@graph/1/address", 0},
		{"name", "Other Kft.", "/name", "", "", 1},
	}
	for _, c := range checks {
		o := observationForTest(t, obs, c.field, c.value, "json_ld")
		if o["source_path"] != c.path || o["object_path"] != c.object || o["script_index"] != c.script || o["basis"] != "explicit" {
			t.Fatalf("wrong source metadata for %s: %#v", c.path, o)
		}
		if c.address != "" && o["address_path"] != c.address {
			t.Fatalf("wrong address grouping for %s", c.path)
		}
		if c.address == "" {
			if _, ok := o["address_path"]; ok {
				t.Fatalf("unexpected address association for %s", c.path)
			}
		}
		if _, ok := o["role"]; ok {
			t.Fatal("source object must not be promoted to operator")
		}
	}
	if got := observationForTest(t, obs, "country", "España", "json_ld")["normalized_value"]; got != "ES" {
		t.Fatalf("explicit country normalization = %v", got)
	}
	b, _ := json.Marshal(evidence)
	for _, forbidden := range []string{"PHONE_SECRET", "EMAIL_SECRET", "VAT_SECRET", "TAX_SECRET", "FR123456789", "0704742612", "IGNORE_PERSON"} {
		if strings.Contains(string(b), forbidden) {
			t.Fatalf("unsafe or unrelated payload was copied: %s", forbidden)
		}
	}
	if evidence["omitted_sensitive"].(float64) < 1 {
		t.Fatal("unsafe value should be counted")
	}
}

func TestImprintEvidenceRealFixturesAndUnrelatedHost(t *testing.T) {
	im, _, obs := evidenceForTest(t, "https://factuo.be/mentions-legales/", factuoBeFixture)
	if im.LegalName != "Saubeo SRL" {
		t.Fatal("legacy selection changed")
	}
	for _, item := range []struct{ name, method string }{{"Saubeo SRL", "imprint_text"}, {"Amen SASU", "imprint_provider_section"}} {
		o := observationForTest(t, obs, "name", item.name, item.method)
		if o["basis"] != "heuristic" {
			t.Fatal("text candidate must remain heuristic")
		}
		if _, ok := o["object_path"]; ok {
			t.Fatal("text name was assigned an unsupported subject")
		}
		if item.method == "imprint_provider_section" && o["role"] != "hosting_provider" {
			t.Fatal("explicit hosting section role lost")
		}
	}
	providerAddress := observationForTest(t, obs, "address", "12-14, Rond-Point des Champs-Élysées – 75008 Paris – France", "imprint_provider_section")
	if providerAddress["role"] != "hosting_provider" {
		t.Fatal("provider section address role lost")
	}
	im, _, obs = evidenceForTest(t, "https://www.hotelrose.at/impressum_de.html", hotelRoseImpressumFixture)
	if im.Address != "Brenner Straße 30, 6150, Steinach am Brenner, A" {
		t.Fatal("legacy selected address changed")
	}
	observationForTest(t, obs, "address", "Brenner Straße 30", "json_ld")
	visible := false
	for _, o := range obs {
		if o["field"] == "address" && o["method"] == "imprint_label" && strings.Contains(o["value"].(string), "Brennerstraße 30") {
			visible = true
			if _, ok := o["object_path"]; ok {
				t.Fatal("heuristic address linked to JSON-LD object")
			}
		}
	}
	if !visible {
		t.Fatal("visible-text address representation was lost")
	}
}

func TestImprintEvidenceUnknownCountryAndIndependentHCards(t *testing.T) {
	const body = `<script type="application/ld+json">{"@type":"Organization","name":"Unit Co","address":{"addressCountry":"Atlantis"}}</script><div class="vcard"><div class="org">First Company</div></div><div class="vcard"><div class="adr">99 Other Street</div><div class="country-name">Germany</div></div>`
	_, _, obs := evidenceForTest(t, "https://example.test/imprint", body)
	country := observationForTest(t, obs, "country", "Atlantis", "json_ld")
	if _, ok := country["normalized_value"]; ok {
		t.Fatal("unknown country was invented")
	}
	for _, o := range obs {
		if o["method"] == "hcard" {
			if _, ok := o["object_path"]; ok {
				t.Fatal("global hCard fields must not be grouped")
			}
		}
	}
}

func TestImprintEvidenceBoundedAndDeterministic(t *testing.T) {
	var body strings.Builder
	body.WriteString(`<script type="application/ld+json">[`)
	for i := 0; i < 2200; i++ {
		if i > 0 {
			body.WriteByte(',')
		}
		fmt.Fprintf(&body, `{"@type":"Organization","legalName":"Example %d"}`, i)
	}
	body.WriteString(`]</script>`)
	_, first, firstObs := evidenceForTest(t, "https://example.test/imprint", body.String())
	_, second, _ := evidenceForTest(t, "https://example.test/imprint", body.String())
	if first["truncated"] != true || len(firstObs) != 2048 {
		t.Fatalf("missing explicit evidence limit: %d observations", len(firstObs))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("observation order must be deterministic")
	}
	deep := `<script type="application/ld+json">` + strings.Repeat(`{"@graph":`, 40) + `{"@type":"Organization","name":"Deep"}` + strings.Repeat(`}`, 40) + `</script>`
	_, limited, _ := evidenceForTest(t, "https://example.test/imprint", deep)
	if limited["truncated"] != true {
		t.Fatal("depth truncation must be reported")
	}
}

func TestImprintEvidenceEmptyAndMalformed(t *testing.T) {
	im := extractImprintFields("https://example.test/imprint", "", "example.test")
	encoded, _ := json.Marshal(im)
	if strings.Contains(string(encoded), `"evidence"`) {
		t.Fatal("empty body cannot supply evidence")
	}
	_, ev, _ := evidenceForTest(t, "https://example.test/imprint", `<script type="application/ld+json">not json</script>`)
	if ev["unparseable_json_ld"] != float64(1) {
		t.Fatal("malformed script must be reported")
	}
	_, ev, _ = evidenceForTest(t, "https://example.test/imprint", `<script type="application/ld+json">{} {}</script>`)
	if ev["unparseable_json_ld"] != float64(1) {
		t.Fatal("trailing JSON values must not be silently accepted")
	}
}

// Frozen, previously captured first-party markup across several languages. The
// evidence is observational: it does not independently prove operator role.
func TestImprintEvidenceMultilingualFirstPartyFixtures(t *testing.T) {
	cases := []struct{ label, page, body, name string }{
		{"Italian", "https://simanova.it/impressum", simanovaFixture, "SIMANOVA S.R.L."},
		{"Spanish", "https://eurostarshotels.com/aviso-legal", eurostarsFixture, "EUROSTARS HOTEL COMPANY SL"},
		{"French", "https://factuo.be/mentions-legales/", factuoBeFixture, "Saubeo SRL"},
		{"Dutch", "https://simpelbootverhuurutrecht.nl/colofon/", simpelBootverhuurFixture, "Simpel Bootverhuur Utrecht"},
		{"Czech", "https://onlineshop.cz/obchodni-podminky/", onlineshopCzFixture, "SHOP TRADING, s.r.o."},
		{"Hungarian", "https://szatmari-izek.shop.hu/impresszum/", szatmariHuFixture, "Szatmári Ízek Kft."},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			im, _, observations := evidenceForTest(t, c.page, c.body)
			if im.LegalName != c.name {
				t.Fatalf("legacy selected name changed: %q", im.LegalName)
			}
			found := false
			for _, o := range observations {
				if (o["field"] == "name" || o["field"] == "legal_name") && o["value"] == c.name {
					found = true
				}
			}
			if !found {
				t.Fatal("selected name lacks an individual source observation")
			}
		})
	}
}

func TestImprintEvidenceProviderSectionDoesNotAdoptClientsOrHiddenContent(t *testing.T) {
	const body = `<h3>Hébergeur</h3><p>Example SASU<br>Adresse du siège : 1 Rue Example, Paris<br>Client : Unrelated SASU<br>Adresse du client : 2 Client Road<br>Adresse du siège : 3 Client Road</p>
 <h3>Développement</h3><p>Nearby Unrelated SASU<br>Adresse du siège : 4 Other Road</p>
 <h3 hidden>Hébergeur</h3><p>Hidden SASU<br>Adresse du siège : 5 Hidden Road</p>
 <section aria-hidden="true"><h3>Hébergeur</h3><p>Aria SASU<br>Adresse du siège : 6 Aria Road</p></section>
 <h3 style="display: none">Hébergeur</h3><p>Styled SASU<br>Adresse du siège : 7 Styled Road</p>
 <h3>Hébergeur</h3><p>Client : Client SASU<br>Adresse du siège : 8 Client Road</p>
 <h3>Hébergeur</h3><div>Nested SASU</div>
 <p>Hébergeur</p><p>Not Heading SASU</p>`
	_, _, observations := evidenceForTest(t, "https://example.test/mentions-legales", body)
	var provider []map[string]interface{}
	for _, o := range observations {
		if o["method"] == "imprint_provider_section" {
			provider = append(provider, o)
		}
	}
	if len(provider) != 2 {
		t.Fatalf("expected only explicitly labeled provider and its address, got %d", len(provider))
	}
	if provider[0]["value"] != "Example SASU" || provider[1]["value"] != "1 Rue Example, Paris" {
		t.Fatal("unrelated or hidden section entered provider evidence")
	}
	for _, o := range provider {
		if o["role"] != "hosting_provider" || o["basis"] != "heuristic" {
			t.Fatal("provider role or basis lost")
		}
		if _, ok := o["object_path"]; ok {
			t.Fatal("section text must not invent structured subject")
		}
	}
}

func TestImprintEvidenceLabeledAddressLinesStayIndependent(t *testing.T) {
	const body = `<p>Owner: Example GmbH<br><strong>Adresse:</strong><br>Brennerstraße 30<br>6150 Steinach in Tirol, Österreich<br>0043 5272 / 62 21<br>Next 1234 Fake Road</p><p><strong>E-Mail-Adresse:</strong><br>dont-copy@example.test</p><div hidden><p><b>Adresse:</b><br>9 Hidden Straße</p></div>`
	_, evidence, obs := evidenceForTest(t, "https://example.test/impressum", body)
	var lines []string
	for _, o := range obs {
		if o["method"] != "imprint_address_label" {
			continue
		}
		if o["component"] != "address_line" || o["basis"] != "heuristic" {
			t.Fatal("address-line evidence metadata missing")
		}
		if _, ok := o["object_path"]; ok {
			t.Fatal("text address was attached to a subject")
		}
		lines = append(lines, o["value"].(string))
	}
	encoded, _ := json.Marshal(evidence)
	count, counted := evidence["omitted_sensitive"].(float64)
	if strings.Contains(string(encoded), "0043 5272 / 62 21") || !counted || count < 1 {
		t.Fatal("unlabeled numeric contact was not omitted and counted")
	}
	if !reflect.DeepEqual(lines, []string{"Brennerstraße 30", "6150 Steinach in Tirol, Österreich"}) {
		t.Fatalf("unexpected address-label line count: %d", len(lines))
	}
}

func TestImprintEvidenceProviderSectionBoundsAreReported(t *testing.T) {
	body := `<h3>Hébergeur</h3><p>` + strings.Repeat(`<span>x</span>`, 300) + `</p>`
	_, evidence, observations := evidenceForTest(t, "https://example.test/mentions-legales", body)
	if evidence["truncated"] != true {
		t.Fatal("fragmented provider section did not report truncation")
	}
	for _, o := range observations {
		if o["method"] == "imprint_provider_section" {
			t.Fatal("oversized section produced a provider")
		}
	}
	body = `<h3>Hébergeur</h3><p>` + strings.Repeat("x", 5000) + ` SASU</p>`
	_, evidence, _ = evidenceForTest(t, "https://example.test/mentions-legales", body)
	if evidence["truncated"] != true {
		t.Fatal("oversized provider line did not report truncation")
	}
}

func TestImprintEvidenceTracksFinalRedirectURL(t *testing.T) {
	allowLoopback(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			fmt.Fprint(w, `<html><head><title>Example</title></head><body><main>`+strings.Repeat("Site introduction ", 20)+`</main><footer><a href="/original-imprint">Imprint</a></footer></body></html>`)
		case "/original-imprint":
			http.Redirect(w, r, "/resolved-imprint", http.StatusFound)
		case "/resolved-imprint":
			fmt.Fprint(w, `<html><head><title>Imprint</title><script type="application/ld+json">{"@type":"Organization","legalName":"Example GmbH","address":{"streetAddress":"1 Example Straße"}}</script></head><body><h1>Imprint</h1><p>Example GmbH, company information and registered address.</p></body></html>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/?target="+url.QueryEscape(server.URL), nil)
	Handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("handler status %d", rec.Code)
	}
	var response struct {
		Imprint struct {
			URL      string `json:"url"`
			Evidence struct {
				URL        string `json:"url"`
				FetchedURL string `json:"fetched_url"`
			} `json:"evidence"`
		} `json:"imprint"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Imprint.URL != server.URL+"/original-imprint" || response.Imprint.Evidence.URL != server.URL+"/original-imprint" {
		t.Fatal("selected link changed")
	}
	if response.Imprint.Evidence.FetchedURL != server.URL+"/resolved-imprint" {
		t.Fatal("redirected body URL was not preserved")
	}
	_, evidence, _ := evidenceForTest(t, "https://example.test/imprint", `<script type="application/ld+json">{"@type":"Organization","name":"Example GmbH"}</script>`)
	if _, ok := evidence["fetched_url"]; ok {
		t.Fatal("unavailable final URL must not be invented")
	}
}

func TestImprintEvidenceReportsSourceLimitBoundary(t *testing.T) {
	allowLoopback(t)
	for _, size := range []int{verifyBodyBytes - 1, verifyBodyBytes, verifyBodyBytes + 1} {
		t.Run(fmt.Sprintf("bytes_%d", size), func(t *testing.T) {
			const prefix = `<html><head><title>Imprint</title><script type="application/ld+json">{"@type":"Organization","legalName":"Example GmbH"}</script></head><body><h1>Imprint</h1>`
			const suffix = `</body></html>`
			body := prefix + strings.Repeat(" ", size-len(prefix)-len(suffix)) + suffix
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/":
					fmt.Fprint(w, `<html><body><main>`+strings.Repeat("Site introduction ", 20)+`</main><footer><a href="/visible-imprint">Imprint</a></footer></body></html>`)
				case "/visible-imprint":
					// Flush headers first so the response is chunked rather than advertising
					// a Content-Length; exact boundary is intentionally treated as uncertain.
					if f, ok := w.(http.Flusher); ok {
						f.Flush()
					}
					fmt.Fprint(w, body)
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			rec := httptest.NewRecorder()
			Handler(rec, httptest.NewRequest(http.MethodGet, "/?target="+url.QueryEscape(server.URL), nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("handler status %d", rec.Code)
			}
			var response struct {
				Imprint struct {
					Evidence struct {
						SourceLimitReached bool `json:"source_limit_reached"`
					} `json:"evidence"`
				} `json:"imprint"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if got, want := response.Imprint.Evidence.SourceLimitReached, size >= verifyBodyBytes; got != want {
				t.Fatalf("source_limit_reached=%t, want %t", got, want)
			}
		})
	}
}

func TestImprintEvidenceNestedJSONLDObjectsStaySeparate(t *testing.T) {
	const body = `<script type="application/ld+json">{
 "@graph":[
  {"@type":"WebSite","publisher":{"@type":"Organization","name":"Publisher GmbH","address":{"streetAddress":"1 Publisher Straße","addressCountry":"DE"},"parentOrganization":{"@type":"Organization","legalName":"Parent SASU","address":{"streetAddress":"2 Rue Parent","addressCountry":"FR"}}}},
  {"@type":"Organization","name":"Graph Company Ltd"}
 ],
 "schema:publisher/group~primary":{"@type":"Organization","name":"Escaped Organization BV"},
 "review":{"@type":"Review","author":{"@type":"Organization","name":"Reviewer Company Ltd"}},
 "person":{"@type":"Person","name":"SHOULD_NOT_APPEAR"}
 }</script>`
	_, _, obs := evidenceForTest(t, "https://example.test/imprint", body)
	checks := []struct{ field, value, source, object string }{
		{"name", "Publisher GmbH", "/@graph/0/publisher/name", "/@graph/0/publisher"},
		{"address", "1 Publisher Straße", "/@graph/0/publisher/address/streetAddress", "/@graph/0/publisher"},
		{"legal_name", "Parent SASU", "/@graph/0/publisher/parentOrganization/legalName", "/@graph/0/publisher/parentOrganization"},
		{"address", "2 Rue Parent", "/@graph/0/publisher/parentOrganization/address/streetAddress", "/@graph/0/publisher/parentOrganization"},
		{"name", "Graph Company Ltd", "/@graph/1/name", "/@graph/1"},
		{"name", "Escaped Organization BV", "/schema:publisher~1group~0primary/name", "/schema:publisher~1group~0primary"},
		{"name", "Reviewer Company Ltd", "/review/author/name", "/review/author"},
	}
	for _, c := range checks {
		o := observationForTest(t, obs, c.field, c.value, "json_ld")
		if o["source_path"] != c.source || o["object_path"] != c.object {
			t.Fatalf("incorrect nested source path: %s", c.source)
		}
		if _, ok := o["role"]; ok {
			t.Fatal("nested source object was promoted to a role")
		}
	}
	graphCount := 0
	for _, o := range obs {
		if o["method"] == "json_ld" && o["value"] == "Graph Company Ltd" {
			graphCount++
		}
		if o["value"] == "SHOULD_NOT_APPEAR" {
			t.Fatal("unrelated Person was exposed")
		}
	}
	if graphCount != 1 {
		t.Fatalf("@graph object emitted %d times", graphCount)
	}
}

func TestImprintEvidenceNumericPostalCodesAreExactAndScoped(t *testing.T) {
	const body = `<script type="application/ld+json">{"@type":"Organization","name":"Example GmbH","legalName":12345,"telephone":1234567890,"identifier":123456789,"address":[{"postalCode":75001,"streetAddress":456,"addressCountry":123},{"postalCode":"01234"},{"postalCode":1e5},{"postalCode":123456789012345678901234567890}]}</script>`
	_, evidence, obs := evidenceForTest(t, "https://example.test/imprint", body)
	for _, c := range []struct{ value, path string }{{"75001", "/address/0/postalCode"}, {"01234", "/address/1/postalCode"}} {
		o := observationForTest(t, obs, "address", c.value, "json_ld")
		if o["component"] != "postal_code" || o["source_path"] != c.path {
			t.Fatal("postal code source was not retained exactly")
		}
	}
	encoded, _ := json.Marshal(evidence)
	for _, unwanted := range []string{"123456789012345678901234567890", "1234567890", `"value":"12345"`, `"value":"456"`, `"value":"123"`, `"value":"1e5"`} {
		if strings.Contains(string(encoded), unwanted) {
			t.Fatal("numeric non-postal or sensitive value entered evidence")
		}
	}
	if count, ok := evidence["omitted_sensitive"].(float64); !ok || count < 1 {
		t.Fatal("oversized numeric payload should be omitted and counted")
	}
}
