// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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

// HeldMove is a checked move of a held message to a later moment. The caller
// writes it with MoveHeldTx in its own transaction, beside the decision it
// records, so a cancel cannot slip between the two.
type HeldMove struct {
	id      ids.UUID
	sched   SendSchedule
	current ScheduledSend
	timer   ScheduleTimer
}

// PlanHeldMove runs every check a move of this held message runs, and writes
// nothing: the grant, the moment, and that the row is still held.
func (s *Store) PlanHeldMove(ctx context.Context, id ids.UUID, sched SendSchedule, timer ScheduleTimer) (HeldMove, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return HeldMove{}, err
	}
	if timer == nil {
		return HeldMove{}, errNoScheduleTimer
	}
	sched, err := s.CheckLater(sched)
	if err != nil {
		return HeldMove{}, err
	}
	current, err := s.GetScheduledSend(ctx, id)
	if err != nil {
		return HeldMove{}, err
	}
	if current.Status != ScheduledStatusHeld {
		return HeldMove{}, apperrors.ErrNotFound
	}
	if err := mayReleaseHeld(ctx); err != nil {
		return HeldMove{}, err
	}
	return HeldMove{id: id, sched: sched, current: current, timer: timer}, nil
}

// MoveHeldTx writes a planned move. The version it was planned at must still
// stand, so a message cancelled or moved since refuses the whole transaction.
func (s *Store) MoveHeldTx(ctx context.Context, tx pgx.Tx, move HeldMove) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	return s.RescheduleInTx(ctx, tx, move.id, move.sched, move.current.Version, move.current, move.timer)
}

// ScheduledSendWire is one message as the scheduled list renders it.
func (s *Store) ScheduledSendWire(ctx context.Context, id ids.UUID) (crmcontracts.ScheduledSend, error) {
	row, err := s.GetScheduledSend(ctx, id)
	if err != nil {
		return crmcontracts.ScheduledSend{}, err
	}
	return scheduledSendResponse(row), nil
}
