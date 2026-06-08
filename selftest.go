package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/baditaflorin/go-common/selftest"
)

// selftestTarget is a stable, always-up site used by the live outbound check.
// example.com is a IANA-maintained reference host that responds with a small
// static HTML page — ideal for proving DNS + TLS + the fetch path work without
// hammering a real customer site.
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

// newSelftest builds the /selftest suite. It returns 200 when the service can
// genuinely do its job (outbound fetch path healthy + discovery logic intact)
// and 503 when its own dependency (the fleet fetch path) is broken — gating the
// deploy smoke step.
func newSelftest(service, version string) *selftest.Suite {
	s := selftest.NewSuite(service, version)

	// classify: pure in-process check of the discovery pipeline. No network.
	s.Check("classify-pipeline", func(ctx context.Context) error {
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
	})

	// outbound: live fetch of a known-good target through the real client. This
	// validates DNS + TLS + the fleet fetch path. Failure ⇒ 503 (deps broken).
	s.Check("outbound-fetch", func(ctx context.Context) error {
		client := newClient()
		_, _, err := fetchPageRetry(ctx, client, selftestTarget)
		if err != nil {
			return fmt.Errorf("homepage fetch of %s failed: %w", selftestTarget, err)
		}
		return nil
	})

	return s
}
