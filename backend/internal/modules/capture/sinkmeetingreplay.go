// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ReplayMeeting applies the normal capture gates and roster updates to an
// existing meeting, without creating a meeting beyond the capture window.
func (s *Sink) ReplayMeeting(ctx context.Context, rec connector.NormalizedRecord) error {
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != meetingKind {
		return errors.New("capture: a calendar replay needs a meeting record")
	}
	if err := admitCalendarVerb(ctx, rec.NaturalKey, "replay"); err != nil {
		return err
	}
	_, err := s.upsert(ctx, rec, true)
	return err
}

func (s *Sink) prepareActivityCapture(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, existingOnly bool) error {
	if _, activity := rec.Fields.(ActivityFields); !activity {
		if existingOnly {
			return errors.New("capture: an existing-only replay needs an activity")
		}
		return nil
	}
	if err := s.takeMergeLockFirst(ctx, tx, rec); err != nil {
		return err
	}
	if existingOnly {
		allowed, err := s.meetingReplayAllowed(ctx, tx, rec)
		if err != nil {
			return err
		}
		if !allowed {
			return connector.ErrSkip
		}
	}
	return nil
}

func (s *Sink) meetingReplayAllowed(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (bool, error) {
	_, found, err := s.meetingForSeat(ctx, tx, rec)
	return found, err
}

func (s *Sink) meetingForSeat(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord) (ids.ActivityID, bool, error) {
	matched, found, err := s.activityHoldingIdentity(ctx, tx, rec)
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	var own ids.ActivityID
	var live bool
	err = tx.QueryRow(ctx, `SELECT id,kind='meeting' AND archived_at IS NULL AND restricted_at IS NULL
 FROM activity WHERE source_system=@source AND source_id=@key FOR UPDATE`, pgx.StrictNamedArgs{
		"source": rec.NaturalKey.SourceSystem, "key": rec.NaturalKey.SourceID,
	}).Scan(&own, &live)
	if err == nil {
		if !live {
			return ids.ActivityID{}, false, nil
		}
		held, err := SeatHoldsActivityTx(ctx, tx, own)
		return own, held, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, fmt.Errorf("capture: finding the meeting to replay: %w", err)
	}
	if !found {
		return ids.ActivityID{}, false, nil
	}
	err = tx.QueryRow(ctx, `SELECT id FROM activity WHERE id=@id AND kind='meeting'
 AND archived_at IS NULL AND restricted_at IS NULL FOR UPDATE`, pgx.StrictNamedArgs{"id": matched}).Scan(&own)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: locking the matched meeting to replay: %w", err)
	}
	held, err := SeatHoldsActivityTx(ctx, tx, own)
	return own, held, err
}
