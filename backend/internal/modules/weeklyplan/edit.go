// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

// Correcting a commitment: what it says, when it is due, and what it is about.

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

// CommitmentEdit is what a rep may correct about their own commitment.
//
// Every field is a POINTER, and the distinction is the whole shape: nil means
// "leave this alone" and a set pointer means "make it this". Without it, a
// client sending only a new label would be indistinguishable from one clearing
// the due date, and the two are different requests.
//
// State is absent on purpose. Moving a commitment between open, done and
// dropped is SetState's, which ties completed_at to the state the schema's own
// CHECK binds them with. Two writers of that pair would drift.
type CommitmentEdit struct {
	Label *string
	// DueOn set to a pointer-to-nil clears the date. A commitment with no date
	// is a real state — something to do this week, not by a day — so clearing
	// has to be expressible rather than only replacing.
	DueOn **time.Time
	// The link moves as a PAIR or not at all, the same rule checkLink holds on
	// create: a type with no id names nothing, and an id with no type cannot be
	// routed to.
	LinkedRecordType *string
	LinkedRecordID   *ids.UUID
}

// EditCommitment corrects one of the caller's own commitments.
//
// A rep types a commitment on Monday and finds the typo on Tuesday. Until this
// existed the only way out was to drop the row and write a new one, which loses
// the manager's answer and any help already asked for — the two fields on the
// row that were not the rep's to throw away.
func (s *Store) EditCommitment(
	ctx context.Context, commitmentID ids.UUID, edit CommitmentEdit,
) error {
	if err := auth.Require(ctx, "weekly_plan", principal.ActionUpdate); err != nil {
		return err
	}
	if err := edit.check(); err != nil {
		return err
	}
	owner, err := planUser(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		planID, current, err := ownEditableCommitmentTx(ctx, tx, commitmentID, owner)
		if err != nil {
			return err
		}
		// Inside the transaction, like the create path: a grant revoked between
		// the probe and the write must not leave the link written.
		if edit.LinkedRecordType != nil {
			if err := ensureLinkVisible(ctx, tx, *edit.LinkedRecordType, *edit.LinkedRecordID); err != nil {
				return err
			}
		}
		patch := edit.patch(current)
		if patch.Empty() {
			// Nothing asked for differs from what is stored. Writing anyway
			// would file an audit row saying a rep changed something they did
			// not, and bump the version a concurrent reader is holding.
			return nil
		}
		lock, err := storekit.LockRow(ctx, tx, "weekly_plan_commitment",
			commitmentID, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "weekly_plan_commitment", commitmentID,
			patch.Before(), patch.After())
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, owner, crmcontracts.PublicEventWeeklyPlanUpdated{
			PlanId: openapi_types.UUID(planID), OwnerUserId: openapi_types.UUID(owner),
			ChangedFields: []string{"commitments"},
		})
	})
}

// check validates what was asked for, before any row is read.
func (e CommitmentEdit) check() error {
	if e.Label == nil && e.DueOn == nil && e.LinkedRecordType == nil {
		return &values.ParseError{
			Field: "label", Code: "required",
			Message: "an edit has to change something",
		}
	}
	if e.Label != nil {
		label, err := bounded("label", *e.Label, labelBound)
		if err != nil {
			return err
		}
		if label == "" {
			return &values.ParseError{
				Field: "label", Code: "required", Message: "a commitment needs saying in words",
			}
		}
	}
	if e.LinkedRecordType != nil {
		if e.LinkedRecordID == nil {
			return &values.ParseError{
				Field: "linked_record_id", Code: "required",
				Message: "a linked record needs both a type and an id",
			}
		}
		return checkLink(*e.LinkedRecordType, *e.LinkedRecordID)
	}
	if e.LinkedRecordID != nil {
		return &values.ParseError{
			Field: "linked_record_type", Code: "required",
			Message: "a linked record needs both a type and an id",
		}
	}
	return nil
}

// patch is the columns this edit actually MOVES.
//
// Each field is compared before it is set, because storekit.Patch records an
// assignment unconditionally — comparing is the caller's job, and every other
// writer here changes a column it already knows differs. An edit does not: a
// client may send back the label it read, and setting it anyway would file an
// audit row saying a rep changed something they did not and bump the version a
// concurrent reader is holding.
func (e CommitmentEdit) patch(current Commitment) *storekit.Patch {
	patch := storekit.NewPatch()
	if e.Label != nil {
		label, _ := bounded("label", *e.Label, labelBound)
		if label != current.Label {
			patch.Set("label", current.Label, label)
		}
	}
	if e.DueOn != nil {
		// SetDate, not Set: due_on is a `date` column, and storekit's own rule
		// is that a date goes in as its own text so the audit image round-trips
		// back into the column an undo would write it to.
		if !sameDay(current.DueOn, *e.DueOn) {
			patch.SetDate("due_on", current.DueOn, *e.DueOn)
		}
	}
	if e.LinkedRecordType != nil {
		// Both columns move together or neither does. An empty type clears the
		// pair, which is how a commitment stops being about a record without
		// being rewritten.
		nextType, nextID := *e.LinkedRecordType, *e.LinkedRecordID
		if nextType == "" {
			nextID = ids.Nil
		}
		if nextType != current.LinkedRecordType || nextID != current.LinkedRecordID {
			patch.Set("linked_record_type", stringOrNil(current.LinkedRecordType), stringOrNil(nextType))
			patch.Set("linked_record_id", uuidOrNil(current.LinkedRecordID), uuidOrNil(nextID))
		}
	}
	return patch
}

// The two nil-shapes a patch compares against. A column that is NULL in the row
// has to be compared as nil rather than as a zero value, or clearing an already
// empty link would report the empty string changing to the empty string and
// file an audit row saying a rep changed something they did not.
// sameDay compares two optional calendar days as days, which is what the column
// stores. Comparing the time.Time values would call a date read back from
// Postgres different from the one written, on the instant alone.
func sameDay(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Format(time.DateOnly) == b.Format(time.DateOnly)
}

func stringOrNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func uuidOrNil(id ids.UUID) any {
	if id == ids.Nil {
		return nil
	}
	return id
}

// ownEditableCommitmentTx reads the row's editable fields under the caller's
// ownership, refusing a closed week exactly as the settle path does.
//
// A second reader beside ownCommitmentTx rather than a wider one: that query
// selects the settle path's columns, and adding four more to it would make
// every state change read fields it does not use.
func ownEditableCommitmentTx(
	ctx context.Context, tx pgx.Tx, commitmentID, owner ids.UUID,
) (ids.UUID, Commitment, error) {
	var planID ids.UUID
	var status string
	var current Commitment
	var linkType *string
	var linkID *ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT p.id, p.status, c.label, c.due_on, c.linked_record_type, c.linked_record_id
		  FROM weekly_plan_commitment c
		  JOIN weekly_plan p ON p.id = c.plan_id
		 WHERE c.id = $1 AND p.owner_id = $2`, commitmentID, owner).
		Scan(&planID, &status, &current.Label, &current.DueOn, &linkType, &linkID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, Commitment{}, apperrors.ErrNotFound
	}
	if err != nil {
		return ids.Nil, Commitment{}, fmt.Errorf("weeklyplan: reading the commitment: %w", err)
	}
	// A closed week is frozen into a review that has already been counted.
	if status != "open" {
		return ids.Nil, Commitment{}, &values.ParseError{
			Field: "week", Code: "week_closed",
			Message: "that week is closed and its outcome is recorded",
		}
	}
	if linkType != nil {
		current.LinkedRecordType = *linkType
	}
	if linkID != nil {
		current.LinkedRecordID = *linkID
	}
	return planID, current, nil
}
