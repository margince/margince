// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

// The plan's mutations. Every one commits its domain row, an audit row and an
// event in ONE transaction, through storekit — the write shape this tree spells
// once and every module store calls.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// StartWeek opens the caller's plan for the week containing now, or answers the
// one they already have.
//
// Idempotent by the unique constraint rather than by the read that precedes it:
// two tabs pressing "plan my week" produce one plan and no error.
func (s *Store) StartWeek(ctx context.Context, now time.Time) (Plan, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionCreate); err != nil {
		return Plan{}, err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return Plan{}, err
	}
	var plan Plan
	err = database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		week, err := s.weekStart(ctx, tx, now)
		if err != nil {
			return err
		}
		capturedBy, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		var id ids.UUID
		row := tx.QueryRow(ctx, `
			INSERT INTO weekly_plan (owner_id, local_week_start, captured_by)
			VALUES ($1, $2, $3)
			ON CONFLICT ON CONSTRAINT uq_weekly_plan_owner_week DO NOTHING
			RETURNING id`, owner, week, capturedBy)
		switch err := row.Scan(&id); {
		case errors.Is(err, pgx.ErrNoRows):
			// Theirs already. Answer it whole rather than reporting a race.
			plan, err = readPlan(ctx, tx, owner, week)
			return err
		case err != nil:
			return fmt.Errorf("weeklyplan: opening the week: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", "weekly_plan", id,
			map[string]any{"owner_id": owner.String(), "local_week_start": week.Format(time.DateOnly)})
		if err != nil {
			return err
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
			PlanId: openapi_types.UUID(id), OwnerUserId: openapi_types.UUID(owner),
			ChangedFields: []string{"status"},
		}); err != nil {
			return err
		}
		plan = Plan{ID: id, OwnerID: owner, LocalWeekStart: week, Status: "open", Version: 1}
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// NewCommitment is one thing a rep undertakes.
type NewCommitment struct {
	Label            string
	LinkedRecordType string
	LinkedRecordID   ids.UUID
	DueOn            *time.Time
}

// AddCommitment writes one commitment onto the caller's own plan.
//
// The plan is resolved from the caller, never from the request: a plan id in a
// body would be a way to write onto somebody else's week, and there is no
// reason for one to exist.
func (s *Store) AddCommitment(ctx context.Context, now time.Time, in NewCommitment) (Commitment, error) {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return Commitment{}, err
	}
	label, err := bounded(fieldLabel, in.Label, labelBound)
	if err != nil {
		return Commitment{}, err
	}
	if label == "" {
		return Commitment{}, &values.ParseError{
			Field: fieldLabel, Code: codeRequired, Message: "a commitment needs saying in words",
		}
	}
	if err := checkLink(in.LinkedRecordType, in.LinkedRecordID); err != nil {
		return Commitment{}, err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return Commitment{}, err
	}
	var out Commitment
	err = database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		plan, err := s.openPlanTx(ctx, tx, owner, now)
		if err != nil {
			return err
		}
		// Inside the transaction, because visibility is a row read: a grant
		// revoked between the probe and the insert must not leave the link
		// written.
		if err := ensureLinkVisible(ctx, tx, in.LinkedRecordType, in.LinkedRecordID); err != nil {
			return err
		}
		if len(plan.Commitments) >= planCap {
			return &values.ParseError{
				Field: "label", Code: "plan_full",
				Message: fmt.Sprintf("a week holds at most %d commitments", planCap),
			}
		}
		capturedBy, err := storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
		var linkType, linkID any
		if in.LinkedRecordType != "" {
			linkType, linkID = in.LinkedRecordType, in.LinkedRecordID
		}
		out = Commitment{
			Label: label, LinkedRecordType: in.LinkedRecordType,
			LinkedRecordID: in.LinkedRecordID, DueOn: in.DueOn,
			State: StateOpen, Position: len(plan.Commitments), Version: 1,
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO weekly_plan_commitment
			    (plan_id, label, linked_record_type, linked_record_id, due_on, position, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id`,
			plan.ID, label, linkType, linkID, in.DueOn, out.Position, capturedBy).
			Scan(&out.ID); err != nil {
			return fmt.Errorf("weeklyplan: writing the commitment: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", "weekly_plan_commitment", out.ID,
			map[string]any{"plan_id": plan.ID.String(), "label": label})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
			PlanId: openapi_types.UUID(plan.ID), OwnerUserId: openapi_types.UUID(owner),
			ChangedFields: []string{changedCommitments},
		})
	})
	if err != nil {
		return Commitment{}, err
	}
	return out, nil
}

// openPlanTx reads the caller's plan for the week, opening one if they have
// none. Inside the write transaction, so adding a commitment on a Monday
// morning does not fail for want of a plan the rep never explicitly started.
func (s *Store) openPlanTx(
	ctx context.Context, tx pgx.Tx, owner ids.UUID, now time.Time,
) (Plan, error) {
	week, err := s.weekStart(ctx, tx, now)
	if err != nil {
		return Plan{}, err
	}
	plan, err := readPlan(ctx, tx, owner, week)
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		capturedBy, err := storekit.CapturedBy(ctx)
		if err != nil {
			return Plan{}, err
		}
		var id ids.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO weekly_plan (owner_id, local_week_start, captured_by)
			VALUES ($1, $2, $3) RETURNING id`, owner, week, capturedBy).Scan(&id); err != nil {
			return Plan{}, fmt.Errorf("weeklyplan: opening the week: %w", err)
		}
		return Plan{ID: id, OwnerID: owner, LocalWeekStart: week, Status: "open", Version: 1}, nil
	case err != nil:
		return Plan{}, err
	}
	// A closed week is history. Editing it would move counts the review has
	// already frozen, so the two would disagree about a week that is over.
	if plan.Status != "open" {
		return Plan{}, &values.ParseError{
			Field: fieldWeek, Code: codeWeekClosed,
			Message: "that week is closed; plan the current one",
		}
	}
	return plan, nil
}

// checkLink refuses half a link.
func checkLink(recordType string, id ids.UUID) error {
	if (recordType == "") != id.IsZero() {
		return &values.ParseError{
			Field: "linked_record", Code: "incomplete",
			Message: "a linked record needs both its type and its id",
		}
	}
	if recordType == "" {
		return nil
	}
	if _, ok := linkTables[recordType]; !ok {
		return &values.ParseError{
			Field: "linked_record_type", Code: "unknown",
			Message: "a commitment links a deal, a lead, a person, an organization or a project",
		}
	}
	return nil
}

// linkTables maps a commitment's link type to the table the row lives in.
//
// A compile-time map rather than the request's own word, because the value is
// interpolated into a statement by auth.EnsureVisibleLive: linked_record_type
// arrives in a request body, and this tree formats an identifier into SQL only
// as a literal or through a catalog. The migration's CHECK constraint holds the
// same five words at the other end — two writers of one list, so a sixth type
// added there and not here is refused rather than written.
var linkTables = map[string]string{
	"deal":         "deal",
	"lead":         "lead",
	"person":       "person",
	"organization": "organization",
	"project":      "project",
}

// ensureLinkVisible refuses a link to a record the caller cannot open.
//
// It is NOT an existence gate over ordinary contacts, and claiming so would
// overstate it: all five linkable types are identity tables (auth/tableclass.go),
// workspace-readable by design, so a colleague's deal or person is already
// open to every seat. Two narrower things are being refused, and both are real.
//
// The unpromoted capture. A row a connector invented is visibility='owner' —
// the capturing user's alone until a human promotes it — and capture privacy
// does not yield to row_scope=all, so not even an admin reads one. Without this
// probe a commitment launders it: paste the id, and the rep's own plan hands
// back a row out of a colleague's inbox.
//
// The erased subject. This is the LIVE probe rather than the plain one because
// Art. 17 anonymizes a person in place and stamps archived_at while leaving
// owner_id alone, so the tombstone still satisfies its original owner's
// predicate. A plain probe answers "yes, still yours" for a record every live
// read path now refuses, and the commitment would go on naming a person the
// installation has certified destroyed.
func ensureLinkVisible(ctx context.Context, tx pgx.Tx, recordType string, id ids.UUID) error {
	if recordType == "" {
		return nil
	}
	table, ok := linkTables[recordType]
	if !ok {
		// checkLink refused this before the transaction opened; reaching here
		// means a caller skipped it, and guessing a table would be worse.
		return fmt.Errorf("weeklyplan: no table for link type %q", recordType)
	}
	return auth.EnsureVisibleLive(ctx, tx, table, id)
}
