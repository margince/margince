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
// human saved: that answers ErrOwnDraftWaiting. grounding names every record
// the words were written from, which GetMailDraft re-proves on each read.
func (s *Store) SaveAgentMailDraft(
	ctx context.Context, anchor MailDraftAnchor, content MailDraftContent, grounding []MailDraftAnchor,
) (MailDraft, error) {
	return s.saveAgentDraft(ctx, anchor, groundedContent{content: content, grounding: grounding, newWords: true})
}

func (s *Store) saveAgentDraft(ctx context.Context, anchor MailDraftAnchor, draft groundedContent) (MailDraft, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return MailDraft{}, err
	}
	// Only an agent's words are marked as an agent's.
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return MailDraft{}, fmt.Errorf("an agent's draft from a %s caller: %w", actor.Type, apperrors.ErrPermissionDenied)
	}
	for _, ground := range draft.grounding {
		if checkDraftAnchor(ground) != nil {
			return MailDraft{}, &InvalidMailDraftError{Field: "grounding", Reason: "names a record a draft cannot be written from"}
		}
	}
	author, err := draftAuthor(ctx)
	if err != nil {
		return MailDraft{}, err
	}
	if draft.content, err = s.withWhatTheDraftHolds(ctx, author, anchor, draft); err != nil {
		return MailDraft{}, err
	}
	content, files, err := s.prepareDraft(ctx, anchor, draft.content)
	if err != nil {
		return MailDraft{}, err
	}
	draft.content = content
	var out MailDraft
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureDraftAnchorVisible(ctx, tx, anchor); err != nil {
			return err
		}
		out, err = upsertAgentMailDraft(ctx, tx, author, anchor, draft)
		return err
	})
	if err != nil {
		return MailDraft{}, err
	}
	out.Files = files
	return out, nil
}

// withWhatTheDraftHolds fills in what a save leaves out from the agent draft already
// there. New words keep the files, the context and a schedule still to come, and a
// save that names no files keeps those. prepareDraft then judges the files again, so
// one the caller can no longer see stays out.
func (s *Store) withWhatTheDraftHolds(ctx context.Context, author ids.UUID, anchor MailDraftAnchor, draft groundedContent) (MailDraftContent, error) {
	content := draft.content
	if content.AttachmentIDs != nil && !draft.newWords {
		return content, nil
	}
	var held MailDraft
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		held, err = scanMailDraft(tx.QueryRow(ctx, `SELECT `+mailDraftColumns+` FROM mail_draft
			WHERE author_id = $1 AND anchor_type = $2 AND anchor_id = $3 AND agent_drafted`, author, string(anchor.Type), anchor.ID))
		return err
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return content, nil
	}
	if err != nil {
		return content, err
	}
	if content.AttachmentIDs == nil {
		content.AttachmentIDs = held.Content.AttachmentIDs
	}
	if draft.newWords {
		content.Context = held.Content.Context
		if held.Content.Schedule != nil && held.Content.Schedule.At.After(s.now()) {
			content.Schedule = held.Content.Schedule
		}
	}
	return content, nil
}

// whyTheDraftStood names what refused an agent's save: a draft the human saved, or
// an agent draft that moved past the version the caller read.
func whyTheDraftStood(ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor) error {
	var byAgent bool
	if err := tx.QueryRow(ctx, `SELECT agent_drafted FROM mail_draft WHERE author_id = $1 AND anchor_type = $2 AND anchor_id = $3`,
		author, string(anchor.Type), anchor.ID).Scan(&byAgent); err != nil {
		return fmt.Errorf("activities: reading the draft that stood: %w", err)
	}
	if byAgent {
		return apperrors.ErrVersionSkew
	}
	return ErrOwnDraftWaiting
}

// refuseAgentOverOwnDraft keeps a draft the human saved from their agent. The
// draft stays held, so the human's save cannot land between this read and the delete.
func refuseAgentOverOwnDraft(ctx context.Context, tx pgx.Tx, id, author ids.UUID) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	var byAgent bool
	err := tx.QueryRow(ctx, `SELECT agent_drafted FROM mail_draft WHERE id = $1 AND author_id = $2 FOR UPDATE`, id, author).Scan(&byAgent)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.ErrNotFound
	case err != nil:
		return fmt.Errorf("activities: reading whose draft this is: %w", err)
	case !byAgent:
		return ErrOwnDraftWaiting
	}
	return nil
}

// groundedContent is what an agent leaves: its words and what they were written from.
type groundedContent struct {
	content   MailDraftContent
	grounding []MailDraftAnchor
	// keepsGrounding is a save that names no grounding, so what the draft was
	// written from stays what it rests on.
	keepsGrounding bool
	// expected is the version the caller read, nil when it names none.
	expected *int64
	// newWords is a save of the words alone, which leaves how the draft goes out as it is.
	newWords bool
}

func upsertAgentMailDraft(ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor, draft groundedContent) (MailDraft, error) {
	content := draft.content
	at, tz := draftScheduleColumns(content.Schedule)
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := fmt.Sprintf(`
		INSERT INTO mail_draft (id, author_id, anchor_type, anchor_id, to_addresses, cc_addresses,
		                        bcc_addresses, subject, body, html_body, agent_drafted, grounding,
		                        attachment_ids, communication_context, scheduled_at, scheduled_tz)
		VALUES ($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, true, $%d::jsonb, $%d, $%d, $%d, $%d)
		ON CONFLICT (author_id, anchor_type, anchor_id) DO UPDATE
		   SET to_addresses = EXCLUDED.to_addresses, cc_addresses = EXCLUDED.cc_addresses,
		       bcc_addresses = EXCLUDED.bcc_addresses, subject = EXCLUDED.subject,
		       body = EXCLUDED.body, html_body = EXCLUDED.html_body,
		       grounding = CASE WHEN $%d::boolean THEN mail_draft.grounding ELSE EXCLUDED.grounding END,
		       attachment_ids = EXCLUDED.attachment_ids,
		       communication_context = EXCLUDED.communication_context,
		       scheduled_at = EXCLUDED.scheduled_at, scheduled_tz = EXCLUDED.scheduled_tz,
		       version = mail_draft.version + 1, updated_at = now()
		 WHERE mail_draft.agent_drafted AND ($%d::bigint IS NULL OR mail_draft.version = $%d)
		RETURNING `+mailDraftColumns,
		arg(ids.NewV7()), arg(author), arg(string(anchor.Type)), arg(anchor.ID),
		arg(addressLine(content.To)), arg(addressLine(content.Cc)), arg(addressLine(content.Bcc)),
		arg(content.Subject), arg(content.Body), arg(nullableText(content.HTMLBody)),
		arg(groundingColumn(draft.grounding)), arg(content.AttachmentIDs),
		arg(nullableText(string(content.Context))), arg(at), arg(tz),
		arg(draft.keepsGrounding), arg(draft.expected), arg(draft.expected))
	row := tx.QueryRow(ctx, query, args...)
	saved, err := scanMailDraft(row)
	if errors.Is(err, apperrors.ErrNotFound) {
		return MailDraft{}, whyTheDraftStood(ctx, tx, author, anchor)
	}
	if err != nil {
		return MailDraft{}, err
	}
	if draft.expected != nil && saved.Version == 1 {
		// The draft the caller read is gone. The rollback undoes this insert.
		return MailDraft{}, apperrors.ErrVersionSkew
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
