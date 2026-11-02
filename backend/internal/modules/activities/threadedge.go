// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ThreadEdgeAuthorizedTx proves common ownership without reading message
// content; repair must retain this evidence while the conversation is hidden.
func ThreadEdgeAuthorizedTx(ctx context.Context, tx pgx.Tx, child, parent ids.ActivityID) (bool, error) {
	var common bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_import c
 JOIN capture_import p ON p.user_id=c.user_id
 JOIN activity ca ON ca.id=c.activity_id JOIN activity pa ON pa.id=p.activity_id
 WHERE c.activity_id=@child AND p.activity_id=@parent AND ca.archived_at IS NULL AND pa.archived_at IS NULL)`, pgx.NamedArgs{"child": child, "parent": parent}).Scan(&common); err != nil {
		return false, fmt.Errorf("activities: proving a reply's shared mailbox: %w", err)
	}
	if common {
		return true, nil
	}
	for _, pair := range [][2]ids.ActivityID{{child, parent}, {parent, child}} {
		proved, err := threadSentEdgeTx(ctx, tx, pair[0], pair[1])
		if err != nil || proved {
			return proved, err
		}
	}
	return false, nil
}

func threadSentEdgeTx(ctx context.Context, tx pgx.Tx, sent, held ids.ActivityID) (bool, error) {
	var stamp string
	err := tx.QueryRow(ctx, `SELECT captured_by FROM activity WHERE id=@sent
 AND kind='email' AND direction='outbound' AND source='manual' AND source_system=@source
 AND archived_at IS NULL`, pgx.NamedArgs{"sent": sent, ledgerSourceKey: connector.EmailSourceSystem}).Scan(&stamp)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading a sent reply's ownership: %w", err)
	}
	human := actingHumanOf(stamp)
	if human == "" {
		return false, nil
	}
	var proved bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM capture_import ci
 JOIN activity a ON a.id=ci.activity_id WHERE ci.activity_id=@held_activity AND ci.user_id=@human::uuid
 AND a.archived_at IS NULL)`, pgx.NamedArgs{"held_activity": held, "human": human}).Scan(&proved); err != nil {
		return false, fmt.Errorf("activities: proving the recipient mailbox of a sent reply: %w", err)
	}
	return proved, nil
}
