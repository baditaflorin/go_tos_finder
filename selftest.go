package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/baditaflorin/go-common/selftest"
)

// selftestTarget supplies the base origin used to resolve fixture links.
const selftestTarget = "https://example.com/"

// selftestFixture is a self-contained homepage with a footer that links to a
// real ToS and Privacy page. The classify check runs the in-process discovery
// logic against it (no network) so a regression in linkScan / classifyLink /
// classifyBody is caught even when outbound is flaky.
const selftestFixture = `<!DOCTYPE html><html><head><title>Acme</title></head><body>
<main>welcome</main>
<footer>
  <a href="/legal/privacy">Privacy Policy</a>
  <a href="/legal/terms">Terms of Service</a>
</footer></body></html>`

// newSelftest builds the deploy-gating suite. It exercises the production
// discovery and classification logic against a complete local fixture, so a
// transient DNS/TLS/proxy failure cannot roll back an otherwise healthy image.
func newSelftest(service, version string) *selftest.Suite {
	s := selftest.NewSuite(service, version)
	s.Check("document-discovery", func(context.Context) error { return checkSelftestFixture() })

	return s
}

func checkSelftestFixture() error {
	base, _ := url.Parse(selftestTarget)
	hits := linkScan(selftestFixture, base)
	if _, ok := hits[DocTermsOfService]; !ok {
		return fmt.Errorf("linkScan missed terms_of_service in fixture footer")
	}
	if _, ok := hits[DocPrivacyPolicy]; !ok {
		return fmt.Errorf("linkScan missed privacy_policy in fixture footer")
	}
	// A clearly type-titled body must classify high; a generic body must not.
	vr := classifyBody(`<html><head><title>Terms of Service</title></head><body>`+
		strings.Repeat("By using the service you agree to these terms governed by law. ", 20)+
		`</body></html>`, 200, DocTermsOfService)
	if !vr.IsReal || vr.Confidence != ConfHigh {
		return fmt.Errorf("classifyBody: titled ToS should be high+real, got real=%v conf=%q", vr.IsReal, vr.Confidence)
	}
	const parkStub = `<!DOCTYPE html><html><head><script>window.onload=function(){window.location.href="/lander"}</script></head></html>`
	park := classifyBody(parkStub, 200, DocTermsOfService)
	if park.IsReal {
		return fmt.Errorf("classifyBody: parking stub must be rejected, got IsReal=true")
	}
	return nil
}
