// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What an agent's draft was written from, and the read that re-proves it: a
// draft whose words rest on a record its reader has lost is discarded rather
// than served.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// discardUngrounded deletes a draft whose words rest on a record its reader
// may no longer see, and says whether it did: the words were derived from that
// record, so serving them hands over what the reader has lost.
func discardUngrounded(ctx context.Context, tx pgx.Tx, draft MailDraft) (bool, error) {
	for _, ground := range draft.Grounding {
		err := ensureGroundReadable(ctx, tx, ground)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			var args []any
			arg := func(v any) int { args = append(args, v); return len(args) }
			if _, err := deleteDrafts(ctx, tx, fmt.Sprintf(`id = $%d`, arg(draft.ID)), args); err != nil {
				return false, err
			}
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}

// ensureGroundReadable asks the full read admission of one record a draft was
// written from: the object grant and the live row. GetMailDraft has already
// required the activity grant, so an activity needs only its content gate.
func ensureGroundReadable(ctx context.Context, tx pgx.Tx, ground MailDraftAnchor) error {
	if ground.Type != crmcontracts.MailDraftAnchorTypeActivity {
		if err := auth.EnsureReadable(ctx, tx, string(ground.Type), ground.ID); err != nil {
			return err
		}
	}
	return ensureDraftAnchorVisible(ctx, tx, ground)
}

// draftGround is one grounding entry as the jsonb column spells it.
type draftGround struct {
	Type crmcontracts.MailDraftAnchorType `json:"entity_type"`
	ID   ids.UUID                         `json:"entity_id"`
}

// groundingColumn is never nil: the column is NOT NULL and a nil slice is null.
func groundingColumn(grounding []MailDraftAnchor) []draftGround {
	out := make([]draftGround, 0, len(grounding))
	for _, ground := range grounding {
		out = append(out, draftGround(ground))
	}
	return out
}
