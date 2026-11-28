// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pendingforward"
)

// ErrActivityNotHeld answers a filing of a held forward that was archived, erased
// or filed already when the ensure took its lock. Nothing was written.
var ErrActivityNotHeld = errors.New("contacts: the forward is no longer held")

// EnsureHeldChannelCounterparty is EnsureChannelCounterparty for an activity that
// must still be a held forward. The check runs under the activity's row lock, in
// the transaction that links it, so a purge cannot slip between them.
func (s *Store) EnsureHeldChannelCounterparty(ctx context.Context, in EnsureChannelCounterpartyInput) (EnsureChannelCounterpartyResult, error) {
	if in.Identity.Provider == "" || in.Identity.ChannelUserID == "" {
		return EnsureChannelCounterpartyResult{}, errors.New("contacts: a channel counterparty needs both a provider and a channel user id")
	}
	in.CorroboratingEmail = normalizeEmail(in.CorroboratingEmail)
	var res EnsureChannelCounterpartyResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The subject first, as the eraser takes it, and again inside the ensure.
		if err := storekit.LockSubjectKeys(ctx, tx, channelKeysOf(in.Identity), correspondingEmails(in.CorroboratingEmail)); err != nil {
			return err
		}
		if err := stampHeldChannelAuthor(ctx, tx, in); err != nil {
			return err
		}
		var err error
		res, err = s.ensureChannelCounterpartyTx(ctx, tx, in)
		return err
	})
	if err != nil {
		return EnsureChannelCounterpartyResult{}, err
	}
	return res, nil
}

// The answer supplies the author's account only after the forward was captured.
func stampHeldChannelAuthor(ctx context.Context, tx pgx.Tx, in EnsureChannelCounterpartyInput) error {
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, in.ActivityID.UUID); err != nil {
		return err
	}
	held, err := lockHeldForward(ctx, tx, in.ActivityID)
	if err != nil {
		return err
	}
	if !held {
		return ErrActivityNotHeld
	}
	_, err = tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, channel_user_id, role)
		SELECT a.id, @account, 'from' FROM activity a
		 WHERE a.id = @held_activity_id AND `+pendingforward.Predicate("a")+`
		   AND NOT EXISTS (SELECT 1 FROM activity_participant p
		        WHERE p.activity_id = a.id AND p.role = 'from')
		ON CONFLICT DO NOTHING`, pgx.NamedArgs{"held_activity_id": in.ActivityID, "account": in.Identity.ChannelUserID})
	if err != nil {
		return fmt.Errorf("recording the author of the forwarded message: %w", err)
	}
	return nil
}

// lockHeldForward locks the activity row and says it is still a forward held
// for its member. A purge that began first holds the lock, so this waits for it
// and then sees the row gone.
func lockHeldForward(ctx context.Context, tx pgx.Tx, activity ids.ActivityID) (bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `SELECT a.id FROM activity a WHERE a.id = $1 AND `+pendingforward.Predicate("a")+` FOR UPDATE OF a`, activity).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: locking a held forward: %w", err)
	}
	return true, nil
}
