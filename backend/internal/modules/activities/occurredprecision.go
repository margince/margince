// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// How much of an activity's time is known. A date-only entry is stored at noon
// of its date in the record zone. Only the precision says that noon is no fact.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// The two precisions, in the column's own spelling.
const (
	PrecisionInstant = string(crmcontracts.OccurredPrecisionOccurredPrecisionInstant)
	PrecisionDay     = string(crmcontracts.OccurredPrecisionOccurredPrecisionDay)
)

const fieldOccurredPrecision = "occurred_precision"

// storedPrecision is the column value for a writer's precision; empty is an
// instant, which every writer that never heard of a date-only entry means.
func storedPrecision(precision string) string {
	if precision == "" {
		return PrecisionInstant
	}
	return precision
}

// precisionInsertFragment appends the precision column to an INSERT whose
// arguments so far are base, deriving its placeholder from their count.
func precisionInsertFragment(precision string, base []any) (col, holder string, args []any) {
	args = append(append(make([]any, 0, len(base)+1), base...), storedPrecision(precision))
	return ", occurred_precision", fmt.Sprintf(", $%d", len(args)), args
}

// admitLoggedPrecision refuses a day with no date to anchor it, and a day on a
// meeting that holds an hour. A day is stored at its date's anchor.
func admitLoggedPrecision(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if in.OccurredPrecision == "" || in.OccurredPrecision == PrecisionInstant {
		return nil
	}
	if in.OccurredPrecision != PrecisionDay {
		return &InvalidPrecisionError{}
	}
	if in.OccurredAt == nil {
		return &RequiredFieldError{Field: auditFieldOccurredAt}
	}
	if err := refuseDayHeldSlot(in.OccurredPrecision, in.ClaimsHostSlot); err != nil {
		return err
	}
	anchor, err := anchorDay(ctx, tx, in.recordZone, *in.OccurredAt)
	in.OccurredAt = &anchor
	return err
}

// admitPatchPrecision settles the precision a patch writes. A new time with
// none named is an instant. A day may not land on a meeting holding a slot,
// and lands on its date's anchor: the sent time's date, else the stored one's.
func admitPatchPrecision(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity, in *UpdateActivityInput) error {
	if in.OccurredPrecision == nil && in.OccurredAt != nil {
		in.OccurredPrecision = new(PrecisionInstant)
	}
	if in.OccurredPrecision == nil || *in.OccurredPrecision == PrecisionInstant {
		return nil
	}
	if *in.OccurredPrecision != PrecisionDay {
		return &InvalidPrecisionError{}
	}
	var holdsSlot bool
	if err := tx.QueryRow(ctx, `SELECT `+occupiesHostSlotSQL+` FROM activity a WHERE a.id = $1`, current.Id).Scan(&holdsSlot); err != nil {
		return fmt.Errorf("activities: reading whether the activity holds a slot: %w", err)
	}
	if err := refuseDayHeldSlot(*in.OccurredPrecision, holdsSlot); err != nil {
		return err
	}
	zone, err := resolveRecordZone(ctx, tx, in.recordZone)
	if err != nil {
		return err
	}
	named := occurredday.Date(current.OccurredAt, zone)
	if in.OccurredAt != nil {
		named = *in.OccurredAt
	}
	in.OccurredAt = new(occurredday.Anchor(named, zone))
	return nil
}

// errNoRecordZone refuses a day entry from a store built without a record zone.
// A guessed zone would file it under another date.
var errNoRecordZone = errors.New("activities: a date-only entry needs the record zone, and this writer has none (WithRecordZone)")

func resolveRecordZone(ctx context.Context, tx pgx.Tx, zone RecordZone) (*time.Location, error) {
	if zone == nil {
		return nil, errNoRecordZone
	}
	loc, err := zone(ctx, tx)
	if err == nil && loc == nil {
		err = errNoRecordZone
	}
	return loc, err
}

// anchorDay is where the date named's own year, month and day name is stored:
// the writer's offset decides the date, the record zone its anchor.
func anchorDay(ctx context.Context, tx pgx.Tx, zone RecordZone, named time.Time) (time.Time, error) {
	loc, err := resolveRecordZone(ctx, tx, zone)
	if err != nil {
		return time.Time{}, err
	}
	return occurredday.Anchor(named, loc), nil
}

// InvalidPrecisionError refuses a precision outside the contract's list.
type InvalidPrecisionError struct{}

func (e *InvalidPrecisionError) Error() string {
	return "occurred_precision is instant or day"
}

// FieldFault names the field to correct; the caller's value is not echoed.
func (e *InvalidPrecisionError) FieldFault() (field, code, message string) {
	return fieldOccurredPrecision, codeInvalidEnum, e.Error()
}

// DayHeldSlotError refuses a date-only meeting that holds its host's calendar
// slot: the slot guard compares hours, and a date has none to compare.
type DayHeldSlotError struct{}

func (e *DayHeldSlotError) Error() string {
	return "a booked meeting holds an hour of its host's calendar, so its time must be an instant, not a day"
}

// FieldFault names the precision, which is what the caller can change.
func (e *DayHeldSlotError) FieldFault() (field, code, message string) {
	return fieldOccurredPrecision, "day_holds_host_slot", e.Error()
}

// refuseDayHeldSlot holds a meeting's precision against its slot claim.
func refuseDayHeldSlot(precision string, holdsSlot bool) error {
	if holdsSlot && precision == PrecisionDay {
		return &DayHeldSlotError{}
	}
	return nil
}
