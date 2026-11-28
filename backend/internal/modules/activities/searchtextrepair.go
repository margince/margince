// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/mailquote"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// SearchTextDataTask rebuilds the search value of the activities stored before
// search read the sender's own text. Until it has passed a row, that row is
// still found through the mail it quotes.
func SearchTextDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795596207_mail_search_text", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795596207_mail_search_text"),
		Requires: []string{"core/1795596207"},
		Steps: []datatask.Step{{
			Name: "rebuild_search_text", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "The stored end and the search value are derived from the body, unaudited as in activities.insertLoggedActivityRow."},
			Estimate: CountActivitiesToInspect, Batch: RebuildSearchTextTx,
		}},
	}
}

// argInspected names the page of activity ids a data task batch inspects.
const argInspected = "inspected"

// storedMail is a mail read under its row lock, so the end computed from the
// body is written beside that same body.
type storedMail struct {
	ID      ids.UUID
	Subject *string
	Body    string
}

// RebuildSearchTextTx stores where the sender's own text ends for each mail in
// the batch that has none, then rebuilds the search values that differ. A row
// already right is not written, so a replay changes nothing.
func RebuildSearchTextTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := activitiesToInspectTx(ctx, tx, in)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	mailIDs, ownEnds, err := lockedMailEndsTx(ctx, tx, `SELECT id, subject, body FROM activity
		WHERE id = ANY(@inspected) AND kind = @mail AND body IS NOT NULL AND body_own_end IS NULL AND restricted_at IS NULL
		ORDER BY id FOR UPDATE`, pgx.NamedArgs{argInspected: page, "mail": mailquote.MailKind})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	var changed int
	if err := tx.QueryRow(ctx, `SELECT public.rebuild_activity_search_text(@inspected::uuid[], @mails::uuid[], @ends::int[])`,
		pgx.NamedArgs{argInspected: page, "mails": mailIDs, "ends": ownEnds}).Scan(&changed); err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: rebuilding search text: %w", err)
	}
	return datatask.AdvanceUUID(in, page, changed)
}

// lockedMailEndsTx reads the mails a query selects under their row lock, and
// derives the end each one's body holds beside the same body.
func lockedMailEndsTx(ctx context.Context, tx pgx.Tx, query string, args pgx.NamedArgs) ([]ids.UUID, []*int, error) {
	rows, err := tx.Query(ctx, query, args)
	if err != nil {
		return nil, nil, fmt.Errorf("activities: reading stored mail bodies: %w", err)
	}
	mails, err := pgx.CollectRows(rows, pgx.RowToStructByPos[storedMail])
	if err != nil {
		return nil, nil, fmt.Errorf("activities: reading stored mail bodies: %w", err)
	}
	mailIDs, ownEnds := make([]ids.UUID, 0, len(mails)), make([]*int, 0, len(mails))
	for _, mail := range mails {
		mailIDs = append(mailIDs, mail.ID)
		ownEnds = append(ownEnds, mailquote.OwnEnd(mailquote.MailKind, deref(mail.Subject), &mail.Body))
	}
	return mailIDs, ownEnds, nil
}
