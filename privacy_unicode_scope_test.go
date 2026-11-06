package main

import "testing"

func TestLocalizedPrivacyPhraseBoundaries(t *testing.T) {
	for _, title := range []string{"Politique de confidentialité", "Politique de confidentialite\u0301", "POLITICA DE CONFIDENŢIALITATE A SPECIAL ARAD"} {
		body := "<html><head><title>" + title + "</title></head><body><h1>" + title + "</h1></body></html>"
		got := classifyBody(body, 200, DocPrivacyPolicy)
		if !got.IsReal || got.Confidence != ConfHigh {
			t.Errorf("privacy title %q missed: %+v", title, got)
		}
		dt, _, floor := classifyLinkEvidence("/opaque", title)
		if dt != DocPrivacyPolicy || floor != ConfLow {
			t.Errorf("privacy anchor %q missed: %s/%s", title, dt, floor)
		}
	}
	for _, title := range []string{"Politique de confidentialitéétendue", "Politique de confidentialité2", "préprivacy policy", "privacy policy_extended"} {
		got := classifyBody("<title>"+title+"</title>", 200, DocPrivacyPolicy)
		if got.Confidence == ConfHigh {
			t.Errorf("partial word promoted %q: %+v", title, got)
		}
	}
}

func TestPageTypeOutranksUnrelatedSectionHeading(t *testing.T) {
	// Current Trolli privacy page also has a general-terms section heading;
	// the Blurb terms page links legal notices under a section heading.
	for _, tc := range []struct {
		title, heading string
		actual, wrong  DocType
	}{
		{"Trolli Datenschutzerklärung", "Allgemeine Geschäftsbedingungen", DocPrivacyPolicy, DocTermsOfService},
		{"Conditions générales | Blurb FR", "Mentions légales", DocTermsOfService, DocImprint},
	} {
		body := "<html><head><title>" + tc.title + "</title></head><body><h2>" + tc.heading + "</h2></body></html>"
		if got := classifyBody(body, 200, tc.actual); !got.IsReal || got.Confidence != ConfHigh {
			t.Fatalf("primary document lost: %+v", got)
		}
		if got := classifyBody(body, 200, tc.wrong); got.IsReal {
			t.Errorf("section overrode explicit page type: %+v", got)
		}
	}
	body := `<title>Legal information</title><h2>Privacy Policy</h2>`
	if got := classifyBody(body, 200, DocPrivacyPolicy); !got.IsReal || got.Confidence != ConfMedium {
		t.Fatalf("section-only valid evidence lost: %+v", got)
	}
	combined := `<title>Privacy Policy and Terms of Service</title>`
	for _, want := range []DocType{DocPrivacyPolicy, DocTermsOfService} {
		if got := classifyBody(combined, 200, want); !got.IsReal || got.Confidence != ConfHigh {
			t.Fatalf("combined-title evidence lost: %+v", got)
		}
	}
}
