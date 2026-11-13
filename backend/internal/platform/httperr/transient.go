// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"errors"
	"net/http"
)

// transientCodes are the refusals that clear on their own: a rate limit whose
// window elapses, a metered budget that refills. Membership is by code, not by
// status range. A 503 is also the shape of "not bootstrapped yet" or
// "misconfigured", which no retry resolves and an operator must act on. Telling
// an agent to wait for one of those spends its step budget on a settled call.
var transientCodes = map[string]struct{}{
	"rate_limited":               {},
	IdempotencyInProgressCode:    {},
	"incumbent_budget_exhausted": {},
	schemaChangedCode:            {},
	writeContentionCode:          {},
	SourceBusyCode:               {},
	AgentBusyCode:                {},
	agentVolumeUnansweredCode:    {},
	WebsiteFormBusyCode:          {},
}

// WebsiteFormBusyCode means another intake or policy cutover owns the form lock.
const WebsiteFormBusyCode = "website_form_busy"

// SourceBusyCode refuses an import test while another run of the same source
// name is writing. It clears when that write ends.
const SourceBusyCode = "import_source_busy"

// AgentBusyCode refuses an agent's second import test while its first is still
// running. It clears when that test ends.
const AgentBusyCode = "import_agent_busy"

// Transient reports whether repeating the same call unchanged could succeed
// later. Every other classified fault is settled: something must change (the
// arguments, a human's decision, an operator's) before a retry means anything.
func (f Fault) Transient() bool {
	_, ok := transientCodes[f.Code]
	return ok
}

// retryAfterFault is a referenced refusal that knows when the same call can
// succeed, such as an agent allowance that resets at a known moment. Classify
// renders it as the Retry-After header.
type retryAfterFault interface {
	RetryAfterSeconds() int
}

// unansweredFault is a refusal made because a count could not be read. It is
// a short outage and no limit reached, so it answers 503 and a short wait.
type unansweredFault interface {
	error
	UnansweredRetrySeconds() int
}

// agentVolumeUnansweredCode is the code of an agent call refused because its
// count could not be read.
const agentVolumeUnansweredCode = "agent_volume_unavailable"

func unansweredFaultOf(err error) (Fault, bool) {
	unanswered, ok := errors.AsType[unansweredFault](err)
	if !ok {
		return Fault{}, false
	}
	return Fault{
		Status: http.StatusServiceUnavailable, Code: agentVolumeUnansweredCode,
		Detail: unanswered.Error(), RetryAfterSeconds: unanswered.UnansweredRetrySeconds(),
	}, true
}
