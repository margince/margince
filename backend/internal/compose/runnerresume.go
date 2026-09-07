// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Resuming a parked run: what happens when the human answers.
//
// Split from runnerservice.go because it is the half with a second actor in it.
// Everything there runs on the scheduler's own clock and answers to nobody;
// this answers to a decision that arrived on the bus, at a time nothing here
// chose, possibly twice, and possibly after the authority that staged the call
// has gone. The rules that follow from that — decide before claiming, claim and
// close in one write, treat a second delivery as normal — belong together and
// nowhere near the tick loop.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/modules/identity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalDecision is the answer as it arrives on the bus. Named rather than
// anonymous because canResume reads it too, and two spellings of one payload is
// how the two ends come to disagree about what "edited" meant.
type approvalDecision struct {
	Verdict      string          `json:"verdict"`
	Edited       bool            `json:"edited"`
	EditedChange json.RawMessage `json:"edited_change"`
}

// HandleEvent is the cg:overnight-agent consumer: an approval decision
// on a runner staging resumes the parked run with the human's answer.
// Every other event on the group's streams is not ours — nil, not an
// error, so the group keeps flowing.
func (s *RunnerService) HandleEvent(ctx context.Context, env kevents.Envelope) error {
	if env.Type != "approval.decided" {
		return nil
	}
	approvalID := ids.From[ids.ApprovalKind](env.Entity.ID)
	// The envelope carries no tenant (ADR-0091 §6): this consumer resolves the
	// installation, exactly as the request paths beside it do.
	ws, err := s.identity.InstallationWorkspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	// The resume path's own actor, for the same reason Tick binds one: every
	// terminal write below announces the occurrence to the AI-activity
	// projection, and an announcement carries the write shape — a ledger row and
	// an outbox row, both of which take their actor from the context. Without it
	// MarkFailed rolls back, the claim is not undone, and the run is parked
	// forever in a state no redelivery can close.
	ctx = principal.WithCorrelationID(ctx, env.EventID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: resumeActor,
	})

	// The payload is read BEFORE the run is claimed: claiming is one-way, so
	// every step after it must end in a terminal status rather than in a
	// retriable error — a redelivery would find nothing to resume and leave
	// the run parked in 'running' forever.
	var payload approvalDecision
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		return fmt.Errorf("runner: approval.decided payload: %w", err)
	}

	// LOOK before claiming, because the claim is one-way and the three answers
	// below are all "this run cannot resume". Claiming first made each of them a
	// second transaction, and a failure there left the claim taken and the
	// reason unwritten: the redelivery finds no awaiting_approval row, correctly
	// declines to start a second loop of a mutation a human approved once, and
	// acks — so nothing closes the run until the abandoned sweep reaches it half
	// an hour later and records the generic reason instead of the specific one
	// (#2224). Deciding first lets each of them claim and close in one write.
	//
	// The peek is not a race the claim then loses: every writer below still
	// gates on status = 'awaiting_approval', so of two deliveries that both peek,
	// exactly one writes.
	suspended, found, err := s.store.PeekSuspendedByApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if !found {
		return nil // a human-surface approval, or a decision already resumed
	}
	agentIdentity, spec, reason := s.canResume(ctx, suspended, payload)
	if reason != "" {
		// Claim and close in ONE write. Claiming first and failing second is
		// what left a run stranded, so the refusal takes the claim with it.
		_, err := s.store.ClaimAndClose(ctx, approvalID, reason)
		return err
	}

	// Every reason to refuse is answered; the claim is the last thing before the
	// loop, so what it takes it can carry to a terminal status.
	claimed, found, err := s.store.ClaimSuspendedByApproval(ctx, approvalID)
	if err != nil {
		return err
	}
	if !found {
		return nil // another delivery resumed or closed it between the peek and here
	}
	// The peeked row decided the branches; the CLAIMED row is what the loop runs,
	// carrying the human's edited args over it again — they are the approval's,
	// not the row's.
	suspended = claimed
	if payload.Verdict == approvalStatusApproved && payload.Edited {
		suspended.Pending.Args = payload.EditedChange
	}

	// The resumed leg is the SAME logical run but a new causal moment;
	// it groups its writes under a fresh correlation id.
	runCtx := principal.WithCorrelationID(principal.WithActor(ctx, agentIdentity.Principal()), ids.NewV7())
	runCtx = principal.WithAgentRunID(runCtx, suspended.RunID)

	bounded, cancel := context.WithTimeout(runCtx, RunWallClock)
	defer cancel()
	// Tools rides the CURRENT catalog entry, beside the current budget and
	// for the same reason: a suspended run resumes under the authority the
	// entry states now, never the one it stated when the call was staged.
	res, err := s.runner.Resume(bounded, runner.Job{
		Goal:       suspended.Goal,
		TriggerRef: suspended.TriggerRef,
		Budget:     spec.Budget,
		Tools:      spec.Tools,
		// Resolved fresh on resume rather than carried in the suspended row:
		// the summary is written after the human answers, so it takes the
		// language the installation has NOW, the same way Tools rides the
		// current catalog entry above.
		LanguageRule: promptlang.Rule(identity.BaseLanguageForPrompt(bounded, s.pool)),
	}, runner.Decision{
		Pending:  suspended.Pending,
		Approved: payload.Verdict == approvalStatusApproved,
	})
	s.landOutcome(runCtx, suspended.RunID, suspended.TriggerRef, res, err)
	return nil
}

// landOutcome persists how a run ended. triggerRef names the occurrence for the
// operator log, which is where a fault's cause goes: the run's own error is a
// wrapped internal one, and agent_run.degrade_reason is read by the human the run
// acted for.

// canResume answers whether the parked run can run at all, BEFORE anything
// claims it. An empty reason means yes, and hands back the two things resuming
// needs; anything else is terminal and the caller closes the run with it.
//
// Every branch here used to sit after the claim, which made each of them a
// second transaction that could fail on its own and strand the run (#2224).
// They are questions about the world rather than about the row, so nothing
// required them to be asked later.
func (s *RunnerService) canResume(
	ctx context.Context, suspended runner.SuspendedRun, payload approvalDecision,
) (identity.AgentIdentity, runner.AgentSpec, runner.FailureReason) {
	// Modify-then-approve (ADR-0036 §4): the authority now binds to the HUMAN's
	// version of the call, so the resumed run must re-present exactly that — the
	// originally staged args no longer redeem.
	if payload.Verdict == approvalStatusApproved && payload.Edited && len(payload.EditedChange) == 0 {
		return identity.AgentIdentity{}, runner.AgentSpec{}, runner.FailureEditedApprovalCarriedNoChange
	}

	agentIdentity, err := s.identity.AuthenticateAgentByID(ctx, suspended.PassportID)
	if err != nil {
		// The passport died while the run was parked (revoked, expired, human
		// deactivated). The run cannot act anymore. WHICH of those happened is
		// the identity module's own message, so it goes to the operator and not
		// to the column the person reads.
		s.log.Warn("runner: a suspended run's authority died before it could resume",
			"trigger_ref", suspended.TriggerRef, "run", suspended.RunID, "cause", err)
		return identity.AgentIdentity{}, runner.AgentSpec{}, runner.FailurePassportNoLongerValid
	}

	spec, known := s.specByName(suspended.SpecName)
	if !known {
		s.log.Warn("runner: a suspended run's agent left the catalog",
			"trigger_ref", suspended.TriggerRef, "run", suspended.RunID, "spec", suspended.SpecName)
		return identity.AgentIdentity{}, runner.AgentSpec{}, runner.FailureSpecLeftTheCatalog
	}
	return agentIdentity, spec, ""
}
