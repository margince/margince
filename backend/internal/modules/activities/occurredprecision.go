// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// How much of an activity's time is known. A date-only entry is stored at noon
// UTC of its date. Only the precision says that noon is no fact.

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
func admitLoggedPrecision(in *LogActivityInput) error {
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
	in.OccurredAt = new(occurredday.Anchor(*in.OccurredAt))
	return nil
}

// admitPatchPrecision settles the precision a patch writes. A new time with
// none named is an instant, and a day leaves only with a new time. A day lands
// on its date's anchor: the sent time's date, else the stored instant's day.
func admitPatchPrecision(ctx context.Context, tx pgx.Tx, current crmcontracts.Activity, in *UpdateActivityInput) error {
	if in.OccurredPrecision == nil && in.OccurredAt != nil {
		in.OccurredPrecision = new(PrecisionInstant)
	}
	storedDay := current.OccurredPrecision == crmcontracts.OccurredPrecisionOccurredPrecisionDay
	if in.OccurredPrecision == nil || *in.OccurredPrecision == PrecisionInstant {
		if in.OccurredPrecision != nil && in.OccurredAt == nil && storedDay {
			return &DayToInstantNeedsTimeError{}
		}
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
	switch {
	case in.OccurredAt != nil:
		in.OccurredAt = new(occurredday.Anchor(*in.OccurredAt))
	case !storedDay:
		zone, err := resolveRecordZone(ctx, tx, in.recordZone)
		if err != nil {
			return err
		}
		in.OccurredAt = new(occurredday.AnchorIn(current.OccurredAt, zone))
	}
	return nil
}

// errNoRecordZone refuses to turn an instant into a day on a store built
// without the installation zone: the instant's day depends on it.
var errNoRecordZone = errors.New("activities: an instant becomes a day in the installation zone, and this writer has none (WithRecordZone)")

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

// DayToInstantNeedsTimeError refuses a patch that makes a day an instant with
// no time: the stored noon would then read as a real 12:00.
type DayToInstantNeedsTimeError struct{}

func (e *DayToInstantNeedsTimeError) Error() string {
	return "a date-only activity becomes an instant only with the time it happened, so send occurred_at"
}

// FieldFault names occurred_at, which is what the caller must add.
func (e *DayToInstantNeedsTimeError) FieldFault() (field, code, message string) {
	return auditFieldOccurredAt, codeRequired, e.Error()
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
