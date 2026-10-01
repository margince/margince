// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Structured-output enforcement (B-EP06.25, ai-operational-spec §5.1/2):
// parse → validate → retry once with the validator's error appended →
// escalate one tier → return the HONEST degraded state. The model never
// gates itself: validation is code, retries are policy, and a final
// failure is an error the caller surfaces — never a partial fabrication.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ErrOutputRejected is the §5.2 policy exhausted: three attempts, the last one
// escalated, and the validator still refused the text.
//
// It is TERMINAL for this input. A caller that retries the same request buys
// the same three calls and the same refusal, so the distinction matters to
// anything with a watermark or a queue: a provider outage is owed a retry, a
// reply this site will not act on is owed a decision.
var ErrOutputRejected = errors.New("ai: model output rejected by the validator")

// ModelDeclined reports whether a call ended on a verdict about this input
// rather than an outage: the validator refused every answer the models gave
// (ErrOutputRejected), or the walk ended on a withheld answer
// (model.ErrOutputWithheld). Both are terminal in the same way — asked again,
// the same words meet the same refusal — so a site that decides "retry or give
// up" asks this rather than either sentinel alone.
func ModelDeclined(err error) bool {
	return errors.Is(err, ErrOutputRejected) || errors.Is(err, model.ErrOutputWithheld)
}

// ErrUnconfiguredModel marks a rejection whose text came from the offline fake
// provider — the stand-in an installation runs on until an operator binds a
// real vendor. The fake answers every request with a hash string, so it fails
// the validator on the first attempt and on every retry forever after.
//
// It exists because the two rejections need opposite words. A real model that
// answered badly is worth another attempt and might be the corpus's fault; the
// fake is neither, and telling its operator that "the model returned something
// we could not read" sent one to audit their own writing samples while the
// answer was a two-field settings screen. Carried alongside ErrOutputRejected
// rather than instead of it, so every caller that classifies a terminal
// rejection keeps classifying this one the same way.
var ErrUnconfiguredModel = errors.New("ai: no AI provider is configured, so the offline stand-in model answered")

// Validator judges one completion's text; the returned error is fed
// back verbatim on the retry so the model sees WHY it failed.
type Validator func(text string) error

// CompleteStructured runs the §5.2 retry policy:
//
//	attempt 1: default route
//	attempt 2: same route, validator error appended
//	attempt 3: one tier escalated (ladder offset), error appended
//
// Every attempt is metered like any call — retries count against the
// workspace budget by construction. All three attempts are ONE logical
// call (spec §4): they share a LogicalCallID and are flushed together, so
// a certification read sees the retry/escalation chain as the single
// served-or-failed decision it is, not three unrelated ai_call rows.
// maxLadderWalks is how many times CompleteStructured can walk the ladder for
// ONE logical call: the first try, the schema-invalid retry, and the escalation.
//
// It is a constant here rather than a comment because RouteWriteDeadline depends
// on it — a handler that calls a model gets one write deadline for the whole
// logical call, and it is set once on the connection where nothing can extend
// it. A fourth walk added below without changing this would cut a healthy call
// mid-response, which is why
// TestStructuredWalksTheLadderNoMoreThanTheRouteDeadlineAssumes counts the walks
// rather than trusting this number. The rail's lease does NOT depend on it: that
// lease covers one model call and is renewed before each further one.
const maxLadderWalks = 3

func (r *Router) CompleteStructured(ctx context.Context, task Task, req model.Request, validate Validator) (model.Response, RouteInfo, error) {
	if _, ok := taskLadders[task]; !ok {
		return model.Response{}, RouteInfo{}, fmt.Errorf("ai: unknown task %q", task)
	}
	lc := newLogicalCall()
	defer r.flushDetached(ctx, r.binding(), lc)
	return r.completeStructuredOn(ctx, lc, task, req, validate, "")
}

// completeStructuredOn is the §5.2 policy over a logical call its caller owns
// and flushes. firstReason is why its first walk runs: "" for an ordinary first
// try, a decision_* reason when a decision attempt in the same logical call
// did not stand.
func (r *Router) completeStructuredOn(ctx context.Context, lc *logicalCall, task Task, req model.Request,
	validate Validator, firstReason string,
) (model.Response, RouteInfo, error) {
	ladder := taskLadders[task]
	resp, info, err := r.serveAttempt(ctx, lc, task, ladder, req, firstReason)
	if err != nil {
		return model.Response{}, info, err
	}
	firstErr := validate(resp.Text)
	if firstErr == nil {
		return resp, info, nil
	}
	r.forgetCached(ctx, task, req)

	retry := feedbackFor(req, resp, firstErr)
	resp, info, err = r.serveAttempt(ctx, lc, task, ladder, retry, attemptReasonSchemaInvalid)
	if err != nil {
		return model.Response{}, info, err
	}
	secondErr := validate(resp.Text)
	if secondErr == nil {
		return resp, info, nil
	}
	r.forgetCached(ctx, task, retry)

	escalated := feedbackFor(req, resp, secondErr)
	resp, info, err = r.completeEscalated(ctx, lc, task, escalated)
	if err != nil {
		return model.Response{}, info, err
	}
	if finalErr := validate(resp.Text); finalErr != nil {
		r.forgetCached(ctx, task, escalated)
		return model.Response{}, info, rejected(task, info, finalErr, truncated(resp))
	}
	return resp, info, nil
}

// rejected names what refused the text. The offline fake cannot produce a
// valid answer for any structured task, so a rejection it served is an
// unconfigured installation and gets its own sentinel on top of the terminal
// one — the caller then has the choice between "your model misbehaved" and
// "you have no model", which is the difference between a retry and a setting.
func rejected(task Task, info RouteInfo, finalErr error, wasTruncated bool) error {
	// The truncation is said BEFORE the validator's complaint, because the
	// complaint is a consequence: a document cut mid-value is invalid for a
	// reason that has nothing to do with how it was written. An operator reading
	// this is owed the output ceiling and not the schema — and the task, which
	// an earlier spelling of this branch dropped by rebuilding the error.
	reason := "after retry and escalation"
	if wasTruncated {
		reason = "after retry and escalation, and the last answer was cut off at the output " +
			"limit rather than finishing, so what the validator refused was an incomplete document"
	}
	err := fmt.Errorf("%w: %s %s: %w", ErrOutputRejected, task, reason, finalErr)
	if info.Provider == ProviderFake {
		return fmt.Errorf("%w: %w", ErrUnconfiguredModel, err)
	}
	return err
}

// forgetCached evicts the cached completion for exactly this request: a
// response the validator rejected must not be replayed to a future
// identical call. Without this, a retried BUILD with an unchanged corpus
// deterministically replays its own failure from the cache until the TTL
// expires. Best-effort — a request outside a workspace has no cache row.
func (r *Router) forgetCached(ctx context.Context, task Task, req model.Request) {
	rawWS, ok := principal.WorkspaceID(ctx)
	if !ok {
		return
	}
	key, err := cacheKey(ids.From[ids.WorkspaceKind](rawWS), task, req)
	if err != nil {
		// The serve path derived this same key moments ago, so a failure
		// here is a real anomaly: an unevicted invalid answer would replay
		// on retry, which must not stay invisible to the operator.
		r.log.WarnContext(ctx, "ai: cache eviction skipped", "task", string(task), "err", err)
		return
	}
	r.cache.forget(key)
}

// truncationFeedback is what a cut-off attempt is told instead of a complaint
// about its shape. Actionable where the schema complaint is not: a model whose
// JSON was well-formed until the ceiling cut it can act on "you ran out of
// room" and cannot act on "your JSON is invalid".
const truncationFeedback = "Your previous answer was cut off because it reached the output limit " +
	"before it finished. Answer the same question again, but much more briefly — " +
	"the shortest complete answer that satisfies the schema."

// truncated reports whether resp is a completion the provider stopped at the
// output ceiling rather than at the end of its answer.
func truncated(resp model.Response) bool {
	return resp.FinishReason == model.FinishReasonLength
}

// withTruncationFeedback is the retry for an attempt that ran out of room.
//
// It does NOT echo the failed text back, unlike the schema-invalid retry: the
// failed text here is up to a whole output budget of runaway, and quoting it
// would spend the retry's own prompt window on the thing that caused the
// problem. The instruction alone is what the model can act on.
func withTruncationFeedback(req model.Request) model.Request {
	out := req
	out.Messages = append(append([]model.Message{}, req.Messages...),
		model.Message{Role: roleUser, Content: truncationFeedback})
	return out
}

// feedbackFor picks the retry an attempt has earned. A cut-off answer gets the
// lever for whatever spent the budget — room, when thinking spent it; brevity,
// when the answer did — and a complete-but-wrong one is shown why it was
// refused.
//
// Room is only a lever while there is room to give. A request already at
// roomToAnswerMaxTokens would go out again unchanged — the same request re-rolled,
// and a byte-identical one besides, which the result cache cannot tell from
// the attempt that failed — so it gets the brevity retry instead: the one lever
// left is asking for less.
func feedbackFor(req model.Request, resp model.Response, cause error) model.Request {
	switch {
	case truncated(resp) && thinkingSpentTheBudget(resp):
		if room := withRoomToAnswer(req, resp); room.MaxTokens > appliedCeiling(req) {
			return room
		}
		return withTruncationFeedback(req)
	case truncated(resp):
		return withTruncationFeedback(req)
	default:
		return withValidatorFeedback(req, resp.Text, cause)
	}
}

// thinkingSpentTheBudget reports whether a cut-off attempt's reasoning, not its
// answer, took the larger share of the output ceiling. Every reasoning wire
// charges thinking to the same ceiling as the answer, and a model can think
// until almost nothing is left; "answer more briefly" then shortens the one
// part that was already short, and the retry runs out the same way.
//
// An adapter that reports no reasoning figure leaves ReasoningTokens 0, which
// reads as an answer that ran long, and gets the brevity retry.
func thinkingSpentTheBudget(resp model.Response) bool {
	answer := resp.OutputTokens - resp.ReasoningTokens
	return resp.ReasoningTokens > 0 && resp.ReasoningTokens >= answer
}

// withRoomToAnswer is the retry for an attempt whose thinking spent its output
// ceiling: the same request, with the ceiling raised by what the thinking took,
// so the answer gets at least the whole budget it was meant to have. The
// messages are unchanged — nothing about the answer was wrong — and the raised
// ceiling alone keys the retry apart from the cut-off attempt in the cache.
//
// Raising the ceiling rather than lowering the thinking level, because it is
// the one lever every wire has: thinking controls are per vendor, and a
// structured Gemini request already thinks at low or at its model's shallower
// default (geminiStructuredThinkingLevel).
//
// The growth is bounded three ways: the ceiling at most doubles; every retry
// grows from the caller's request rather than from the previous retry, so a
// model that keeps thinking to the ceiling cannot ratchet it upward; and no
// retry asks for more than roomToAnswerMaxTokens, though it never lowers a
// ceiling the caller set above that.
func withRoomToAnswer(req model.Request, resp model.Response) model.Request {
	out := req
	ceiling := appliedCeiling(req)
	grown := min(ceiling+min(resp.ReasoningTokens, ceiling), roomToAnswerMaxTokens)
	out.MaxTokens = max(ceiling, grown)
	return out
}

// appliedCeiling is the output ceiling a request actually ran under. A request
// that set none ran under the one every adapter sends for it, and that
// constant, not the attempt's reported output count, is what a retry grows
// from: a count off the wire is the provider's claim, and it must not decide
// what the next request may spend.
func appliedCeiling(req model.Request) int {
	if req.MaxTokens <= 0 {
		return unsetMaxOutputTokens
	}
	return req.MaxTokens
}

// roomToAnswerMaxTokens is the most output a room-to-answer retry asks for:
// one more structured budget (ReasoningOutputMaxTokens) on top of the one a
// structured lane already runs under. It bounds what a retry may spend, and
// it is not a model's output limit: a serving model that cannot emit this much
// answers the retry with a 400, which walks the ladder like any other refusal.
const roomToAnswerMaxTokens = 2 * ReasoningOutputMaxTokens

// withValidatorFeedback appends the failed output and its validation
// error as conversation turns, so the retry is a correction, not a
// blind re-roll. The changed messages also miss the result cache — a
// retry can never be served the cached invalid answer.
//
// Both echoed turns are DATA and go inside the request's boundary. The failed
// output is the model repeating text a sender steered, and the validator's
// message quotes tokens out of it, so appending either in the clear would put
// captured text back in the instruction region — while the system prompt still
// says the markers are the ONLY boundary. That is the hole the fence exists to
// close, reached on the repair path instead of the first attempt, and it
// compounds: attempt 3 escalates to a STRONGER model carrying the same turns.
//
// A prompt that declares NO boundary was shown no untrusted data, so its failed
// output is not attacker-steered and goes back plainly. The quarantine is for
// the prompts that named a fence, which are exactly the ones that read captured
// text. A prompt that declares a boundary this package could not have minted is
// neither of those: it claims to carry untrusted data behind a marker nothing
// guarantees, so the echo is dropped rather than sent under a false protection.
func withValidatorFeedback(req model.Request, failedText string, cause error) model.Request {
	out := req
	fence, boundary := feedbackFence(out)
	switch boundary {
	case boundaryMalformed:
		// Fail closed: the retry keeps the instruction and loses the detail,
		// which costs a correction and risks nothing.
		out.Messages = append(append([]model.Message{}, req.Messages...),
			model.Message{Role: roleUser, Content: retryInstruction})
		return out
	case boundaryNone:
		return appendFeedback(out, req, failedText, cause, func(s string) string { return s })
	default:
		// WrapAuthored, not Wrap: this text came from a model that was SHOWN the
		// marker, so it is the one author who can close the span exactly.
		return appendFeedback(out, req, failedText, cause, fence.WrapAuthored)
	}
}

func appendFeedback(out, req model.Request, failedText string, cause error, echo func(string) string) model.Request {
	out.Messages = append(
		append([]model.Message{}, req.Messages...),
		model.Message{Role: roleAssistant, Content: echo(failedText)},
		model.Message{Role: roleUser, Content: "That output failed validation: " +
			echo(cause.Error()) + "\n" + retryInstruction},
	)
	return out
}

// retryInstruction is the only part of the feedback turns written by this
// codebase, and therefore the only part that carries authority.
const retryInstruction = "Return ONLY the corrected output in the required format."

// boundaryState is what a prompt says about its own data boundary. The three
// cases need three different answers, and collapsing "malformed" into "none" is
// the one combination that fails OPEN.
type boundaryState int

const (
	boundaryNone boundaryState = iota
	boundaryDeclared
	boundaryMalformed
)

// feedbackFence resolves the boundary the echoed turns belong in: the one the
// prompt already declared, never a second one beside it.
func feedbackFence(req model.Request) (promptfence.Fence, boundaryState) {
	marker, declared := promptfence.MarkerIn(req.System)
	if !declared {
		return promptfence.Fence{}, boundaryNone
	}
	fence, ok := promptfence.FromMarker(marker)
	if !ok {
		return promptfence.Fence{}, boundaryMalformed
	}
	return fence, boundaryDeclared
}

// completeEscalated serves one attempt from the task's ladder with the
// first rung dropped — "escalate one tier on second failure" (§5.2) — as
// the third attempt of lc's logical call. An escalation with nowhere to
// go — a single-rung ladder, or no bound client on any remaining rung —
// answers from the default route instead of failing a call the task's own
// ladder could still serve.
func (r *Router) completeEscalated(ctx context.Context, lc *logicalCall, task Task, req model.Request) (model.Response, RouteInfo, error) {
	ladder, ok := taskLadders[task]
	if !ok || len(ladder) < 2 || !r.anyBound(r.binding(), ladder[1:]) {
		return r.serveAttempt(ctx, lc, task, ladder, req, attemptReasonSchemaInvalid)
	}
	return r.serveAttempt(ctx, lc, task, ladder[1:], req, attemptReasonSchemaInvalid)
}

// anyBound reports whether at least one rung resolves to a bound client.
func (r *Router) anyBound(b *binding, ladder []Tier) bool {
	for _, t := range ladder {
		if _, ok := b.clients[t]; ok {
			return true
		}
	}
	return false
}
