// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const fieldAuthor = "author_id"

// MailDraftsWrittenBy counts the saved drafts author wrote. The count says
// nothing of what a draft holds, so the handover preview may show it.
func (s *Store) MailDraftsWrittenBy(ctx context.Context, tx pgx.Tx, author ids.UserID) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM mail_draft WHERE author_id = $1`, author).Scan(&n); err != nil {
		return 0, fmt.Errorf("mail draft: counting a member's drafts: %w", err)
	}
	return n, nil
}

// HandOverMailDraftsTx gives every saved draft from wrote to to, who reads it
// from then on as their own, and answers how many moved. Where to already
// holds a draft on the same anchor, theirs stays and from's is discarded,
// because a draft is unique per author and anchor.
//
// A caller without the update grant on activities moves none, as for tasks, so
// the rest of the handover still lands.
func (s *Store) HandOverMailDraftsTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, nilIfDenied(err)
	}
	if _, err := deleteDrafts(ctx, tx, `author_id = $1 AND EXISTS (
		SELECT 1 FROM mail_draft held
		 WHERE held.author_id = $2 AND held.anchor_type = mail_draft.anchor_type
		   AND held.anchor_id = mail_draft.anchor_id)`, []any{from, to}); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `UPDATE mail_draft SET author_id = $2, version = version + 1
		WHERE author_id = $1 RETURNING `+mailDraftColumns, from, to)
	if err != nil {
		return 0, fmt.Errorf("mail draft: handing over: %w", err)
	}
	moved, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailDraft, error) {
		return scanMailDraft(row)
	})
	if err != nil {
		return 0, err
	}
	for _, draft := range moved {
		before := map[string]any{fieldAuthor: from, fieldVersion: draft.Version - 1}
		after := draftImage(draft)
		after[fieldAuthor] = to
		if _, err := storekit.Audit(ctx, tx, "update", entityMailDraft, draft.ID, before, after); err != nil {
			return 0, err
		}
	}
	return len(moved), nil
}
