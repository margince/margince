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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
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
	namesFiles := draft.content.AttachmentIDs != nil
	if draft.content, _, err = s.prepareDraft(ctx, anchor, draft.content); err != nil {
		return MailDraft{}, err
	}
	var out MailDraft
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureDraftAnchorVisible(ctx, tx, anchor); err != nil {
			return err
		}
		// What the draft holds is read under the row's lock and written in this same
		// transaction. An agent save meanwhile then loses nothing this one keeps.
		if draft.content, err = s.withWhatTheDraftHolds(ctx, tx, author, anchor, draft, namesFiles); err != nil {
			return err
		}
		out, err = upsertAgentMailDraft(ctx, tx, author, anchor, draft)
		return err
	})
	if err != nil {
		return MailDraft{}, err
	}
	// The files are judged after the lock is gone, the way every read judges them.
	if out.Files, err = s.draftFilesOf(ctx, out.Content.AttachmentIDs); err != nil {
		return MailDraft{}, err
	}
	return out, nil
}

// withWhatTheDraftHolds fills in what new words leave out from the agent draft already
// there. That is the files as stored, the context and a schedule still to come. It
// looks up no file while the row is held. A file since hidden or unsendable is left to the read
// and the send, which judge every file again.
func (s *Store) withWhatTheDraftHolds(
	ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor, draft groundedContent, namesFiles bool,
) (MailDraftContent, error) {
	content := draft.content
	if !draft.newWords {
		return content, nil
	}
	var held MailDraftContent
	var at, wall *time.Time
	var tz, claimed *string
	err := tx.QueryRow(ctx, `SELECT attachment_ids, communication_context, `+scheduleInstantSQL+`,scheduled_tz,scheduled_wall FROM mail_draft
 WHERE author_id=@author AND anchor_type=@anchor_type AND anchor_id=@anchor_id AND agent_drafted FOR UPDATE`,
		pgx.NamedArgs{"author": author, "anchor_type": string(anchor.Type), "anchor_id": anchor.ID}).Scan(&held.AttachmentIDs, &claimed, &at, &tz, &wall)
	if errors.Is(err, pgx.ErrNoRows) {
		return content, nil
	}
	if err != nil {
		return content, fmt.Errorf("activities: reading what the draft holds: %w", err)
	}
	if !namesFiles {
		content.AttachmentIDs = held.AttachmentIDs
	}
	content.Context = ""
	if claimed != nil {
		content.Context = commsauthz.Category(*claimed)
	}
	content.Schedule = nil
	if at != nil && tz != nil && (wall != nil || at.After(s.now())) {
		schedule, err := readSchedule(at, tz, wall)
		if err != nil {
			return content, err
		}
		content.Schedule = schedule
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
	at, tz, wall := draftScheduleColumns(content.Schedule)
	row := tx.QueryRow(ctx, `
 INSERT INTO mail_draft(id,author_id,anchor_type,anchor_id,to_addresses,cc_addresses,
 bcc_addresses,subject,body,html_body,agent_drafted,grounding,attachment_ids,
 communication_context,scheduled_at,scheduled_tz,scheduled_wall)
 SELECT @id,@author,@anchor_type,@anchor_id,@to,@cc,@bcc,@subject,@body,@html,true,@grounding::jsonb,@attachments,
 @context,planned.instant,COALESCE(planned.timezone,@zone),planned.wall_time
 FROM public.update_intent_time(NULL,NULL,NULL,@wall::text::timestamp,@zone,@at) AS planned
 WHERE true
 ON CONFLICT(author_id,anchor_type,anchor_id) DO UPDATE
 SET to_addresses=EXCLUDED.to_addresses,cc_addresses=EXCLUDED.cc_addresses,bcc_addresses=EXCLUDED.bcc_addresses,
 subject=EXCLUDED.subject,body=EXCLUDED.body,html_body=EXCLUDED.html_body,
 grounding=CASE WHEN @keep_grounding::boolean THEN mail_draft.grounding ELSE EXCLUDED.grounding END,
 attachment_ids=EXCLUDED.attachment_ids,communication_context=EXCLUDED.communication_context,
 (scheduled_at,scheduled_tz,scheduled_wall)=(
 SELECT planned.instant,COALESCE(planned.timezone,EXCLUDED.scheduled_tz),planned.wall_time
 FROM public.update_intent_time(mail_draft.scheduled_wall,mail_draft.scheduled_tz,mail_draft.scheduled_at,
 EXCLUDED.scheduled_wall,EXCLUDED.scheduled_tz,EXCLUDED.scheduled_at) AS planned),
 editor_state='{}'::jsonb,version=mail_draft.version+1,updated_at=now()
 WHERE mail_draft.agent_drafted AND (@version::bigint IS NULL OR mail_draft.version=@version)
 RETURNING `+mailDraftColumns, pgx.NamedArgs{
		"id": ids.NewV7(), "author": author, "anchor_type": string(anchor.Type), "anchor_id": anchor.ID,
		"to": addressLine(content.To), "cc": addressLine(content.Cc), "bcc": addressLine(content.Bcc),
		"subject": content.Subject, "body": content.Body, "html": nullableText(content.HTMLBody),
		"grounding": groundingColumn(draft.grounding), "attachments": content.AttachmentIDs, "context": nullableText(string(content.Context)),
		"at": at, "zone": tz, "wall": wall, "keep_grounding": draft.keepsGrounding, "version": draft.expected,
	})
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
