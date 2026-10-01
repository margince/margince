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
// endpoints gate on, so what /me reports and what those routes serve agree.
//
// It does not decide whether the Company settings page exists: that page opens
// on the settings catalog's grants, and the company-context card on it asks the
// endpoints' own capabilities read and hides itself.
//
// Held by: TestOneWriterReportsTheCompanyContextRollout
// (backend/internal/compose/companycontextavailability_test.go), which scans
// every non-test source in the composition root and fails on a second writer or
// on a different expression. The claim is about the TEXT, not the value: two
// expressions agreeing on today's five rollout stages produce an identical
// server, so nothing but the source can tell them apart.
//
// TestAnUnsetRolloutResolvesThroughTheSamePredicate holds the other half — that
// a server which ran no rollout option agrees with itself. It did not, once: the
// value was written inside the option, so an unset rollout left the endpoints
// serving a surface /me reported as absent.
//
// Without it /me reports unavailable, the fail-closed default that under-reports
// a surface rather than claiming one the endpoints would refuse.
func (h Handlers) WithCompanyContextAvailable(available bool) Handlers {
	h.companyContextAvailable = available
	return h
}

// CompanyContextAvailable reports what /me will say about the company-context
// rollout. Exported so the composition root can assert its own wiring agrees with
// the endpoints it gates, which is a claim about two packages and cannot be made
// inside either one alone.
func (h Handlers) CompanyContextAvailable() bool {
	return h.companyContextAvailable
}

// WithEmbedReindexAvailable injects whether an embeddings model is bound. Its
// one writer is the option that wires the reindex engine, so /me cannot offer
// the reindex surface on an installation whose routes answer 501.
//
// Held by: TestOneWriterDecidesWhetherTheReindexSurfaceExists
// (backend/internal/compose/embedreindexavailability_test.go).
//
// Bound or unbound only: which model is bound stays behind the reindex status
// route's own grant. Without it /me reports unavailable, failing closed.
func (h Handlers) WithEmbedReindexAvailable(available bool) Handlers {
	h.embedReindexAvailable = available
	return h
}

// EmbedReindexAvailable reports what /me will say about the reindex surface,
// exported so the composition root can assert it agrees with the engine it wired.
func (h Handlers) EmbedReindexAvailable() bool {
	return h.embedReindexAvailable
}

// WithListsAvailable injects whether the installation has switched lists on,
// the same value the list routes, the list_id reads and the agent tools are
// gated on. Without it /me reports lists absent, failing closed.
func (h Handlers) WithListsAvailable(available bool) Handlers {
	h.listsAvailable = available
	return h
}

// WithReportingAvailable publishes the rollout state used by the authenticated shell.
func (h Handlers) WithReportingAvailable(available bool) Handlers {
	h.reportingAvailable = available
	return h
}
