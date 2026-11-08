// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a provider-side deletion may destroy.
//
// Its own file rather than a third selector beside the other two, because the
// question is a different one. An exclusion rule and the personal-mail window
// both ask "which of this seat's mail matches a standing decision"; this asks
// about ONE message the owner has just acted on, by the key it was captured
// under. The buckets it sorts into are the same, which is the point — a
// deletion answers to a colleague's claim and the statutory floor exactly as
// every other purge does.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SelectRemovedPurgeTx matches the current mailbox's provider id. Colleague
// copies and statutory duties shield the captured activity from destruction.
func SelectRemovedPurgeTx(
	ctx context.Context, tx pgx.Tx, user ids.UUID, provider, providerMessageID string, floor StatutoryFloor,
) (PurgeSubject, error) {
	var subject PurgeSubject
	if user == ids.Nil || provider == "" || providerMessageID == "" {
		return subject, nil
	}
	var args []any
	arg := func(value any) string { args = append(args, value); return fmt.Sprintf("$%d", len(args)) }
	seatParam, providerParam, messageParam := arg(user), arg(provider), arg(providerMessageID)
	keys := providerMessageKeysSQL(seatParam, providerParam, messageParam)
	// Another provider copy keeps the activity. Consume only the removed alias.
	_, err := tx.Exec(ctx, `UPDATE capture_import i SET provider_message_ids = array_remove(i.provider_message_ids, (`+keys+`)[1])
        WHERE i.user_id = `+seatParam+` AND i.provider_message_ids @> `+keys+`
          AND cardinality(`+keys+`) > 0
 AND EXISTS (SELECT 1 FROM unnest(i.provider_message_ids) alias
 JOIN capture_connection c ON c.user_id = i.user_id AND c.archived_at IS NULL
 AND c.status IN ('connected','error')
 AND starts_with(alias, c.provider || ':' || c.generation || ':')
 WHERE alias <> (`+keys+`)[1])`, args...)
	if err != nil {
		return subject, fmt.Errorf("capture: removing a mailbox message alias: %w", err)
	}
	shielded, args := floor.column(len(args), args)
	rows, err := tx.Query(ctx, `
        SELECT a.id,
               `+withheldReason(shielded, floor.shieldedAs())+` AS withheld,
               `+importersOfActivity+` AS importers
          FROM activity a
          JOIN capture_import i ON i.activity_id = a.id AND i.user_id = `+seatParam+`
         WHERE i.provider_message_ids @> `+keys+` AND cardinality(`+keys+`) > 0
 AND a.sent_by_colleague IS NULL
 AND (starts_with(a.captured_by, 'connector:') OR a.captured_by = 'agent:capture_counterparty_verdict')
         ORDER BY a.id`, args...)
	if err != nil {
		return subject, fmt.Errorf("capture: selecting what a mailbox-side deletion would destroy: %w", err)
	}
	if err := collectPurgeRows(rows, &subject, "selecting what a mailbox-side deletion would destroy"); err != nil {
		return subject, err
	}
	return subject, nil
}
