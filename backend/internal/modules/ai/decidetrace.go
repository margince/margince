// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The decision attempt's ai_call row. It is the completion row's shape — one
// more attempt of the same logical call — with the decision lane's identity,
// its input tokens and no output, and a payload in the decision form.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
)

// newDecisionTrace opens the row for one decision attempt, joined on the same
// ambient ids a completion attempt of the call carries.
func (r *Router) newDecisionTrace(ctx context.Context, task Task, lane routeMeta) Call {
	trace := Call{
		Task: task, Kind: callKindDecision, Tier: TierDecideLane,
		Provider: lane.provider, ModelID: lane.model, CacheOff: r.cacheOff,
	}
	withAmbientIdentity(ctx, &trace)
	return trace
}

// finalizeDecisionAttempt completes trace from the call's outcome and appends
// it to lc. A decision reports no output tokens: the wire bills input alone.
func (r *Router) finalizeDecisionAttempt(ctx context.Context, lc *logicalCall, trace *Call,
	dreq decision.Request, resp decision.Response, callErr error, start time.Time,
) {
	trace.LatencyMS = r.now().Sub(start).Milliseconds()
	trace.ErrorSentinel = classifyError(callErr)
	trace.TokensIn = resp.InputTokens
	trace.ServedModel, trace.ServedIdentitySource = servedIdentity(trace.Provider, trace.ModelID, resp.ServedModel)
	// A report or nothing, as on a completion row: an absent provider means
	// the wire named none, and the configured one is not a substitute.
	trace.ServedProvider = resp.ServedProvider
	// Best-effort, like the completion capture: a working call must not fail
	// on its trace.
	if r.CapturesPayload(trace.Task) && trace.ErrorSentinel == "" {
		if p, err := r.buildDecisionPayload(ctx, dreq, resp); err != nil {
			r.log.WarnContext(ctx, "ai: decision payload capture failed", "err", err)
		} else {
			trace.Payload = p
		}
	}
	lc.append(*trace)
}

// buildDecisionPayload is the decision form of the Layer-3 capture: the state
// (already stripped on its way to the wire) and the questions asked, and the
// answers given. The state is stored as a STRING, capped like any captured
// field, so a truncation cannot leave invalid JSON behind; the whole request
// document then passes the stripper again, as buildPayload's does, because
// the questions are text too.
func (r *Router) buildDecisionPayload(ctx context.Context, dreq decision.Request, resp decision.Response) (*Payload, error) {
	budget := captureBudget{remaining: maxCapturedRequestRunes}
	reqDoc, err := json.Marshal(struct {
		State     string                          `json:"state"`
		Questions map[string]decisionWireQuestion `json:"questions"`
	}{budget.take(string(dreq.State)), decisionToWire(dreq).Questions})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal decision capture request: %w", err)
	}
	stripped, _, err := r.stripper.Strip(ctx, reqDoc)
	if err != nil {
		return nil, fmt.Errorf("ai: strip decision capture request: %w", err)
	}
	answers := make(map[string]decisionWireAnswer, len(resp.Answers))
	for name, a := range resp.Answers {
		answers[name] = decisionWireAnswer{Choice: a.Choice, Confidence: a.Confidence, Probabilities: a.Probabilities}
	}
	respDoc, err := json.Marshal(struct {
		Answers map[string]decisionWireAnswer `json:"answers"`
	}{answers})
	if err != nil {
		return nil, fmt.Errorf("ai: marshal decision capture response: %w", err)
	}
	return &Payload{Request: json.RawMessage(stripped), Response: json.RawMessage(respDoc)}, nil
}
