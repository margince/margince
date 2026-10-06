// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// An agent's draft, left in the saved drafts of the human it acts for.
//
// The row is the human's: the author is the principal's user, which for an
// agent is its granting human, and the anchor is probed under that human's row
// scope. It is marked agent_drafted so the screen can say whose words they are
// before anyone sends them, and it never touches a timeline — a draft is the
// message that has NOT happened yet.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ErrOwnDraftWaiting refuses an agent's draft where the human already keeps
// one they saved themselves. Their words outrank an agent's, so the agent's
// draft is answered to the agent and not stored.
var ErrOwnDraftWaiting = errors.New("the human you act for already has an unsent draft of their own here")

// SaveAgentMailDraft leaves an agent's draft for its human to review.
//
// It replaces an earlier agent draft for the same anchor, so asking again
// ("shorter") updates the one waiting draft. It never replaces a draft the
// human saved: that answers ErrOwnDraftWaiting.
func (s *Store) SaveAgentMailDraft(ctx context.Context, anchor MailDraftAnchor, content MailDraftContent) (MailDraft, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return MailDraft{}, err
	}
	// Only an agent's words are marked as an agent's.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return MailDraft{}, fmt.Errorf("an agent's draft from a %s caller: %w", actor.Type, apperrors.ErrPermissionDenied)
	}
	content = canonicalDraft(content)
	if err := validateDraft(anchor, content); err != nil {
		return MailDraft{}, err
	}
	author, err := draftAuthor(ctx)
	if err != nil {
		return MailDraft{}, err
	}
	var out MailDraft
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureDraftAnchorVisible(ctx, tx, anchor); err != nil {
			return err
		}
		out, err = upsertAgentMailDraft(ctx, tx, author, anchor, content)
		return err
	})
	return out, err
}

func upsertAgentMailDraft(ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor, content MailDraftContent) (MailDraft, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	row := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO mail_draft (id, author_id, anchor_type, anchor_id, to_addresses, cc_addresses,
		                        bcc_addresses, subject, body, html_body, agent_drafted)
		VALUES ($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, true)
		ON CONFLICT (author_id, anchor_type, anchor_id) DO UPDATE
		   SET to_addresses = EXCLUDED.to_addresses, cc_addresses = EXCLUDED.cc_addresses,
		       bcc_addresses = EXCLUDED.bcc_addresses, subject = EXCLUDED.subject,
		       body = EXCLUDED.body, html_body = EXCLUDED.html_body,
		       version = mail_draft.version + 1, updated_at = now()
		 WHERE mail_draft.agent_drafted
		RETURNING `+mailDraftColumns,
		arg(ids.NewV7()), arg(author), arg(string(anchor.Type)), arg(anchor.ID),
		arg(addressLine(content.To)), arg(addressLine(content.Cc)), arg(addressLine(content.Bcc)),
		arg(content.Subject), arg(content.Body), arg(nullableText(content.HTMLBody))), args...)
	saved, err := scanMailDraft(row)
	if errors.Is(err, apperrors.ErrNotFound) {
		// The conflict target held and the WHERE refused: the draft there is
		// one the human saved.
		return MailDraft{}, ErrOwnDraftWaiting
	}
	if err != nil {
		return MailDraft{}, err
	}
	if saved.Version == 1 {
		_, err = storekit.AuditEvent(ctx, tx, "create", entityMailDraft, saved.ID, draftImage(saved))
	} else {
		before := map[string]any{fieldVersion: saved.Version - 1}
		_, err = storekit.Audit(ctx, tx, "update", entityMailDraft, saved.ID, before, draftImage(saved))
	}
	if err != nil {
		return MailDraft{}, err
	}
	return saved, nil
}
