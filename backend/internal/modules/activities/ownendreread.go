// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// vietnameseHeaderLine finds a stored body that holds one of the header labels
// mailquote reads since this task's release. It only narrows the batch, and Go
// decides each end, so the list stays as that release shipped it.
const vietnameseHeaderLine = `(^|\n)\s*\*{0,2}(Từ|Đã gửi|Ngày gửi|Ngày|Chủ đề|Tiêu đề):`

// OwnEndRereadDataTask derives the stored end again for mail written before
// mailquote read Vietnamese header lines. Until it has passed a row, a forward
// with those lines is not found by the mail it passes on.
func OwnEndRereadDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795866483_vietnamese_mail_headers", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795866483_vietnamese_mail_headers"),
		Requires: []string{"core/1795866483"},
		Steps: []datatask.Step{{
			Name: "reread_own_end", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "The stored end and the search value are derived from the body, unaudited as in activities.insertLoggedActivityRow."},
			Estimate: CountActivitiesToInspect, Batch: RereadOwnEndTx,
		}},
	}
}

// RereadOwnEndTx stores the end mailquote now derives for each mail in the
// batch that holds a Vietnamese header line. A row whose end is already right
// is not written, so a replay changes nothing.
func RereadOwnEndTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := activitiesToInspectTx(ctx, tx, in)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	mailIDs, ownEnds, err := lockedMailEndsTx(ctx, tx, `SELECT id, subject, body FROM activity
		WHERE id = ANY(@inspected) AND kind = @mail AND body IS NOT NULL AND body_own_end IS NOT NULL
		  AND restricted_at IS NULL AND body ~ @header
		ORDER BY id FOR UPDATE`, pgx.NamedArgs{argInspected: page, "mail": mailquote.MailKind, "header": vietnameseHeaderLine})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	var changed int
	if err := tx.QueryRow(ctx, `SELECT public.reread_activity_own_end(@mails::uuid[], @ends::int[])`,
		pgx.NamedArgs{"mails": mailIDs, "ends": ownEnds}).Scan(&changed); err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: storing the reread mail ends: %w", err)
	}
	return datatask.AdvanceUUID(in, page, changed)
}
