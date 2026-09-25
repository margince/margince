// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A decision site's call: the decisions lane first, when it may answer this
// task and site, then the task's own ladder and prompt — one logical call, one
// rail occurrence, one flush. With no lane bound it is CompleteStructured
// exactly, row for row.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// maxDecisionStateBytes bounds the state a decision request carries. A
// self-hosted Jev-wire server (Laya) refuses a state over 50,000 characters, and a byte count at or under this
// is under that in any encoding; a larger state goes to the ladder, whose
// prompt the site already bounds for itself.
const maxDecisionStateBytes = 48_000

// DecisionProbe is the decision half of Decide alone, as the certification
// lane reads it.
type DecisionProbe struct {
	// Asked is whether the lane was called; false when it was refused before
	// the call (Reason says why).
	Asked bool
	// Decided is whether the answer stood: the site would act on it.
	Decided bool
	// Answer is what the lane answered, whenever it answered at all — a
	// below-floor answer included, which is what certification measures.
	Answer decision.Answer
	// Reason is the attempt reason the ladder would have been walked with;
	// empty when Decided.
	Reason      string
	ServedModel string
}

// decisionTry is one decision attempt's outcome.
type decisionTry struct {
	asked   bool
	decided bool
	answer  decision.Answer
	info    RouteInfo
	// reason is the fallback walk's attempt_reason; "" when the lane was not
	// eligible at all.
	reason      string
	servedModel string
}

// Decide serves a decision site: the decisions lane first when it may answer
// this task and site, then the task's own ladder with the LLM request. A lane
// that is unbound, uncertified for the site, barred from a local-only task,
// failing or unsure never costs the caller its answer — the ladder serves as
// CompleteStructured would, and the completion row says why it ran.
func (r *Router) Decide(ctx context.Context, task Task, site string, dreq decision.Request,
	req model.Request, validate Validator, gate DecisionGate,
) (DecideOutcome, RouteInfo, error) {
	if _, ok := taskLadders[task]; !ok {
		return DecideOutcome{}, RouteInfo{}, fmt.Errorf("ai: unknown task %q", task)
	}
	// One snapshot for the decision half and the flush, so a Rebind mid-call
	// cannot split them across two routings. The ladder walk behind it loads
	// its own binding per walk, exactly as CompleteStructured's walks do.
	b := r.binding()
	lc := newLogicalCall()
	defer r.flushDetached(ctx, b, lc)
	try, err := r.decideFirst(ctx, lc, b, task, site, dreq, req, gate)
	if err != nil {
		return DecideOutcome{}, RouteInfo{}, err
	}
	if try.decided {
		return DecideOutcome{Decided: true, Answer: try.answer, ServedModel: try.servedModel}, try.info, nil
	}
	resp, info, err := r.completeStructuredOn(ctx, lc, task, req, validate, try.reason)
	return DecideOutcome{Response: resp, ServedModel: resp.ServedModel}, info, err
}

// decideFirst runs the decision attempt when the lane may be asked at all. It
// leaves to the ladder walk every state that walk already reports: no
// workspace, a budget it cannot read, and a cached answer it will serve and
// meter as a hit. A deferral is returned as the ladder would return it: no
// call, no trace. b is the caller's snapshot of the binding.
func (r *Router) decideFirst(ctx context.Context, lc *logicalCall, b *binding, task Task, site string,
	dreq decision.Request, req model.Request, gate DecisionGate,
) (decisionTry, error) {
	if b.decisions == nil {
		return decisionTry{}, nil
	}
	rawWS, ok := principal.WorkspaceID(ctx)
	if !ok {
		return decisionTry{}, nil
	}
	wsID := ids.From[ids.WorkspaceKind](rawWS)
	// The decision attempt reads the band once for itself; each ladder walk
	// after it reads the band again, as every walk does.
	ladder, _, budgetErr := r.applyBudget(ctx, task, wsID, taskLadders[task])
	switch {
	case errors.Is(budgetErr, ErrBudgetDeferred):
		return decisionTry{}, budgetErr
	case budgetErr != nil:
		return decisionTry{}, nil //nolint:nilerr // the ladder walk reads the budget again and traces this failure
	case r.cachedAnswerServes(b, task, wsID, req, ladder):
		// An entry that expires between this peek and the walk's own read
		// costs one LLM call, never a wrong answer: the walk serves fresh.
		return decisionTry{}, nil
	}
	return r.decisionAttempt(ctx, lc, b, task, site, dreq, gate)
}

// DecideProbe is the decision half of Decide alone, for the certification
// lane: the same refusals, call, trace and meter, and no ladder behind it. A
// state Decide would leave to the ladder — no lane, no workspace, a budget it
// cannot read — is an error here, because there is no ladder to report it.
func (r *Router) DecideProbe(ctx context.Context, task Task, site string, dreq decision.Request,
	gate DecisionGate,
) (DecisionProbe, error) {
	if _, ok := taskLadders[task]; !ok {
		return DecisionProbe{}, fmt.Errorf("ai: unknown task %q", task)
	}
	b := r.binding()
	if b.decisions == nil {
		return DecisionProbe{}, fmt.Errorf("ai: task %s: no decisions lane is bound", task)
	}
	rawWS, ok := principal.WorkspaceID(ctx)
	if !ok {
		return DecisionProbe{}, fmt.Errorf("ai: task %s outside workspace context", task)
	}
	if _, _, err := r.applyBudget(ctx, task, ids.From[ids.WorkspaceKind](rawWS), taskLadders[task]); err != nil {
		return DecisionProbe{}, err
	}
	lc := newLogicalCall()
	defer r.flushDetached(ctx, b, lc)
	try, err := r.decisionAttempt(ctx, lc, b, task, site, dreq, gate)
	if err != nil {
		return DecisionProbe{}, err
	}
	return DecisionProbe{Asked: try.asked, Decided: try.decided, Answer: try.answer, Reason: try.reason, ServedModel: try.servedModel}, nil
}

// cachedAnswerServes peeks the result cache the way serveAttempt reads it,
// over the same servable ladder, without serving: a hit skips the decision
// call and the walk after it serves, traces and meters the hit as it always
// did.
func (r *Router) cachedAnswerServes(b *binding, task Task, wsID ids.WorkspaceID, req model.Request, ladder []Tier) bool {
	if r.cacheOff {
		return false
	}
	key, err := cacheKey(wsID, task, req)
	if err != nil {
		return false
	}
	_, tier, hit := r.cache.get(key, wsID, b.generation)
	return hit && tierOnLadder(servableLadder(b, task, ladder), tier)
}

// decisionAttempt asks the lane when it may answer this task and site, and
// reads the answer through the site's own gate.
func (r *Router) decisionAttempt(ctx context.Context, lc *logicalCall, b *binding, task Task, site string,
	dreq decision.Request, gate DecisionGate,
) (decisionTry, error) {
	if reason := r.decisionRefusal(b, task, site); reason != "" {
		return decisionTry{reason: reason}, nil
	}
	// The state is data a site assembled from mail and pages, bound for a
	// vendor like any prompt, so it passes the same stripper a prompt does —
	// through the recorder a completion attempt uses, so the row says what was
	// removed.
	strips := newStripRecorder(r.stripper)
	state, _, err := strips.Strip(ctx, dreq.State)
	if err != nil {
		r.log.WarnContext(ctx, "ai: decision state could not be stripped; asking the ladder", "task", string(task), "err", err)
		return decisionTry{reason: attemptReasonDecisionError}, nil
	}
	if len(state) > maxDecisionStateBytes {
		return decisionTry{reason: attemptReasonDecisionStateTooLarge}, nil
	}
	dreq.State, dreq.Model = state, b.decisions.meta.model
	return r.callDecider(ctx, lc, b, task, dreq, gate, strips)
}

// decisionRefusal is why the lane may not answer this site, as the attempt
// reason the ladder walk carries; "" when it may. The order matches the route
// preview's: keep local-only data local, then the certification row.
func (r *Router) decisionRefusal(b *binding, task Task, site string) string {
	m := b.decisions.meta
	if decisionSkipFor(&DecisionsConfig{Provider: m.provider, Model: m.model, BaseURL: m.baseURL}, task) == DecisionSkipLocalOnly {
		return attemptReasonDecisionLocalOnly
	}
	if !r.decisionCertified(DecisionCertKey{Task: task, Site: site, Provider: m.provider, Model: m.model}) {
		return attemptReasonDecisionUncertified
	}
	return ""
}

// callDecider makes the one decision call: rail, trace, meter, answer.
func (r *Router) callDecider(ctx context.Context, lc *logicalCall, b *binding, task Task,
	dreq decision.Request, gate DecisionGate, strips *stripRecorder,
) (decisionTry, error) {
	// The rail opens here, past every refusal above, for the reason
	// announceRailStartOnce gives: a trace is always appended after it.
	lc.announceRailStartOnce(ctx, r, task)
	lc.renewRailLease(ctx)
	trace := r.newDecisionTrace(ctx, task, b.decisions.meta)
	start := r.now()
	callCtx, cancel := context.WithTimeout(ctx, r.decisionTimeout)
	resp, callErr := b.decisions.client.Decide(callCtx, dreq)
	cancel()
	var meterErr error
	if callErr == nil {
		meterErr = r.meterDecision(ctx, task, resp)
	}
	// Before finalize, which buffers the row, as on a completion attempt.
	trace.SecretsRemoved, trace.SecretKinds = strips.report()
	r.finalizeDecisionAttempt(ctx, lc, &trace, dreq, resp, errors.Join(callErr, meterErr), start)
	if meterErr != nil {
		return decisionTry{}, meterErr
	}
	try := decisionTry{asked: true, servedModel: trace.ServedModel}
	if callErr != nil {
		r.logDecisionFailure(ctx, task, callErr)
		try.reason = attemptReasonDecisionError
		return try, nil
	}
	answer, verdict := readDecision(dreq, resp, gate)
	try.answer = answer
	switch verdict {
	case DecisionAccepted:
		try.decided = true
		try.info = RouteInfo{Tier: TierDecideLane, Provider: trace.Provider, ModelID: trace.ModelID}
	case DecisionBelowFloor:
		try.reason = attemptReasonDecisionBelowFloor
	default:
		try.reason = attemptReasonDecisionOffEnum
	}
	return try, nil
}

// meterDecision records what an answered decision call spent, failing loudly
// as the ladder does: unmetered spend would hollow out the budget guardrail.
// Only an answered call is metered; a failed one reports no spend.
func (r *Router) meterDecision(ctx context.Context, task Task, resp decision.Response) error {
	if err := r.meter.Record(ctx, Usage{Task: task, Tier: TierDecideLane, TokensIn: resp.InputTokens}); err != nil {
		return fmt.Errorf("ai: decision served but metering failed: %w", errors.Join(errMeteringFailed, err))
	}
	r.spendAgentTokens(ctx, resp.InputTokens)
	return nil
}

// logDecisionFailure logs a failed decision call before the ladder answers.
// A rejected request is a spec bug on this side — the site built a request
// the wire refuses — so it is an error; anything else is the vendor's weather.
func (r *Router) logDecisionFailure(ctx context.Context, task Task, err error) {
	if errors.Is(err, errDecisionRejected) {
		r.log.ErrorContext(ctx, "ai: the decision model rejected the request; asking the ladder", "task", string(task), "err", err)
		return
	}
	r.log.WarnContext(ctx, "ai: decision call failed; asking the ladder", "task", string(task), "err", err)
}

// readDecision reads the one question's answer against the request that asked
// it. A missing answer or a label the question never offered is off the enum
// whatever the gate would say; otherwise the site's gate decides, and a zero
// verdict — a gate that forgot a case — is read as off the enum, never as
// accepted.
func readDecision(dreq decision.Request, resp decision.Response, gate DecisionGate) (decision.Answer, DecisionVerdict) {
	if len(dreq.Questions) != 1 {
		return decision.Answer{}, DecisionOffEnum
	}
	for name, question := range dreq.Questions {
		answer, answered := resp.Answers[name]
		if _, offered := question.Criteria[answer.Choice]; !answered || !offered {
			return answer, DecisionOffEnum
		}
		switch verdict := gate(answer); verdict {
		case DecisionAccepted, DecisionBelowFloor, DecisionOffEnum:
			return answer, verdict
		}
		return answer, DecisionOffEnum
	}
	return decision.Answer{}, DecisionOffEnum
}
