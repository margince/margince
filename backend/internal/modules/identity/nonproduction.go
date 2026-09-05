// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// WithNonProduction injects the deployment posture the composition root
// resolves from runtimeenv.Environment. Without it /me reports production
// (the fail-closed default).
func (h Handlers) WithNonProduction(nonProduction bool) Handlers {
	h.nonProduction = nonProduction
	return h
}

// WithDataResetAvailable injects whether this installation armed the data
// reset (operations.allow_data_reset). It is a SEPARATE fact from the posture,
// because a deployment being non-production is not consent to purge its tenant
// data — that is what the switch is for.
//
// It is the same value the endpoint gates on, so the action a client offers and
// the route it would call cannot disagree. Without it /me reports unavailable,
// the fail-closed default that hides the action rather than risk offering one
// the server will refuse.
func (h Handlers) WithDataResetAvailable(available bool) Handlers {
	h.dataResetAvailable = available
	return h
}

// WithCompanyContextAvailable injects whether the installation's company-context
// rollout has typed reads active — the same predicate the company-context
// endpoints gate on, so a settings page a client offers and the route behind it
// cannot disagree.
//
// Held by: TestOneWriterDecidesWhetherTheCompanyPageExists
// (backend/internal/compose/companycontextavailability_test.go), which scans
// every non-test source in the composition root and fails on a second writer or
// on a different expression. The claim is about the TEXT, not the value: two
// expressions agreeing on today's five rollout stages produce an identical
// server, so nothing but the source can tell them apart.
//
// TestAnUnsetRolloutResolvesThroughTheSamePredicate holds the other half — that
// a server which ran no rollout option agrees with itself. It did not, once: the
// value was written inside the option, so an unset rollout left the endpoints
// serving a page /me reported as absent.
//
// It rides /me rather than a probe of its own so that navigation, the command
// palette, settings home and settings search resolve one cached snapshot. They
// each have to agree about which pages exist, and a screen-specific probe beside
// them is how they came to disagree.
//
// Without it /me reports unavailable, the fail-closed default that hides a page
// that exists rather than offering one that does not.
func (h Handlers) WithCompanyContextAvailable(available bool) Handlers {
	h.companyContextAvailable = available
	return h
}

// CompanyContextAvailable reports what /me will say about the Company settings
// page. Exported so the composition root can assert its own wiring agrees with
// the endpoints it gates, which is a claim about two packages and cannot be made
// inside either one alone.
func (h Handlers) CompanyContextAvailable() bool {
	return h.companyContextAvailable
}
