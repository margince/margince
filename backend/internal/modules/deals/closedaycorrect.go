// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A user's correction of the day a closed deal closed.
//
// closed_at is the one close day every report reads, and an imported win can
// carry the import day instead of the real one. Only a human may move it: the
// stage advance stamps it from the clock and no other door touches it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const closedOnField = "closed_on"

// CorrectCloseDayInput is the day a closed deal is said to have closed.
type CorrectCloseDayInput struct {
	ClosedOn  time.Time
	IfVersion *int64
}

// CloseDayOpenDealError maps to 422: an open deal has no close day to correct.
type CloseDayOpenDealError struct{}

func (e *CloseDayOpenDealError) Error() string {
	return "only a won or lost deal has a close day; an open deal's forecast date is expected_close_date"
}

// FieldFault refuses a correction on a deal that has not closed.
func (e *CloseDayOpenDealError) FieldFault() (field, code, message string) {
	return closedOnField, "deal_not_closed", e.Error()
}

// CloseDayFutureError maps to 422: a deal cannot have closed after today.
type CloseDayFutureError struct{}

func (e *CloseDayFutureError) Error() string {
	return "a deal cannot have closed after today; pick today or an earlier day"
}

// FieldFault refuses a close day later than today in the installation's zone.
func (e *CloseDayFutureError) FieldFault() (field, code, message string) {
	return closedOnField, "close_day_future", e.Error()
}

// CorrectCloseDay moves a closed deal's closed_at to midday of the given day in
// the installation's zone, and freezes the exchange rate again for that day.
//
// Midday keeps the instant on the named day in every zone within half a day of
// the installation's. The deal screen draws it in that reading.
func (s *Store) CorrectCloseDay(ctx context.Context, id ids.DealID, in CorrectCloseDayInput) (crmcontracts.Deal, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.Deal{}, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return crmcontracts.Deal{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	var out crmcontracts.Deal
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.correctCloseDayInTx(ctx, tx, id, in, active)
		return err
	})
	return out, err
}

func (s *Store) correctCloseDayInTx(ctx context.Context, tx pgx.Tx, id ids.DealID,
	in CorrectCloseDayInput, active []fieldcatalog.Column,
) (crmcontracts.Deal, error) {
	if _, err := storekit.LockRow(ctx, tx, dealTable, id.UUID, storekit.LiveOnly); err != nil {
		return crmcontracts.Deal{}, err
	}
	if err := auth.EnsureWritable(ctx, tx, dealTable, id.UUID); err != nil {
		return crmcontracts.Deal{}, err
	}
	current, err := readDeal(ctx, tx, id, storekit.LiveOnly, nil)
	if err != nil {
		return crmcontracts.Deal{}, fmt.Errorf("read deal before correcting its close day: %w", err)
	}
	if string(current.Status) == string(DealOpen) || current.ClosedAt == nil {
		return crmcontracts.Deal{}, &CloseDayOpenDealError{}
	}
	tzName, err := s.installation.Timezone(ctx, tx)
	if err != nil {
		return crmcontracts.Deal{}, fmt.Errorf("resolve the installation's timezone: %w", err)
	}
	zone, err := installationZone(tzName)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	day := dateOnly(in.ClosedOn)
	today, err := s.installationToday(ctx, tx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	if day.After(today) {
		return crmcontracts.Deal{}, &CloseDayFutureError{}
	}
	if err := storekit.EnsureIfMatch(ctx, tx, dealTable, id.UUID, in.IfVersion); err != nil {
		return crmcontracts.Deal{}, err
	}
	if storekit.WorkspaceDay(*current.ClosedAt, zone).Equal(day) {
		return readDealForCaller(ctx, tx, id, storekit.LiveOnly, active)
	}

	closedAt := time.Date(day.Year(), day.Month(), day.Day(), 12, 0, 0, 0, zone).UTC()
	p := storekit.NewPatch()
	p.Set("closed_at", current.ClosedAt, closedAt)
	if current.AmountMinor != nil && current.Currency != nil {
		if err := s.freezeBaseRate(ctx, tx, p, current.Id, string(*current.Currency),
			*current.AmountMinor, closedAt); err != nil {
			return crmcontracts.Deal{}, fmt.Errorf("re-freeze fx at the corrected close day: %w", err)
		}
	}
	if err := applyDealPatchGuarded(ctx, tx, id, p, in.IfVersion); err != nil {
		return crmcontracts.Deal{}, fmt.Errorf("apply close day correction: %w", err)
	}
	if err := recordDealUpdate(ctx, tx, id, current, UpdateDealInput{}, p); err != nil {
		return crmcontracts.Deal{}, err
	}
	return readDealForCaller(ctx, tx, id, storekit.LiveOnly, active)
}
