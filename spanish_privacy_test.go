package main

import "testing"

func TestSpanishPrivacyTitleVerification(t *testing.T) {
	// Current first-party evidence: Telefónica's policy returned HTTP 200 at
	// https://www.telefonica.com/es/politica-de-privacidad/ on 2026-11-06,
	// with <title>Política de privacidad - Telefónica</title>.
	for _, title := range []string{
		"Política de privacidad - Telefónica",
		"Politica de privacidad",
		"Aviso de privacidad",
		"AVISO DE PRIVACIDAD INTEGRAL",
	} {
		for _, tag := range []string{"title", "h1"} {
			t.Run(title+"/"+tag, func(t *testing.T) {
				body := "<html><head><title>Información</title></head><body><h1>" + title + "</h1><p>Datos personales.</p></body></html>"
				if tag == "title" {
					body = "<html><head><title>" + title + "</title></head><body><p>Datos personales.</p></body></html>"
				}
				got := classifyBody(body, 200, DocPrivacyPolicy)
				if !got.IsReal || got.Confidence != ConfHigh {
					t.Fatalf("privacy verification = %+v, want real/high", got)
				}
			})
		}
	}
}

func TestSpanishPrivacyTitleRejectsWrongDocumentType(t *testing.T) {
	body := `<html><head><title>Política de privacidad</title></head><body><p>Datos personales.</p></body></html>`
	got := classifyBody(body, 200, DocTermsOfService)
	if got.IsReal || got.Confidence != ConfNone {
		t.Fatalf("privacy policy verified as terms = %+v", got)
	}
}

func TestSpanishPrivacyTitleBoundaries(t *testing.T) {
	for _, title := range []string{
		"Política de privacid",
		"Aviso de privacid",
		"Política de privacidadextendida",
		"Aviso de privacidadextendida",
		"Información general",
	} {
		body := "<html><head><title>" + title + "</title></head><body><p>Información general.</p></body></html>"
		got := classifyBody(body, 200, DocPrivacyPolicy)
		if got.Confidence == ConfHigh {
			t.Errorf("unrelated/truncated title %q verified as high", title)
		}
	}
	if got := classifyBody(`<html><title>Política de privacidad</title></html>`, 404, DocPrivacyPolicy); got.IsReal {
		t.Fatal("HTTP 404 policy template was accepted")
	}
}
