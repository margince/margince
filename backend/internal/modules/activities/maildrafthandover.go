// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const fieldAuthor = "author_id"

// draftFreeOfLiveSend keeps a draft that a scheduled or held message still
// names. That message discards the draft from its author when it goes, so the
// draft stays with the author.
const draftFreeOfLiveSend = `NOT EXISTS (
	SELECT 1 FROM scheduled_send live
	 WHERE live.status IN ('scheduled', 'held')
	   AND live.payload->>'pending_draft_id' = mail_draft.id::text)`

// receiverHoldsAnchor is a draft whose anchor the receiver already has a
// draft on. A draft is unique per author and anchor, so the receiver's stays.
const receiverHoldsAnchor = `EXISTS (
	SELECT 1 FROM mail_draft held
	 WHERE held.author_id = $2 AND held.anchor_type = mail_draft.anchor_type
	   AND held.anchor_id = mail_draft.anchor_id)`

// MailDraftHandover is what a handover did to the leaver's drafts.
type MailDraftHandover struct {
	Moved, Discarded int
}

// MailDraftsWrittenBy counts the saved drafts author wrote. The count says
// nothing of what a draft holds, so the handover preview may show it.
func (s *Store) MailDraftsWrittenBy(ctx context.Context, tx pgx.Tx, author ids.UserID) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM mail_draft WHERE author_id = $1`, author).Scan(&n); err != nil {
		return 0, fmt.Errorf("mail draft: counting a member's drafts: %w", err)
	}
	return n, nil
}

// TransferableMailDraftsTx counts the drafts a handover by this caller would
// move: none unless the caller may change activities and author has left.
func (s *Store) TransferableMailDraftsTx(ctx context.Context, tx pgx.Tx, author ids.UserID) (int, error) {
	if movable, err := mayMoveDraftsOf(ctx, tx, author); err != nil || !movable {
		return 0, err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM mail_draft WHERE author_id = $1 AND `+draftFreeOfLiveSend, author).Scan(&n); err != nil {
		return 0, fmt.Errorf("mail draft: counting a member's movable drafts: %w", err)
	}
	return n, nil
}

// HandOverMailDraftsTx gives the saved drafts of from, a member who left, to
// to, who reads them from then on as their own. A draft on an anchor to
// already holds a draft on is discarded. A member who has not left keeps
// every draft, so no handover can read an active colleague's unsent mail.
//
// A caller without the update grant on activities moves none, as for tasks, so
// the rest of the handover still lands.
func (s *Store) HandOverMailDraftsTx(ctx context.Context, tx pgx.Tx, from, to ids.UserID) (MailDraftHandover, error) {
	if movable, err := mayMoveDraftsOf(ctx, tx, from); err != nil || !movable {
		return MailDraftHandover{}, err
	}
	rows, err := tx.Query(ctx, `UPDATE mail_draft SET author_id = $2, version = version + 1
		WHERE author_id = $1 AND `+draftFreeOfLiveSend+` AND NOT `+receiverHoldsAnchor+`
		RETURNING `+mailDraftColumns, from, to)
	if err != nil {
		return MailDraftHandover{}, fmt.Errorf("mail draft: handing over: %w", err)
	}
	moved, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailDraft, error) {
		return scanMailDraft(row)
	})
	if storekit.IsUniqueViolation(err) {
		// The receiver saved a draft on one of these anchors while this ran.
		return MailDraftHandover{}, apperrors.ErrConflict
	}
	if err != nil {
		return MailDraftHandover{}, err
	}
	for _, draft := range moved {
		before := map[string]any{fieldAuthor: from, fieldVersion: draft.Version - 1}
		after := draftImage(draft)
		after[fieldAuthor] = to
		if _, err := storekit.Audit(ctx, tx, "update", entityMailDraft, draft.ID, before, after); err != nil {
			return MailDraftHandover{}, err
		}
	}
	discarded, err := deleteDrafts(ctx, tx, `author_id = $1 AND `+draftFreeOfLiveSend+` AND `+receiverHoldsAnchor, []any{from, to})
	if err != nil {
		return MailDraftHandover{}, err
	}
	return MailDraftHandover{Moved: len(moved), Discarded: len(discarded)}, nil
}

// mayMoveDraftsOf is the gate the count and the move share.
func mayMoveDraftsOf(ctx context.Context, tx pgx.Tx, author ids.UserID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return false, nilIfDenied(err)
	}
	return auth.MemberDeparted(ctx, tx, author)
}
