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
		held, err := lockHeldForward(ctx, tx, in.ActivityID)
		if err != nil {
			return err
		}
		if !held {
			return ErrActivityNotHeld
		}
		res, err = s.ensureChannelCounterpartyTx(ctx, tx, in)
		return err
	})
	if err != nil {
		return EnsureChannelCounterpartyResult{}, err
	}
	return res, nil
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
