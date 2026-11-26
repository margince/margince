// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AttachmentCountsFor counts what came with each of these messages, in ONE
// statement over the whole page.
//
// A count per row cost a round trip per visible line, which a timeline of
// twenty turns into twenty statements and a search page of two hundred into
// two hundred. The same batching rule the email-row reader follows.
//
// It carries NO gate of its own, and must only be called with ids a
// content-gated read already admitted. The count is content: knowing that a
// contract arrived with a message is knowing something about the message, so
// the caller's own gate is what decides, exactly as readEmailAttachments is
// reached only after its parent admitted the caller.
//
// An id absent from the map had no attachments, which is a count of zero.
func AttachmentCountsFor(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]int, error) {
	if len(activityIDs) == 0 {
		return map[ids.UUID]int{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT at.entity_id, count(*)
		  FROM attachment at
		 WHERE at.entity_type = 'activity' AND at.entity_id = ANY($1) AND at.archived_at IS NULL
		   AND `+arrivedWithMessage("at")+`
		 GROUP BY at.entity_id`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("activities: counting what came with a page of messages: %w", err)
	}
	defer rows.Close()

	out := make(map[ids.UUID]int, len(activityIDs))
	for rows.Next() {
		var id ids.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("activities: scanning an attachment count: %w", err)
		}
		out[id] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: counting what came with a page of messages: %w", err)
	}
	return out, nil
}

// applyAttachmentCounts stamps the counts onto the email rows of a page.
//
// WITHHELD ROWS ARE SKIPPED, and that is the whole reason this is a separate
// pass rather than a field RowEmailSummary fills. A withheld summary returns
// early with the content stripped, and a count written over it afterwards
// would tell a colleague that a contract came with a message they may not
// read — the file list is refused to them, and the fact that files exist is
// the same fact in smaller print.
func applyAttachmentCounts(page []crmcontracts.Activity, counts map[ids.UUID]int) {
	for i := range page {
		summary := page[i].EmailSummary
		if summary == nil || summary.DisplayStatus == crmcontracts.EmailAccessStatusWithheld {
			continue
		}
		summary.AttachmentCount = counts[ids.UUID(page[i].Id)]
	}
}

// emailIDsOf answers the ids of the email rows on a page, which are the only
// ones an attachment count is asked for: a call and a note carry no email row
// to stamp, and a withheld one must not be stamped.
func emailIDsOf(page []crmcontracts.Activity) []ids.UUID {
	var out []ids.UUID
	for i := range page {
		summary := page[i].EmailSummary
		if summary == nil || summary.DisplayStatus == crmcontracts.EmailAccessStatusWithheld {
			continue
		}
		out = append(out, ids.UUID(page[i].Id))
	}
	return out
}

// AttachmentNamesFor reads wire files, never uploads added to a message later.
// A sent delivery's snapshot survives later changes to its source documents.
func AttachmentNamesFor(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID][]string, error) {
	out := map[ids.UUID][]string{}
	if len(activityIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	idsPosition := arg(activityIDs)
	rows, err := tx.Query(ctx, `SELECT a.id, delivery.id IS NOT NULL, coalesce(delivery.attachments, '[]'::jsonb),
 ARRAY(SELECT at.filename FROM attachment at WHERE at.entity_type='activity' AND at.entity_id=a.id
 AND at.archived_at IS NULL AND at.captured_by LIKE 'connector:%' AND `+arrivedWithMessage("at")+` ORDER BY at.filename,at.id)
 FROM activity a LEFT JOIN comms_outbound delivery ON delivery.activity_id=a.id AND delivery.status='sent'
 WHERE a.id=ANY($`+fmt.Sprint(idsPosition)+`) AND `+content, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the files carried by messages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var sent bool
		var files []OutboundFile
		var names []string
		if err := rows.Scan(&id, &sent, &files, &names); err != nil {
			return nil, err
		}
		if sent {
			names = make([]string, len(files))
			for i, file := range files {
				names[i] = file.Filename
			}
		}
		out[id] = names
	}
	return out, rows.Err()
}
