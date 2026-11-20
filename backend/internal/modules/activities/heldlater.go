// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CheckLater normalizes a moment to send at and refuses one that is invalid or
// not in the future. A caller asks before it records anything the move needs.
func (s *Store) CheckLater(sched SendSchedule) (SendSchedule, error) {
	normalized, err := normalizeSchedule(sched)
	if err != nil {
		return SendSchedule{}, err
	}
	if err := validateSchedule(normalized, s.now()); err != nil {
		return SendSchedule{}, err
	}
	if !normalized.At.After(s.now()) {
		return SendSchedule{}, &InvalidScheduleError{Field: FieldScheduledAt, Reason: "is in the past"}
	}
	return normalized, nil
}

// ScheduleHeldLater moves a held message to a later moment, at the version it
// has now. A Send anyway decision recorded on its review stays and is spent
// when the message fires. A row no longer held was settled by someone else.
func (s *Store) ScheduleHeldLater(ctx context.Context, id ids.UUID, sched SendSchedule, timer ScheduleTimer) (crmcontracts.ScheduledSend, error) {
	current, err := s.GetScheduledSend(ctx, id)
	if err != nil {
		return crmcontracts.ScheduledSend{}, err
	}
	if current.Status != ScheduledStatusHeld {
		return crmcontracts.ScheduledSend{}, apperrors.ErrVersionSkew
	}
	row, err := s.RescheduleScheduledSend(ctx, id, sched, current.Version, timer)
	if err != nil {
		return crmcontracts.ScheduledSend{}, err
	}
	return scheduledSendResponse(row), nil
}
