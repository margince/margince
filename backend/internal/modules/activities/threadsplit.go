// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const threadFromArg = "from"

// ThreadSplitActivityTx moves one message; the repair barrier prevents readers
// from observing a conversation before all its bounded moves have committed.
func ThreadSplitActivityTx(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, from, to string) (bool, error) {
	if from == "" || to == "" || from == to {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `UPDATE activity SET thread_key=@to
 WHERE id=@id AND thread_key=@from`, pgx.NamedArgs{"id": activityID, threadFromArg: from, "to": to})
	if err != nil {
		return false, fmt.Errorf("activities: moving a message to its conversation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	before, after := map[string]string{fieldThreadKey: from}, map[string]string{fieldThreadKey: to}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", activityID.UUID, before, after)
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{ThreadKey: &to},
	}); err != nil {
		return false, err
	}
	// Sales judgements remain on the source: nobody judged the new conversation.
	return true, nil
}

// ThreadPublishedTx wakes readers after the conversation becomes available;
// subscribers may have processed its move while the repair barrier still stood.
func ThreadPublishedTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, key string) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	var version int64
	err := tx.QueryRow(ctx, `UPDATE activity SET version=version+1,updated_at=now()
 WHERE id=@id AND thread_key=@thread_key AND archived_at IS NULL AND restricted_at IS NULL
 AND activity_thread_available(thread_key) RETURNING version`, pgx.NamedArgs{"id": id, fieldThreadKey: key}).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", id.UUID, map[string]int64{fieldVersion: version - 1}, map[string]int64{fieldVersion: version})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{ThreadKey: &key},
	})
}
