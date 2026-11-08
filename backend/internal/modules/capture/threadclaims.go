// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A held neighbour proves membership in one message, not every message its
// untrusted References header may have filed under the same key.
func (s *Sink) seatClaimsThreadKeysTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, keys []string) ([]string, error) {
	type unclaimed struct {
		ID  ids.ActivityID
		Key string
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.thread_key FROM activity a
  WHERE a.thread_key = ANY(@keys::text[])
    AND (a.archived_at IS NULL OR EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = a.id))
    AND a.captured_by IS DISTINCT FROM @asserted
    AND NOT EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = a.id AND i.user_id = @seat)`,
		pgx.NamedArgs{"keys": keys, "asserted": assertedProvenancePrefix + seat.String(), "seat": seat})
	if err != nil {
		return nil, fmt.Errorf("capture: checking thread membership: %w", err)
	}
	missing, err := pgx.CollectRows(rows, pgx.RowToStructByPos[unclaimed])
	if err != nil {
		return nil, fmt.Errorf("capture: reading thread membership: %w", err)
	}
	candidates := make([]ids.ActivityID, len(missing))
	for i, message := range missing {
		candidates[i] = message.ID
	}
	var sent []ids.ActivityID
	if len(candidates) > 0 && s.threadJoin.SentBySeat != nil {
		sent, err = s.threadJoin.SentBySeat(ctx, tx, seat, candidates)
		if err != nil {
			return nil, err
		}
	}
	blocked := map[string]bool{}
	for _, message := range missing {
		if !slices.Contains(sent, message.ID) {
			blocked[message.Key] = true
		}
	}
	return slices.DeleteFunc(slices.Clone(keys), func(key string) bool { return blocked[key] }), nil
}

func (s *Sink) admittedNewThreadKeyTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, proposed, own string) (string, error) {
	claimed, err := s.seatClaimsThreadKeysTx(ctx, tx, seat, []string{proposed, own})
	if err != nil {
		return "", err
	}
	if slices.Contains(claimed, proposed) {
		return proposed, nil
	}
	if slices.Contains(claimed, own) {
		return own, nil
	}
	// A sender may choose a Message-ID another thread already uses as a root.
	// A fresh internal key cannot be steered by another header naming that root.
	separate := ids.NewV7().String() + "@isolated.invalid"
	if err := storekit.LockThreadKeysTx(ctx, tx, separate); err != nil {
		return "", err
	}
	return separate, nil
}
