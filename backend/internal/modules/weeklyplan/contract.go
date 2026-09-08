// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

// What the rep expects to get in the way, and what they say about their room.
//
// The commitments beside these say what the week is FOR. These say what it is
// up against, and they are the half a lead actually reads: five commitments
// with no room to do them is a plan that has already failed, and nobody has
// said so out loud.
//
// Plan-level rather than per-commitment, which is why this is not AskForHelp.
// That asks for help with ONE thing; this is the shape of the whole week, and a
// risk that belongs to no single commitment has nowhere else to live.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The two columns this write owns, and the field name its event reports.
const (
	fieldRisks        = "risks"
	fieldCapacityNote = "capacity_note"

	// changedContract is what a consumer filters on to see either half move.
	// One name for both, because they are written together and a reader of the
	// week wants to know the rep restated it, not which column moved.
	changedContract = "contract"
)

// SetContract records what the rep says about the week ahead.
//
// Both fields travel together and either may be nil, which is what makes
// "unset" reachable: a nil clears the column back to NULL, and NULL is the rep
// having said nothing. An empty string is a different statement — they looked,
// and there is nothing to name — so the two are not folded here.
//
// A CLOSED WEEK IS REFUSED, through the same door every other write uses. Its
// counts are frozen into a review, and a plan that kept accepting prose after
// the retrospective quoted it would let a rep rewrite what their lead already
// read.
func (s *Store) SetContract(ctx context.Context, now time.Time, in ContractEdit) (Plan, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return Plan{}, err
	}
	// Bounded at the seam so the client gets a named field back rather than a
	// constraint violation. The column carries the same ceiling, and
	// TestEveryPlanProseColumnMatchesItsGoBound fails if the two drift.
	risks, err := boundedOptional(fieldRisks, in.Risks)
	if err != nil {
		return Plan{}, err
	}
	capacityNote, err := boundedOptional(fieldCapacityNote, in.CapacityNote)
	if err != nil {
		return Plan{}, err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return Plan{}, err
	}

	var out Plan
	err = database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		// Opens the week if the rep has not started one. Writing their risks IS
		// starting to plan, and refusing until they have added a commitment
		// would make the first thing they type the one thing they cannot save.
		plan, err := s.openPlanTx(ctx, tx, owner, now)
		if err != nil {
			return err
		}
		lock, err := storekit.LockRow(ctx, tx, "weekly_plan", plan.ID, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		storedRisks, storedNote, err := contractUnderLock(ctx, tx, plan.ID)
		if err != nil {
			return err
		}
		patch := storekit.NewPatch()
		// An UNSENT half keeps what is stored, resolved from the row this
		// transaction holds rather than from one read before the lock.
		if in.SetRisks {
			patch.Set(fieldRisks, storedRisks, risks)
		}
		if in.SetCapacityNote {
			patch.Set(fieldCapacityNote, storedNote, capacityNote)
		}
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "weekly_plan", plan.ID,
			patch.Before(), patch.After())
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, owner,
			crmcontracts.PublicEventWeeklyPlanUpdated{
				PlanId: openapi_types.UUID(plan.ID), OwnerUserId: openapi_types.UUID(owner),
				ChangedFields: []string{changedContract},
			}); err != nil {
			return err
		}
		out, err = readPlan(ctx, tx, owner, plan.LocalWeekStart)
		return err
	})
	if err != nil {
		return Plan{}, err
	}
	// The counted capacity, which readPlan does not carry: it reads the plan's
	// own tables and the seam reads other modules'. Without this a successful
	// save answers with no capacity at all, which a client reads as "no
	// calendar composed" moments after a GET told it otherwise.
	out.Capacity, err = s.capacityFor(ctx, owner, out.LocalWeekStart)
	if err != nil {
		return Plan{}, err
	}
	return out, nil
}

// contractUnderLock re-reads what the write depends on, holding the row lock.
//
// openPlanTx read the row BEFORE the lock, so both the status and the current
// text were a snapshot somebody else could have moved. Two things go wrong if
// the write trusts it:
//
//   - the week can close in between (a request spanning the Monday rollover),
//     and a plan whose counts are frozen into a review must refuse the write
//     rather than rewrite what a lead has already read;
//   - two saves each touching one half would both read the old values, and the
//     second would write back the first's field as it was before, silently
//     undoing a save that reported success.
func contractUnderLock(
	ctx context.Context, tx pgx.Tx, planID ids.UUID,
) (risks, capacityNote *string, err error) {
	var status string
	if err := tx.QueryRow(ctx, `
		SELECT status, risks, capacity_note FROM weekly_plan WHERE id = $1`,
		planID).Scan(&status, &risks, &capacityNote); err != nil {
		return nil, nil, fmt.Errorf("weeklyplan: re-reading the plan under lock: %w", err)
	}
	if status != PlanOpen {
		return nil, nil, &values.ParseError{
			Field: fieldWeek, Code: codeWeekClosed,
			Message: "that week is closed; plan the current one",
		}
	}
	return risks, capacityNote, nil
}

// ContractEdit says which halves of the contract this write touches.
//
// A bare pair of pointers cannot: nil would have to mean both "clear this
// field" and "leave it alone", and those are opposite instructions. The
// Set… flags carry the distinction the wire already draws between an omitted
// key and an explicit null.
type ContractEdit struct {
	SetRisks bool
	Risks    *string

	SetCapacityNote bool
	CapacityNote    *string
}

// boundedOptional trims and bounds a field that may be absent.
//
// It keeps nil as nil rather than folding it to "", because the two are
// different answers: nobody has written this, versus the rep says there is
// nothing to write.
func boundedOptional(field string, value *string) (*string, error) {
	if value == nil {
		return nil, nil //nolint:nilnil // an absent field IS the answer here: nil means the rep left it alone, which is not an empty string.
	}
	text, err := bounded(field, *value, proseBound)
	if err != nil {
		return nil, err
	}
	return &text, nil
}

// Capacity is what next week's calendar already holds.
//
// A SEAM rather than a query, because the meetings and tasks it counts belong
// to other modules and this one may not read their tables. compose binds it.
//
// Absent — a nil Capacity on the plan — is not zero. An installation that
// composed no calendar has an UNKNOWN week ahead, and drawing that as "no
// meetings booked" would tell a rep their week is free when nothing has looked.
type Capacity interface {
	// ForWeek reports how much of ONE week is already committed, for the given
	// rep, in the installation's reporting zone.
	//
	// The week is named by the caller rather than derived here, and that is the
	// whole point: the plan's own local_week_start is the week being planned,
	// and a seam that computed "next week" for itself described a different
	// seven days from the ones the plan was stored against.
	ForWeek(ctx context.Context, owner ids.UUID, weekStart time.Time) (Committed, error)
}

// Committed is the count behind the capacity line.
//
// Two figures and not one: a week of six meetings reads differently from a week
// of six overdue tasks, and a single "commitments" total would hide which.
type Committed struct {
	Meetings int
	Tasks    int
}

// WithCapacity binds the reader behind the plan's capacity line.
//
// Called inside compose's weeklyPlanStore rather than at a call site, because
// that constructor has more than one caller: an option applied at one of them
// would give the handlers a capacity reader and leave the weekly job's store
// without one, and the difference would show as a capacity line that is present
// on the page and absent in the mail.
func (s *Store) WithCapacity(c Capacity) *Store {
	s.capacity = c
	return s
}

// capacityFor reads the load of the week the PLAN is about, or reports it
// unknown.
//
// The week comes from the plan rather than from the clock. Read as "next week"
// instead, the heading, the stored row and the capacity line answered about
// three different sets of seven days: a plan created on Tuesday was stored
// against the current week, called "next week" on the page, and priced against
// the week after it.
//
// An unbound seam answers nil, and every caller must carry that through as
// absent. This is the one place that decides it, so a surface cannot
// accidentally substitute a zero.
func (s *Store) capacityFor(ctx context.Context, owner ids.UUID, weekStart time.Time) (*Committed, error) {
	if s.capacity == nil {
		//nolint:nilnil // no capacity seam composed means the week is UNKNOWN, which a zero would misreport as free.
		return nil, nil
	}
	committed, err := s.capacity.ForWeek(ctx, owner, weekStart)
	if err != nil {
		return nil, fmt.Errorf("weeklyplan: reading the planned week's capacity: %w", err)
	}
	return &committed, nil
}
