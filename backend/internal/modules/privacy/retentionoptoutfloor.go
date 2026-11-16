// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// A refusal of advertising and its receipt carry their own retention floor,
// kept_until, stamped by consent from the subject's messaging rules. No
// retention policy anonymizes, erases or archives a record while one stands.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// optOutRecordsKept holds while the record whose activity_link column is
// column, matched by idExpr, has a refusal or a receipt inside its floor.
func optOutRecordsKept(column, idExpr string) string {
	return `(EXISTS (SELECT 1 FROM communication_suppression ks
	          WHERE ks.` + column + ` = ` + idExpr + ` AND ks.kept_until > now())
	  OR EXISTS (SELECT 1 FROM comms_outbound ko JOIN activity_link kl ON kl.activity_id = ko.activity_id
	          WHERE kl.` + column + ` = ` + idExpr + ` AND ko.kept_until > now()))`
}

// receiptKept holds while the activity matched by activityExpr is a receipt
// inside its floor.
func receiptKept(activityExpr string) string {
	return `EXISTS (SELECT 1 FROM comms_outbound ko WHERE ko.activity_id = ` + activityExpr + ` AND ko.kept_until > now())`
}

// refuseIfOptOutKeptNow re-asks the opt-out floor for one contact or lead inside
// the acting transaction. The caller holds the subject's stop lock, so a
// refusal recorded after the pass selected the record is seen here.
func refuseIfOptOutKeptNow(ctx context.Context, tx pgx.Tx, column string, id ids.UUID) error {
	var kept bool
	if err := tx.QueryRow(ctx, `SELECT `+optOutRecordsKept(column, "$1"), id).Scan(&kept); err != nil {
		return fmt.Errorf("privacy: re-checking the opt-out floor before acting on a record: %w", err)
	}
	if kept {
		return errKeptByFloor
	}
	return nil
}
