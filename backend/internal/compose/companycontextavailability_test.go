// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/identity"
)

// One writer decides whether the Company settings page exists, and it resolves
// the same rollout the endpoints gate on.
//
// The failure is silent in both directions. A stack advertising a page its own
// endpoints refuse sends the reader to a surface that 404s; one hiding a page
// they would serve loses a feature the operator paid to enable. Neither shows up
// as an error anywhere, and both are what a second writer produces.
//
// Read from SOURCE, because the defect is invisible in the result: both sides
// are booleans, so any two expressions agreeing on the five stages this package
// can produce build an identical Server. What has to hold is that there is ONE
// expression, which is a property of the text.
//
// The corpus is every non-test source in the package, not the one file the
// writer lives in today. A gate naming its own subject's file passes beside a
// rival writer in the file next door — which is the exact scenario this test's
// reason for existing names.
func TestOneWriterDecidesWhetherTheCompanyPageExists(t *testing.T) {
	call := regexp.MustCompile(`(?m)^\s*s\.authHandlers = s\.WithCompanyContextAvailable\((.*)\)$`)

	writers := map[string]string{}
	for name := range composeSourceFiles(t) {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		for _, found := range call.FindAllStringSubmatch(readSource(t, name), -1) {
			writers[name] = found[1]
		}
	}

	const owner = "companycontextrollout.go"
	const want = "companyContextReadEnabled(s.companyContextRollout)"

	if len(writers) != 1 {
		t.Fatalf("%d files set /me's company-context availability (%v), want exactly one. "+
			"Two writers of one question drift, and the drift is a page a client offers that "+
			"the server refuses.", len(writers), writers)
	}
	got, ok := writers[owner]
	if !ok {
		t.Fatalf("/me's company-context availability is set from %v, not from %s — "+
			"either it moved, in which case this gate needs to follow it, or a rival writer "+
			"replaced the owner", writers, owner)
	}
	if got != want {
		t.Errorf("/me's company-context availability is set from %q, but the endpoints gate on "+
			"%q. Two expressions for one question agree until they do not, and nothing fails "+
			"when they stop.", got, want)
	}

	// The endpoints' own side. Without this the comparison above could keep
	// passing against a predicate that no longer gates anything.
	if !regexp.MustCompile(`func companyContextReadEnabled\(`).MatchString(readSource(t, owner)) {
		t.Errorf("companyContextReadEnabled is gone from %s, so the expression checked above no "+
			"longer names the predicate the endpoints use", owner)
	}
}

// A server composed with NO rollout option agrees with itself.
//
// This is the case the earlier wiring got wrong, and it got it wrong invisibly.
// An unset rollout means every stage is on — companyContextReadEnabled says so
// and GetCompanyContextCapabilities re-derives it — while the injected boolean
// was the zero value, false. So /me reported the Company page absent on a server
// whose endpoints served it, and only a server that HAD the option agreed.
//
// Asked of the predicate rather than of a booted Server, because booting one
// needs a pool. What the fix moved is WHERE the value is resolved: reading
// s.companyContextRollout after the option loop means the unset case resolves
// through the same predicate as every other, which is what this pins.
func TestAnUnsetRolloutResolvesThroughTheSamePredicate(t *testing.T) {
	for _, ran := range []bool{false, true} {
		srv := &Server{authHandlers: identity.NewHandlers(&identity.Service{})}
		if ran {
			WithCompanyContextRollout("")(srv, nil)
		}
		srv.publishCompanyContextAvailability()

		advertised := srv.CompanyContextAvailable()
		gated := companyContextReadEnabled(srv.companyContextRollout)
		if advertised != gated {
			t.Errorf("rollout option ran=%v: /me advertises the Company page as available=%v "+
				"while the endpoints admit=%v — the reader is sent to a page that 404s, or a "+
				"page that works is hidden", ran, advertised, gated)
		}
	}
}
