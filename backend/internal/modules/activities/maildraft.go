// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// A rep's unsent message: the composer's own state, saved so that closing it
// loses nothing. It is never an activity and never on a timeline, and it is
// readable by its author alone — one per author and anchor, the place the
// composer opened on.
//
// Audit-only, like scheduled_send: the closed event catalog has no draft type,
// and nothing downstream could act on one. The audit images carry the anchor
// and the version and never the content, so the compliance log learns that a
// draft was kept without learning what it said.

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// MailDraftAnchor is what the composer opened against.
type MailDraftAnchor struct {
	Type crmcontracts.MailDraftAnchorType
	ID   ids.UUID
}

// MailDraftContent is the composer's fields as they stand. Addresses are
// trimmed and lowercased on save and otherwise unchecked until the message is
// sent — a draft may hold one half-typed.
type MailDraftContent struct {
	To       []string
	Cc       []string
	Bcc      []string
	Subject  string
	Body     string
	HTMLBody string
	// AttachmentIDs, Context and Schedule are what the send would be asked
	// for. A saved draft holds only files its author could attach when it saved.
	AttachmentIDs []ids.UUID
	Context       commsauthz.Category
	Schedule      *SendSchedule
	EditorState   crmcontracts.MailDraftEditorState
}

// MailDraft is one saved draft.
type MailDraft struct {
	ID      ids.UUID
	Anchor  MailDraftAnchor
	Content MailDraftContent
	Version int64
	// AgentDrafted is the AI marking: an agent wrote these words for the
	// author, and no save from the author's composer has replaced them yet.
	AgentDrafted bool
	// Grounding lists the records an agent wrote these words from. It outlives
	// the author's edits, because edited words still rest on what they read.
	Grounding []MailDraftAnchor
	// Files is what the reader may attach of the chosen files; only a read or a save resolves it.
	Files     resolvedFiles
	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	entityMailDraft = "mail_draft"
	fieldAnchorType = "anchor_type"
	fieldVersion    = "version"
	fieldAnchorID   = "anchor_id"
	fieldBcc        = "bcc"
	// maxDraftAddresses bounds each address line. A composer holds a handful;
	// the ceiling stops a caller parking a mailing list in a row nothing reads.
	maxDraftAddresses = 100
	// maxDraftSubject and maxDraftAddress are the contract's caps (RFC 5322's
	// line and mailbox lengths), counted in characters.
	maxDraftSubject = 998
	maxDraftAddress = 320
)

const mailDraftColumns = `id, anchor_type, anchor_id, to_addresses, cc_addresses, bcc_addresses,
	subject, body, html_body, version, agent_drafted, grounding, attachment_ids,
	communication_context, ` + scheduleInstantSQL + ` AS scheduled_at, scheduled_tz, scheduled_wall, created_at, updated_at, editor_state`

// InvalidMailDraftError refuses a draft the store will not keep.
type InvalidMailDraftError struct {
	Field  string
	Reason string
}

func (e *InvalidMailDraftError) Error() string { return e.Field + " " + e.Reason }

// FieldFault names the field the caller has to correct.
func (e *InvalidMailDraftError) FieldFault() (field, code, message string) {
	return e.Field, "invalid_mail_draft", e.Error()
}

// GetMailDraft reads the caller's draft for one anchor. No draft, somebody
// else's, and an anchor the caller can no longer see are all ErrNotFound, as
// is a draft resting on a record they can no longer see, which is discarded.
func (s *Store) GetMailDraft(ctx context.Context, anchor MailDraftAnchor) (MailDraft, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return MailDraft{}, err
	}
	if err := checkDraftAnchor(anchor); err != nil {
		return MailDraft{}, err
	}
	author, err := draftAuthor(ctx)
	if err != nil {
		return MailDraft{}, err
	}
	var (
		out       MailDraft
		discarded bool
	)
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureDraftAnchorVisible(ctx, tx, anchor); err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		query := fmt.Sprintf(`
			SELECT `+mailDraftColumns+`
			  FROM mail_draft
			 WHERE author_id = $%d AND anchor_type = $%d AND anchor_id = $%d`,
			arg(author), arg(string(anchor.Type)), arg(anchor.ID))
		row := tx.QueryRow(ctx, query, args...)
		if out, err = scanMailDraft(row); err != nil {
			return err
		}
		// The discard returns nil so it commits; the refusal is answered below.
		discarded, err = discardUngrounded(ctx, tx, out)
		return err
	})
	if err != nil {
		return MailDraft{}, err
	}
	if discarded {
		return MailDraft{}, apperrors.ErrNotFound
	}
	if out.Files, err = s.draftFilesOf(ctx, out.Content.AttachmentIDs); err != nil {
		return MailDraft{}, err
	}
	return out, nil
}

// SaveMailDraft keeps the composer's fields for one anchor.
//
// No expected version means "I hold no draft here": it creates one and answers
// ErrVersionSkew when a draft already exists. An expected version replaces the
// draft at that version and answers ErrVersionSkew when it has moved or gone.
// Two composers open on one place therefore cannot overwrite each other.
//
// An agent's save is an agent's draft, whichever door it came through.
func (s *Store) SaveMailDraft(ctx context.Context, anchor MailDraftAnchor, content MailDraftContent, expected *int64) (MailDraft, error) {
	if actor, ok := principal.Actor(ctx); ok && actor.Type == principal.PrincipalAgent {
		return s.saveAgentDraft(ctx, anchor, groundedContent{content: content, keepsGrounding: true, expected: expected})
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return MailDraft{}, err
	}
	author, err := draftAuthor(ctx)
	if err != nil {
		return MailDraft{}, err
	}
	content, files, err := s.prepareDraft(ctx, anchor, content)
	if err != nil {
		return MailDraft{}, err
	}
	var out MailDraft
	err = s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureDraftAnchorVisible(ctx, tx, anchor); err != nil {
			return err
		}
		if expected == nil {
			out, err = insertMailDraft(ctx, tx, author, anchor, content)
			return err
		}
		out, err = replaceMailDraft(ctx, tx, author, anchor, content, *expected)
		return err
	})
	if err != nil {
		return MailDraft{}, err
	}
	out.Files = files
	return out, nil
}

// mailDraftArgs binds the anchor and content arguments that every write of a
// draft shares.
func mailDraftArgs(author ids.UUID, anchor MailDraftAnchor, content MailDraftContent) pgx.NamedArgs {
	at, tz, wall := draftScheduleColumns(content.Schedule)
	return pgx.NamedArgs{
		argAuthor: author, fieldAnchorType: string(anchor.Type), fieldAnchorID: anchor.ID,
		"to": addressLine(content.To), "cc": addressLine(content.Cc), fieldBcc: addressLine(content.Bcc),
		fieldSubject: content.Subject, fieldBody: content.Body, argHTML: nullableText(content.HTMLBody),
		"attachments": content.AttachmentIDs, "context": nullableText(string(content.Context)),
		"at": at, argZone: tz, argWall: wall,
	}
}

func insertMailDraft(ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor, content MailDraftContent) (MailDraft, error) {
	args := mailDraftArgs(author, anchor, content)
	args["id"], args["editor"] = ids.NewV7(), content.EditorState
	row := tx.QueryRow(ctx, `
 INSERT INTO mail_draft(id,author_id,anchor_type,anchor_id,to_addresses,cc_addresses,
 bcc_addresses,subject,body,html_body,attachment_ids,communication_context,
 scheduled_at,scheduled_tz,scheduled_wall,editor_state)
 SELECT @id,@author,@anchor_type,@anchor_id,@to,@cc,@bcc,@subject,@body,@html,
 @attachments,@context,planned.instant,COALESCE(planned.timezone,@zone),planned.wall_time,@editor
 FROM public.update_intent_time(NULL,NULL,NULL,@wall::text::timestamp,@zone,@at) AS planned
 WHERE true
 ON CONFLICT(author_id,anchor_type,anchor_id) DO NOTHING
 RETURNING `+mailDraftColumns, args)
	saved, err := scanMailDraft(row)
	if errors.Is(err, apperrors.ErrNotFound) {
		// The conflict target held: a draft is already here that this caller
		// did not say they had read.
		return MailDraft{}, apperrors.ErrVersionSkew
	}
	if err != nil {
		return MailDraft{}, err
	}
	if _, err := storekit.AuditEvent(ctx, tx, "create", entityMailDraft, saved.ID, draftImage(saved)); err != nil {
		return MailDraft{}, err
	}
	return saved, nil
}

func replaceMailDraft(ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor, content MailDraftContent, expected int64) (MailDraft, error) {
	args := mailDraftArgs(author, anchor, content)
	args[fieldVersion], args["editor"] = expected, content.EditorState
	row := tx.QueryRow(ctx, `
 UPDATE mail_draft
 SET to_addresses=@to,cc_addresses=@cc,bcc_addresses=@bcc,
 subject=@subject,body=@body,html_body=@html,attachment_ids=@attachments,
 communication_context=@context,
 (scheduled_at,scheduled_tz,scheduled_wall)=(
 SELECT planned.instant,COALESCE(planned.timezone,@zone),planned.wall_time
 FROM public.update_intent_time(scheduled_wall,scheduled_tz,scheduled_at,@wall::text::timestamp,@zone,@at) AS planned),
 editor_state=@editor,version=version+1,agent_drafted=false,updated_at=now()
 WHERE author_id=@author AND anchor_type=@anchor_type AND anchor_id=@anchor_id AND version=@version
 RETURNING `+mailDraftColumns, args)
	saved, err := scanMailDraft(row)
	if errors.Is(err, apperrors.ErrNotFound) {
		// Moved by another composer, or discarded: the draft this caller read
		// is not the one there now.
		return MailDraft{}, apperrors.ErrVersionSkew
	}
	if err != nil {
		return MailDraft{}, err
	}
	before := map[string]any{fieldVersion: expected}
	if _, err := storekit.Audit(ctx, tx, "update", entityMailDraft, saved.ID, before, draftImage(saved)); err != nil {
		return MailDraft{}, err
	}
	return saved, nil
}

// DiscardMailDraft deletes one of the caller's drafts.
//
// The anchor is not probed: discarding only removes what the author wrote, and
// a draft whose anchor the author lost sight of would otherwise stay forever.
func (s *Store) DiscardMailDraft(ctx context.Context, id ids.UUID) error {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return err
	}
	author, err := draftAuthor(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := refuseAgentOverOwnDraft(ctx, tx, id, author); err != nil {
			return err
		}
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		discarded, err := deleteDrafts(ctx, tx, fmt.Sprintf(
			`id = $%d AND author_id = $%d`, arg(id), arg(author)), args)
		if err != nil {
			return err
		}
		if len(discarded) == 0 {
			return apperrors.ErrNotFound
		}
		return nil
	})
}

// ensureDraftAnchorVisible asks whether the caller may still see what the
// draft is about. A reply needs the conversation's content, so an activity is
// probed through its content gate rather than the discover one.
func ensureDraftAnchorVisible(ctx context.Context, tx pgx.Tx, anchor MailDraftAnchor) error {
	if anchor.Type == crmcontracts.MailDraftAnchorTypeActivity {
		return auth.EnsureActivityContentVisibleLive(ctx, tx, anchor.ID)
	}
	// Each record type lives in a table named for it (linktarget.go), and the
	// row-scope probe refuses any name outside its own table set.
	return auth.EnsureVisibleLive(ctx, tx, string(anchor.Type), anchor.ID)
}

// checkDraftAnchor admits the conversation a reply answers or a record a
// message can be filed under — the timeline link's own vocabulary.
func checkDraftAnchor(anchor MailDraftAnchor) error {
	if anchor.Type == crmcontracts.MailDraftAnchorTypeActivity || linkColumn(string(anchor.Type)) != "" {
		return nil
	}
	return &InvalidMailDraftError{Field: fieldAnchorType, Reason: "is not a place a message can be written from"}
}

// draftAuthor is the seat a draft belongs to — the principal, never the body.
func draftAuthor(ctx context.Context) (ids.UUID, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return ids.UUID{}, err
	}
	if actor.UserID.IsZero() {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

func validateDraft(anchor MailDraftAnchor, content MailDraftContent, now time.Time) error {
	if err := checkDraftAnchor(anchor); err != nil {
		return err
	}
	for _, line := range []struct {
		field     string
		addresses []string
	}{{"to", content.To}, {"cc", content.Cc}, {fieldBcc, content.Bcc}} {
		if len(line.addresses) > maxDraftAddresses {
			return &InvalidMailDraftError{Field: line.field, Reason: fmt.Sprintf("holds more than %d addresses", maxDraftAddresses)}
		}
		for _, addr := range line.addresses {
			if utf8.RuneCountInString(addr) > maxDraftAddress {
				return &InvalidMailDraftError{Field: line.field, Reason: fmt.Sprintf("holds an address longer than %d characters", maxDraftAddress)}
			}
		}
	}
	if utf8.RuneCountInString(content.Subject) > maxDraftSubject {
		return &InvalidMailDraftError{Field: fieldSubject, Reason: fmt.Sprintf("is longer than %d characters", maxDraftSubject)}
	}
	return validateDraftSending(content, now)
}

// canonicalDraft keeps each address the way the send path compares it, so
// erasure and the subject export find a draft by the address they hold.
func canonicalDraft(content MailDraftContent) MailDraftContent {
	content.EditorState = boundedDraftEditorState(content.EditorState)
	content.To = canonicalAddresses(content.To)
	content.Cc = canonicalAddresses(content.Cc)
	content.Bcc = canonicalAddresses(content.Bcc)
	content.EditorState.ContentFingerprint = new(draftContentFingerprint(content))
	return content
}

func canonicalAddresses(line []string) []string {
	out := make([]string, 0, len(line))
	for _, addr := range line {
		if addr = normalizeAddress(addr); addr != "" {
			out = append(out, addr)
		}
	}
	return out
}

// draftImage is what the audit trail keeps: where and which version, never
// what the rep wrote.
//
//craft:ignore naked-any the audit seam serializes this image to jsonb
func draftImage(d MailDraft) map[string]any {
	return map[string]any{fieldAnchorType: string(d.Anchor.Type), fieldAnchorID: d.Anchor.ID, fieldVersion: d.Version}
}

// addressLine is never nil: the columns are NOT NULL and a nil slice is NULL.
func addressLine(line []string) []string {
	if line == nil {
		return []string{}
	}
	return line
}

func nullableText(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func scanMailDraft(row pgx.Row) (MailDraft, error) {
	var (
		out        MailDraft
		anchorType string
		html       *string
		grounding  []draftGround
		claimed    *string
		at         *time.Time
		wall       *time.Time
		tz         *string
	)
	err := row.Scan(&out.ID, &anchorType, &out.Anchor.ID, &out.Content.To, &out.Content.Cc,
		&out.Content.Bcc, &out.Content.Subject, &out.Content.Body, &html, &out.Version,
		&out.AgentDrafted, &grounding, &out.Content.AttachmentIDs, &claimed, &at, &tz, &wall,
		&out.CreatedAt, &out.UpdatedAt, &out.Content.EditorState)
	if errors.Is(err, pgx.ErrNoRows) {
		return MailDraft{}, apperrors.ErrNotFound
	}
	if err != nil {
		return MailDraft{}, fmt.Errorf("mail draft: reading: %w", err)
	}
	out.Anchor.Type = crmcontracts.MailDraftAnchorType(anchorType)
	if html != nil {
		out.Content.HTMLBody = *html
	}
	if out.Content.EditorState.ContentFingerprint == nil || *out.Content.EditorState.ContentFingerprint != draftContentFingerprint(out.Content) {
		out.Content.EditorState = crmcontracts.MailDraftEditorState{}
	}
	for _, ground := range grounding {
		out.Grounding = append(out.Grounding, MailDraftAnchor(ground))
	}
	out.Content.Context = commsauthz.Category(orEmpty(claimed))
	schedule, err := readSchedule(at, tz, wall)
	if err != nil {
		return MailDraft{}, err
	}
	out.Content.Schedule = schedule
	return out, nil
}
