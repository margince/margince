// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The §2.4 ladder from the tool surface's side: what an agent is told, and where
// the question goes, when it crosses a threshold a human can release.
//
// The 🟡 loop next door and this one look alike and are not. A confirm-first
// call asks a human to release ONE act, and the agent redeems that release by
// repeating the identical call with its approval_id. A step-up asks whether this
// agent may CONTINUE, and there is nothing to redeem: approving it widens the
// window, and the agent's next ordinary call succeeds. An agent told to retry
// with an approval_id here would present a token for a kind no tool can redeem.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// VolumeReleaseRequest is what the surface asks the approvals engine to put in
// front of the human who lent this passport.
type VolumeReleaseRequest struct {
	// Proposal is the question itself, in the one spelling both modules read.
	Proposal agentvolume.ReleaseProposal
	// Summary is the sentence the inbox shows.
	Summary string
}

// StepUpStagedError answers a call refused on a releasable volume budget once the
// question has been put to a human.
//
// It is its own type, and it does NOT unwrap to ErrRequiresApproval. That
// sentinel means "this act is confirm-first", and every transport that sees it
// tells the caller to retry with an approval_id — which for a step-up is advice
// that cannot work. It unwraps to the budget sentinel instead, which is what the
// refusal actually is: a volume threshold, now with a human looking at it.
type StepUpStagedError struct {
	ApprovalID ids.ApprovalID
	Counter    agentvolume.Counter
}

func (e *StepUpStagedError) Error() string {
	return fmt.Sprintf(
		"this agent has spent its %s allowance for this window; the person who connected it has been asked whether it may continue (approval %s)",
		e.Counter, e.ApprovalID)
}

// Unwrap keeps every budget-aware caller — the REST gate, the transports' error
// mapping — seeing this for what it is.
func (e *StepUpStagedError) Unwrap() error { return apperrors.ErrBudgetExceeded }

// releasableVolumeRefusal answers the refusal a human can still say yes to, and
// nil for every other outcome — including a volume budget refusal on a hard stop, which
// no approval lifts and which must therefore never reach an inbox.
func releasableVolumeRefusal(err error) *auth.VolumeExceededError {
	var over *auth.VolumeExceededError
	if errors.As(err, &over) && over.Releasable() {
		return over
	}
	return nil
}

// askedOfAHuman reports whether this admission outcome ends with a question in
// somebody's inbox — the two shapes whose arguments are therefore worth
// validating first, so no human is asked to release a call that could not have
// run anyway.
func askedOfAHuman(err error) bool {
	return err == nil || errors.Is(err, apperrors.ErrRequiresApproval) || releasableVolumeRefusal(err) != nil
}

// stageStepUp puts a releasable volume budget refusal in front of the human who lent
// this passport, and answers what the agent is told.
//
// The REFUSAL IS STILL THE ANSWER if the question cannot be asked. A surface
// with no approvals engine, a staging that fails, and a human who has already
// REJECTED this question all end the same way: the call is refused on the
// volume budget, exactly as it would have been. That is the conservative direction —
// the alternative is a refused call reported as an error about staging, which
// tells the agent to retry the staging rather than to stop.
//
// A human's NO is remembered, which is the whole reason this goes through the
// declined-aware staging path. An agent looping on a refusal would otherwise
// re-ask a question its human just answered, once per call, forever.
func (r *Registry) stageStepUp(ctx context.Context, refusal *auth.VolumeExceededError) error {
	if r.approvals == nil {
		return refusal
	}
	// The passport this call presented, which is the window the question is
	// about. Read from the principal rather than passed down, because it is the
	// same value the staging stamps the row with.
	//
	// No passport, no question. A step-up names WHOSE window it is, and one
	// stamped with the zero uuid names nobody: the identity would collide with
	// every other such call, and applyVolumeRelease refuses to release a row
	// without a passport anyway — so staging it would put a question in an inbox
	// that approving could never answer. The volume budget refusal stands instead, which
	// is the same conservative direction every other branch here takes.
	actor, present := principal.Actor(ctx)
	if !present || actor.PassportID == (ids.UUID{}) {
		return refusal
	}
	proposal := agentvolume.NewReleaseProposal(refusal.Reading, actor.PassportID, refusal.Tool)
	id, staged, err := r.approvals.StageVolumeRelease(ctx, VolumeReleaseRequest{
		Proposal: proposal,
		Summary:  stepUpSummary(proposal),
	})
	if err != nil {
		// The refusal stands as the answer; the staging failure is ours and
		// belongs in the log, not in a message that would send the agent back to
		// retry it.
		slog.ErrorContext(ctx, "putting a step-up in front of the connecting human failed",
			"counter", string(refusal.Reading.Counter), "tool", refusal.Tool, "err", err)
		return refusal
	}
	if !staged {
		return refusal
	}
	return &StepUpStagedError{ApprovalID: id, Counter: refusal.Reading.Counter}
}

// stepUpSummary is the question, in the human's words rather than the counter's.
// It states the volume, the ceiling and the consequence of saying yes, because a
// human answering "continue?" with no numbers is answering nothing.
func stepUpSummary(p agentvolume.ReleaseProposal) string {
	act := "been handed"
	unit := "records"
	if p.Counter != agentvolume.Reads {
		act, unit = "made", "changes"
	}
	return fmt.Sprintf(
		"This agent has %s %d %s against a limit of %d for this window (most recently through %s). Approve to let it continue for another %d.",
		act, p.Observed, unit, p.Limit, p.Tool, p.Allowance)
}
