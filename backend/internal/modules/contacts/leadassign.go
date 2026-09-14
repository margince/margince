// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maxBulkAssign is the contract's cap, enforced in the store because nothing
// between the wire and here reads maxItems.
const maxBulkAssign = 500

// AssignLeadsInput is one destination and the leads to hand to it.
type AssignLeadsInput struct {
	OwnerID ids.UserID
	Leads   []AssignLeadItem
}

// AssignLeadItem names one lead, and optionally the version the caller read.
type AssignLeadItem struct {
	ID ids.LeadID
	// IfVersion makes this row's write conditional exactly as If-Match does on
	// the single update. Nil assigns whatever the lead is now.
	IfVersion *int64
}

// AssignLeadOutcome is what happened to one named lead.
type AssignLeadOutcome struct {
	LeadID  ids.LeadID
	Outcome crmcontracts.AssignLeadOutcomeKind
	// Version after the write; nil unless the lead was read back.
	Version *int64
}

// AssignLeads hands a named set of leads to one owner, one row at a time.
//
// PER ROW rather than one transaction, which is the opposite of the choice
// relinkActivities makes and for a stated reason: its rows are one
// conversation, and these are a manager's screenful of independent records.
// Refusing the whole selection because the fortieth lead moved under the
// reader would make the queue unworkable, so each lead answers for itself.
//
// Every row goes through UpdateLead — the same writer the single assignment
// uses, with the same gate, the same audit row and the same event. A second
// assignment path that wrote the owner itself would be a second answer to who
// may hand on a lead, and the two would drift.
func (s *Store) AssignLeads(ctx context.Context, in AssignLeadsInput) ([]AssignLeadOutcome, error) {
	// The contract's 1..500 is enforced HERE, not by the schema: this
	// installation runs no request-validator middleware, so maxItems is
	// documentation until a handler reads it. RelinkActivities checks its own
	// cap the same way and for the same reason.
	if len(in.Leads) == 0 || len(in.Leads) > maxBulkAssign {
		return nil, httperr.Validation("leads", "out_of_range",
			fmt.Sprintf("leads names between 1 and %d leads; this request names %d",
				maxBulkAssign, len(in.Leads)))
	}
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return nil, err
	}
	// Putting somebody's name on a customer record is a human's act, in bulk
	// exactly as it is one at a time (ClaimRecord says the same).
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	// The destination is a fact about a SEAT, not about any lead, so it is
	// asked once before anything is written: a run that assigned thirty leads
	// and then discovered the owner was suspended would leave the reader to
	// undo thirty writes by hand. Each row re-asks it inside its own
	// transaction, because a seat can be suspended mid-run.
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		return auth.EnsureAssignee(ctx, tx, in.OwnerID.UUID)
	}); err != nil {
		return nil, err
	}

	owner := in.OwnerID
	out := make([]AssignLeadOutcome, 0, len(in.Leads))
	for _, item := range in.Leads {
		lead, err := s.UpdateLead(ctx, item.ID, UpdateLeadInput{
			OwnerID:   &owner,
			IfVersion: item.IfVersion,
		})
		outcome, ok := assignOutcomeOf(item.ID, lead, err)
		if !ok {
			// Not a verdict about this lead — the database went away, or the
			// request was cancelled. Reporting it as "forbidden" would tell the
			// reader to ask for permission they already have, and would persist
			// that lie as this request's replayable answer. The run fails.
			return nil, err
		}
		out = append(out, outcome)
	}
	return out, nil
}

// assignOutcomeOf reads one row's answer off what the writer returned.
//
// The refusals a bulk run reports are the ones a caller can act on — retry the
// row, drop it from the selection, ask somebody else. Anything else is a fault
// in the run itself and is not turned into a row outcome, because a result
// that reports "forbidden" for a database that went away tells the reader to
// fix the wrong thing.
func assignOutcomeOf(id ids.LeadID, lead crmcontracts.Lead, err error) (AssignLeadOutcome, bool) {
	switch {
	case err == nil:
		return AssignLeadOutcome{
			LeadID:  id,
			Outcome: crmcontracts.AssignLeadOutcomeKindAssigned,
			Version: lead.Version,
		}, true
	case errors.Is(err, apperrors.ErrVersionSkew), errors.Is(err, apperrors.ErrConflict):
		return AssignLeadOutcome{LeadID: id, Outcome: crmcontracts.AssignLeadOutcomeKindConflict}, true
	case errors.Is(err, apperrors.ErrNotFound):
		return AssignLeadOutcome{LeadID: id, Outcome: crmcontracts.AssignLeadOutcomeKindNotFound}, true
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return AssignLeadOutcome{LeadID: id, Outcome: crmcontracts.AssignLeadOutcomeKindForbidden}, true
	default:
		return AssignLeadOutcome{}, false
	}
}

// ensureLeadUpdateAuthority is the write gate in front of a lead update, and
// it asks a different question when the update hands the lead to somebody.
//
// An ordinary edit stays EnsureWritable's: a lead nobody owns is nobody's to
// rewrite. An ASSIGNMENT is the one act that must reach an ownerless lead —
// otherwise the queue every inbound lead lands in has no door out for a Team
// Lead, and the rep's only exit is the claim verb, which cannot hand a lead to
// a colleague.
//
// The exception is deliberately narrow. It admits an ownership-ONLY patch, so
// the ownerless arm never becomes a way to rewrite a lead's score, status or
// identity on the way past. A caller who may already write the row keeps every
// field, exactly as before; a caller who may not gets the owner and nothing
// else.
func ensureLeadUpdateAuthority(ctx context.Context, tx pgx.Tx, id ids.LeadID, in UpdateLeadInput) error {
	if in.OwnerID == nil {
		return auth.EnsureWritable(ctx, tx, "lead", id.UUID)
	}
	// The LOCK comes before the decision, the same order ClaimOwnership takes
	// and for the same reason. Whether this lead is ownerless is the fact the
	// whole gate turns on, and reading it unlocked leaves an interval in which
	// somebody else claims it: two reps both pass the ownerless arm, both
	// write, and the second silently takes a lead off the first — a record
	// neither rep could otherwise have touched. Holding the row makes the
	// loser's gate see the winner's name.
	if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	if !ownershipOnlyLeadUpdate(in) {
		// Mixed content: the write arm answers, so an ownerless lead refuses
		// the whole patch rather than admitting the fields riding along with
		// the owner.
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
	}
	return auth.EnsureAssignable(ctx, tx, "lead", id.UUID, in.OwnerID.UUID)
}

// ownershipOnlyLeadUpdate answers whether this patch changes the owner and
// NOTHING else.
//
// Written as an exhaustive read of the input rather than a count of set
// fields: a new field added to UpdateLeadInput must be considered here, and a
// reader adding one sees the list it has to join. IfVersion and Trail are
// absent on purpose — a version precondition is not a field being written, and
// the trail names the write rather than changing the row.
func ownershipOnlyLeadUpdate(in UpdateLeadInput) bool {
	return len(in.Clear) == 0 && len(in.CustomFields) == 0 &&
		in.FullName == nil && in.Email == nil && in.Title == nil &&
		in.CompanyName == nil && in.CandidateCompanyKey == nil &&
		in.Status == nil && in.Source == nil && in.Score == nil &&
		in.ScoreOverrideReason == nil && !in.ClearScoreOverride &&
		in.ProjectID == nil
}
