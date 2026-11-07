// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	requestSystemArgument = "system"
	requestSourceArgument = "source"
)

// Recovery creates only the accepting reader's task; the private machine artifact remains history.
func prepareRequestAcceptance(ctx context.Context, tx pgx.Tx, request *LogActivityInput, userID ids.UUID, pending bool) error {
	sourceID := *request.SourceActivityID
	acceptedKey := sourceID.String() + "/accepted/" + userID.String()
	var accepted bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity WHERE source_system=@system AND source_id=@key)`, pgx.NamedArgs{requestSystemArgument: EmailRequestTaskSource, "key": acceptedKey}).Scan(&accepted); err != nil {
		return err
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM activity sibling WHERE sibling.source_system=@system
 AND sibling.source_activity_id=@source AND sibling.assignee_id IS DISTINCT FROM @reader
 AND `+storekit.HeldTaskClause("sibling")+`)`, pgx.NamedArgs{requestSystemArgument: EmailRequestTaskSource, requestSourceArgument: sourceID, "reader": userID}).Scan(&held); err != nil {
		return err
	}
	if held {
		return &requestHeldError{}
	}
	if accepted {
		request.SourceID = &acceptedKey
		return nil
	}
	if !pending {
		return nil
	}
	var original ids.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM activity WHERE source_system=@system AND source_id=@key`, pgx.NamedArgs{requestSystemArgument: EmailRequestTaskSource, "key": *request.SourceID}).Scan(&original)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", original, storekit.IncludeArchived); err != nil {
		return err
	}
	recoverable, err := requestAvailableForAcceptance(ctx, tx, original, userID)
	if err != nil {
		return err
	}
	if recoverable {
		request.SourceID = &acceptedKey
	}
	return nil
}

func requestAvailableForAcceptance(ctx context.Context, tx pgx.Tx, original, reader ids.UUID) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return false, err
	}
	readerSQL := fmt.Sprintf("$%d::uuid", arg(reader))
	recipient := storekit.DirectMailRecipientSQL("source", storekit.MailSeatAddressesSQL(identities, readerSQL))
	var recoverable bool
	query := fmt.Sprintf(`SELECT coalesce(%s AND %s, false)
 FROM activity reminder JOIN activity source ON source.id=reminder.source_activity_id
 WHERE reminder.id=$%d AND reminder.restricted_at IS NULL AND source.archived_at IS NULL AND source.restricted_at IS NULL`, `reminder.assignee_id IS DISTINCT FROM `+readerSQL+` AND reminder.kind='task' AND NOT reminder.is_done AND NOT `+storekit.HeldTaskClause("reminder")+` AND EXISTS(SELECT 1 FROM audit_log creation WHERE creation.entity_type='activity' AND creation.entity_id=reminder.id AND creation.action='create')`, recipient, arg(original))
	err = tx.QueryRow(ctx, query, args...).Scan(&recoverable)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return recoverable, err
}
