// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PreviewDraftCarryTx exposes only the acting author's drafts and never discards lost grounding.
func (s *Store) PreviewDraftCarryTx(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) (*crmcontracts.MailDraft, *crmcontracts.MailDraft, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, nil, err
	}
	author, err := draftAuthor(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, id := range []ids.UUID{source, target} {
		anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorType(kind), ID: id}
		if err := checkDraftAnchor(anchor); err != nil {
			return nil, nil, err
		}
		if err := ensureGroundReadable(ctx, tx, anchor); err != nil {
			return nil, nil, err
		}
	}
	var private bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM mail_draft WHERE anchor_type=@kind AND anchor_id=@source AND author_id<>@author)`, pgx.StrictNamedArgs{fieldKind: kind, ledgerSourceKey: source, "author": author}).Scan(&private); err != nil {
		return nil, nil, err
	}
	if private {
		return nil, nil, apperrors.ErrNotFound
	}
	from, err := readCarriedDraft(ctx, tx, kind, source, author)
	if err != nil || from == nil {
		return nil, nil, err
	}
	to, err := readCarriedDraft(ctx, tx, kind, target, author)
	if err != nil {
		return nil, nil, err
	}
	return from, to, nil
}

func readCarriedDraft(ctx context.Context, tx pgx.Tx, kind string, anchor, author ids.UUID) (*crmcontracts.MailDraft, error) {
	draft, err := scanMailDraft(tx.QueryRow(ctx, `SELECT `+mailDraftColumns+` FROM mail_draft
 WHERE anchor_type=@kind AND anchor_id=@anchor AND author_id=@author`, pgx.StrictNamedArgs{fieldKind: kind, "anchor": anchor, "author": author}))
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, nil //nolint:nilnil // No draft means no effect for this author.
	}
	if err != nil {
		return nil, err
	}
	// Previewing must not perform GetMailDraft's discard-on-lost-grounding mutation.
	for _, ground := range draft.Grounding {
		if err := ensureGroundReadable(ctx, tx, ground); err != nil {
			return nil, err
		}
	}
	out := mailDraftResponse(draft)
	return &out, nil
}
