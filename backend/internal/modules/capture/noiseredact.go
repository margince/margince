// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which hidden noise mail the redaction sweep may destroy.
//
// Destroying it runs the same purge every erasure arm runs. So it is held to
// the protections of the other destructive mail selectors. Those are a legal
// hold, the statutory floor, an open subject request, and a colleague's import.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
)

// NoiseMessage is one message the redaction sweep found due, with the position
// a later page of the backlog resumes after.
type NoiseMessage struct {
	ID         ids.UUID
	OccurredAt time.Time
}

// NoiseMailToRedact lists one page of hidden mail from judged-noise senders
// whose undo window has passed and that still has content to destroy. A nil
// after starts at the oldest message; otherwise the page starts past it.
//
// The window is measured from when this message was hidden, not from the
// verdict. A sender keeps writing after a verdict, and a window keyed on the
// verdict would give later mail no undo window at all.
//
// "Still has content" includes the provider original. A message whose activity
// is already nulled while its raw_capture row survives is unfinished work. That
// makes the sweep resumable after a crash between two writes.
//
// The corroboration requirement (`a.bulk_mail_attested`) separates this from
// NoiseMailToHide. Hiding is reversible and destroying is not, so a model
// verdict alone may hide mail. Destroying it also needs the message's own
// List-Unsubscribe header, which a forger cannot plant on somebody else's mail.
// A keep_out a seat typed needs no such corroboration.
func (s *PendingStore) NoiseMailToRedact(
	ctx context.Context, window time.Duration, floor StatutoryFloor, after *NoiseMessage, limit int,
) ([]NoiseMessage, error) {
	args := []any{limit}
	page := ""
	if after != nil {
		args = append(args, after.OccurredAt, after.ID)
		page = fmt.Sprintf(`
		AND (a.occurred_at, a.id) > ($%d, $%d)`, len(args)-1, len(args))
	}
	due, args := noiseMailDestroyable(window, floor, args)
	var out []NoiseMessage
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT a.id, a.occurred_at
			  FROM activity a
			  JOIN `+disownedSenders+` ON p.email = a.counterparty_email
			 WHERE `+due+page+`
			 ORDER BY a.occurred_at, a.id
			 LIMIT $1`, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowToStructByPos[NoiseMessage])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the noise mail due for redaction: %w", err)
	}
	return out, nil
}

// ClaimNoiseMailForRedactionTx locks one message the sweep is about to destroy
// and asks again, under the lock, whether it may. A hold, a request or a
// colleague's import that arrived after the backlog was read wins over the
// sweep. A message that no longer qualifies answers false and is left alone.
func (s *PendingStore) ClaimNoiseMailForRedactionTx(
	ctx context.Context, tx pgx.Tx, id ids.UUID, window time.Duration, floor StatutoryFloor,
) (bool, error) {
	due, args := noiseMailDestroyable(window, floor, []any{id})
	var claimed ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT a.id
		  FROM activity a
		  JOIN `+disownedSenders+` ON p.email = a.counterparty_email
		 WHERE a.id = $1 AND `+due+`
		 LIMIT 1
		 FOR UPDATE OF a`, args...).Scan(&claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("capture: claiming noise mail for redaction: %w", err)
	}
	return true, nil
}

// noiseMailDestroyable is the whole predicate a message must pass to be
// destroyed, over an activity aliased `a` joined to disownedSenders. It appends
// the statutory floor's two arguments after those already in args.
//
// The protections are the ones the purge selectors apply, through the same
// fragments. withheldReason covers a hold, the floor and an open request.
// HeldThroughAnyLink covers a hold on a linked record. importersOfActivity
// covers a colleague's copy.
func noiseMailDestroyable(window time.Duration, floor StatutoryFloor, args []any) (string, []any) {
	shielded, args := floor.column(len(args), args)
	return `a.restricted_at IS NULL
		   AND ` + noiseMailScope + withinVerdictReach() + `
		   AND (NOT p.machine OR (p.resolved_at IS NOT NULL AND a.bulk_mail_attested))
		   AND a.archived_at IS NOT NULL
		   AND a.archived_at <= now() - ` + quoteInterval(window) + `
		   AND (a.subject IS NOT NULL OR a.body IS NOT NULL OR a.raw IS NOT NULL
		        OR EXISTS (
		          SELECT 1 FROM raw_capture r
		           WHERE r.source_system = a.source_system AND r.source_id = a.source_id))
		   AND NOT ` + retentionscope.HeldThroughAnyLink("a.id") + `
		   AND (` + withheldReason(shielded, floor.shieldedAs()) + `) = ''
		   AND ` + importersOfActivity + ` <= 1`, args
}
