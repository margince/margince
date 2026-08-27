// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Applying the corrections nobody needs to be asked about.
//
// A queue that asks about everything trains the reflex it exists to prevent: a
// person who has approved fourteen close-date corrections unchanged is not
// reading the fifteenth. So the reversible ones apply themselves and appear as
// receipts a person can undo, and the queue keeps only what is actually a
// decision.
//
// WHAT MAKES A KIND ELIGIBLE, and it is three things at once:
//   - the change is REVERSIBLE — compose/undoability.go answers this, and the
//     apply below refuses a kind whose own audit row it could not offer an undo
//     for. That evaluator is the product's one answer to reversibility; this
//     file asks it rather than keeping a second opinion.
//   - the change is INTERNAL. Nothing here reaches a customer. An outbound send
//     is gated whoever asked for it, and agentMayDecide charges a send cap this
//     pass deliberately does not carry, so a sending kind arriving here is
//     refused twice rather than once.
//   - an AUTHORITY can be named — see autoapplyauthority.go. No owner, no apply.
//
// The three eligible kinds are declared in autoApplyKinds. It is a list rather
// than a rule because the product decided each one deliberately, and a rule that
// admitted kinds nobody had looked at would be the wrong kind of clever.
//
// WHAT THIS IS NOT. It does not decide better than a person; it decides the same
// and sooner, on changes a person can put back in one click. Every apply writes
// the audit row an undo needs, so the receipt is not a promise the product keeps
// separately — it is the same trail the history screen already reads.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// autoApplyKinds are the proposal kinds that apply themselves.
//
// Each is a single reversible field write on a record whose owner can be named,
// and each was chosen one at a time. Adding to this list is a product decision,
// not a refactor: TestAutoAppliedKindsAreUndoable holds that a kind here can
// actually be put back, and TestAutoAppliedKindsNeverSend holds that none of
// them reaches a customer.
var autoApplyKinds = []string{
	deals.CloseDateCorrectionKind,
	orgNameProposalKind,
	lifecycleProposalKind,
}

// autoApplyBatch caps one pass, so a backlog is worked down over several runs
// rather than held in one transaction. Nothing here is urgent enough to justify
// a long-running write.
const autoApplyBatch = 50

// autoApplyOutcome is what one pass did, for the log line and the tests.
//
// Refusals are counted rather than raised: a proposal this pass cannot apply is
// a proposal a person will see, which is the system working. Only a fault the
// pass cannot attribute to one proposal ends the run.
type autoApplyOutcome struct {
	Applied  int
	Refused  int
	OptedOut int
}

// autoApplier applies the eligible proposals a pass finds.
type autoApplier struct {
	approvals *approvals.Service
	authority autoApplyAuthority
	// undoable reports whether the change this proposal would make could be put
	// back. Bound to the real evaluator in the composition root; a port here so
	// the refusal can be tested without standing up six modules.
	undoable func(ctx context.Context, targetType string, id ids.UUID) (bool, error)
	// optedOut reports whether this owner has switched automatic changes off.
	optedOut func(ctx context.Context, owner ids.UUID) (bool, error)
	log      *slog.Logger
}

// run applies what it can and reports what it did.
//
// The pass reads under its own principal — a system sweep sees every pending
// proposal, which is what lets it work a queue nobody is watching. It APPLIES
// under the record owner's authority, which is the boundary that matters: the
// read chooses the work, the write is bounded by the person it acts for.
func (a autoApplier) run(ctx context.Context) (autoApplyOutcome, error) {
	var out autoApplyOutcome
	for _, kind := range autoApplyKinds {
		pending, _, err := a.approvals.ListWire(ctx, approvals.ListInput{
			Status: ptrString(string(crmcontracts.ApprovalStatusPending)),
			Kind:   ptrString(kind),
			Limit:  autoApplyBatch,
		})
		if err != nil {
			return out, fmt.Errorf("compose: reading %s proposals to apply: %w", kind, err)
		}
		for _, proposal := range pending {
			applied, err := a.applyOne(ctx, proposal)
			switch {
			case errors.Is(err, errAutoApplyOptedOut):
				out.OptedOut++
			case err != nil:
				// One proposal's refusal is not the pass's failure: the card
				// stays for a person, and the next proposal is unaffected.
				out.Refused++
				a.log.InfoContext(ctx, "auto-apply: leaving a proposal for a person",
					"kind", proposal.Kind, "approval_id", proposal.Id, "reason", err)
			case applied:
				out.Applied++
			}
		}
	}
	return out, nil
}

// errAutoApplyOptedOut reports an owner who has switched automatic changes off.
// Its own sentinel so the count separates "the person said no" from "the product
// could not", which are different facts about a queue that is not shrinking.
var errAutoApplyOptedOut = errors.New("compose: this record's owner has switched automatic changes off")

// applyOne applies one proposal, or says why it did not.
//
// The order is deliberate and is cheapest-refusal-first, the same shape the
// undoability evaluator uses: the checks that need no database come before the
// ones that do, and the authority is resolved before the reversibility question
// because an unowned record has no reader to ask it for.
func (a autoApplier) applyOne(ctx context.Context, proposal crmcontracts.Approval) (bool, error) {
	if proposal.TargetEntityType == nil || proposal.TargetEntityId == nil {
		return false, errors.New("the proposal names no target to apply to")
	}
	targetType := *proposal.TargetEntityType
	targetID := ids.UUID(*proposal.TargetEntityId)

	// An unownable target type is refused by recordOwner itself, which is the
	// one place that knows which types it can read an owner from. A second
	// check here would be a second copy of that list, and the two would drift.
	owner, err := a.authority.recordOwner(ctx, targetType, targetID)
	if err != nil {
		return false, err
	}
	if owner.IsZero() {
		return false, errNoRecordOwner
	}
	off, err := a.optedOut(ctx, owner)
	if err != nil {
		return false, fmt.Errorf("reading whether the owner takes automatic changes: %w", err)
	}
	if off {
		return false, errAutoApplyOptedOut
	}

	// Reversibility is asked BEFORE the apply, under the owner's own authority,
	// because the evaluator's answer depends on who is asking — a record they
	// may not write is not one they may put back either.
	ownerCtx, err := a.authority.asAgentFor(ctx, owner)
	if err != nil {
		return false, err
	}
	reversible, err := a.undoable(ownerCtx, targetType, targetID)
	if err != nil {
		return false, fmt.Errorf("asking whether the change could be put back: %w", err)
	}
	if !reversible {
		return false, errors.New("the change could not be put back, so it is not applied automatically")
	}

	approvalID := ids.From[ids.ApprovalKind](ids.UUID(proposal.Id))
	if _, err := a.approvals.Decide(ownerCtx, approvalID, true, nil); err != nil {
		return false, fmt.Errorf("applying the proposal: %w", err)
	}
	return true, nil
}
