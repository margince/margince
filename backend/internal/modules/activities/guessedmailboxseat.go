// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditFieldMailboxSeats names the seats a repair took off a message as its
// mailbox owner, in the audit images.
const auditFieldMailboxSeats = "mailbox_seats"

// guessedMailboxSeatSQL holds for participant p of activity a when p is an
// our-side row on mail whose bare `connector:<provider>` stamp names no mailbox,
// and nothing on record proves the seat's mailbox held it. That is the row the
// participant backfill once wrote for the provider's only connection.
//
// Proof is any of: the capture ledger recorded that seat's mailbox delivering
// it, the connector that created it acted for that seat, or the seat's proven
// mailbox address is on the message. A row a human has acted on is never
// touched, and neither is a row under a hold. The bind func numbers each value.
func guessedMailboxSeatSQL(identities string, bind func(any) int) string {
	return fmt.Sprintf(`a.captured_by LIKE 'connector:%%' AND %[1]s IS NULL
	   AND a.restricted_at IS NULL
	   AND p.user_id IS NOT NULL AND p.contact_id IS NULL AND p.address IS NULL AND p.channel_user_id IS NULL
	   AND p.role = CASE WHEN a.direction = 'inbound' THEN 'to' ELSE 'from' END
	   AND NOT EXISTS (SELECT 1 FROM capture_import ci WHERE ci.activity_id = a.id AND ci.user_id = p.user_id)
	   AND NOT EXISTS (SELECT 1 FROM audit_log made
	        WHERE made.entity_type = 'activity' AND made.entity_id = a.id
	          AND made.actor_type = $%[2]d AND made.on_behalf_of = p.user_id)
	   AND NOT EXISTS (SELECT 1 FROM audit_log worked
	        WHERE worked.entity_type = 'activity' AND worked.entity_id = a.id AND worked.actor_type = $%[3]d)
	   AND NOT EXISTS (SELECT 1 FROM activity_participant seen
	        WHERE seen.activity_id = a.id AND seen.address IS NOT NULL
	          AND (%[4]s -> p.user_id::text) ? lower(seen.address))`,
		storekit.ConnectorSeatSQL("a"), bind(string(principal.PrincipalConnector)), bind(string(principal.PrincipalHuman)), identities)
}

// guessedMailboxQuery renders one statement around guessedMailboxSeatSQL. The
// statement's own leading values come first in args.
func guessedMailboxQuery(ctx context.Context, q storekit.MailboxReader, statement func(guessed string) string, leading ...any) (string, []any, error) {
	args := leading
	bind := func(v any) int { args = append(args, v); return len(args) }
	identities, err := storekit.ProvedMailSeatIdentitiesSQL(ctx, q, bind)
	if err != nil {
		return "", nil, err
	}
	return statement(guessedMailboxSeatSQL(identities, bind)), args, nil
}

// GuessedMailboxActivitiesTx locks the next limit activities after the cursor,
// in id order, that carry a guessed mailbox owner.
func GuessedMailboxActivitiesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	query, args, err := guessedMailboxQuery(ctx, tx, func(guessed string) string {
		return `SELECT a.id FROM activity a WHERE a.id > $1
		   AND EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND ` + guessed + `)
		 ORDER BY a.id LIMIT $2 FOR UPDATE OF a`
	}, after, limit)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading mail with a guessed mailbox owner: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("activities: reading mail with a guessed mailbox owner: %w", err)
	}
	return found, nil
}

// CountGuessedMailboxActivities counts the activities carrying a guessed
// mailbox owner, reading at most maximum+1. It is the repair's preview.
func CountGuessedMailboxActivities(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	query, args, err := guessedMailboxQuery(ctx, q, func(guessed string) string {
		return `SELECT count(*) FROM (SELECT 1 FROM activity a
		   WHERE EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND ` + guessed + `)
		 LIMIT $1) guessed`
	}, maximum+1)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := q.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("activities: counting mail with a guessed mailbox owner: %w", err)
	}
	return count, nil
}

// RetireGuessedMailboxSeatsTx takes the guessed mailbox owners off one
// activity, so the message stays unowned. The message, its stored original and
// its other participants stay. The rule is re-read under the row lock, so a
// replay changes nothing.
func RetireGuessedMailboxSeatsTx(ctx context.Context, tx pgx.Tx, activityID, runID ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID, storekit.IncludeArchived); err != nil {
		return false, err
	}
	query, args, err := guessedMailboxQuery(ctx, tx, func(guessed string) string {
		return `DELETE FROM activity_participant p USING activity a
		 WHERE a.id = $1 AND p.activity_id = a.id AND ` + guessed + `
		 RETURNING p.user_id`
	}, activityID)
	if err != nil {
		return false, err
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return false, fmt.Errorf("activities: removing the guessed mailbox owner of %s: %w", activityID, err)
	}
	seats, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return false, fmt.Errorf("activities: removing the guessed mailbox owner of %s: %w", activityID, err)
	}
	if len(seats) == 0 {
		return false, nil
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", "activity", activityID,
		map[string]any{auditFieldMailboxSeats: seats}, map[string]any{auditFieldMailboxSeats: []ids.UUID{}},
		map[string]any{"data_task_run": runID.String()})
	if err != nil {
		return false, err
	}
	// The interaction graph refolds an activity's participants on this event.
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID, crmcontracts.PublicEventActivityUpdated{}); err != nil {
		return false, err
	}
	return true, nil
}
