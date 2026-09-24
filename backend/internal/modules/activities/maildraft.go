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

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailDraftAnchor is what the composer opened against.
type MailDraftAnchor struct {
	Type crmcontracts.MailDraftAnchorType
	ID   ids.UUID
}

// MailDraftContent is the composer's fields as they stand. Addresses are kept
// as typed; they are checked when the message is sent, never here.
type MailDraftContent struct {
	To       []string
	Cc       []string
	Bcc      []string
	Subject  string
	Body     string
	HTMLBody string
}

// MailDraft is one saved draft.
type MailDraft struct {
	ID        ids.UUID
	Anchor    MailDraftAnchor
	Content   MailDraftContent
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

const (
	entityMailDraft = "mail_draft"
	fieldAnchorType = "anchor_type"
	// maxDraftAddresses bounds each address line. A composer holds a handful;
	// the ceiling stops a caller parking a mailing list in a row nothing reads.
	maxDraftAddresses = 100
)

// mailDraftAnchorTables names the table each anchor's visibility is probed
// against, as compile-time literals so no request string is formatted into SQL.
var mailDraftAnchorTables = map[crmcontracts.MailDraftAnchorType]string{
	crmcontracts.MailDraftAnchorTypeActivity: linkEntityActivity,
	crmcontracts.MailDraftAnchorTypeContact:  linkEntityContact,
	crmcontracts.MailDraftAnchorTypeCompany:  linkEntityCompany,
	crmcontracts.MailDraftAnchorTypeDeal:     linkEntityDeal,
	crmcontracts.MailDraftAnchorTypeLead:     "lead",
	crmcontracts.MailDraftAnchorTypeProject:  linkEntityProject,
}

const mailDraftColumns = `id, anchor_type, anchor_id, to_addresses, cc_addresses, bcc_addresses,
	subject, body, html_body, version, created_at, updated_at`

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
// else's, and an anchor the caller can no longer see are all ErrNotFound.
func (s *Store) GetMailDraft(ctx context.Context, anchor MailDraftAnchor) (MailDraft, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
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
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		row := tx.QueryRow(ctx, fmt.Sprintf(`
			SELECT `+mailDraftColumns+`
			  FROM mail_draft
			 WHERE author_id = $%d AND anchor_type = $%d AND anchor_id = $%d`,
			arg(author), arg(string(anchor.Type)), arg(anchor.ID)), args...)
		out, err = scanMailDraft(row)
		return err
	})
	return out, err
}

// SaveMailDraft keeps the composer's fields for one anchor.
//
// No expected version means "I hold no draft here": it creates one and answers
// ErrVersionSkew when a draft already exists. An expected version replaces the
// draft at that version and answers ErrVersionSkew when it has moved or gone.
// Two composers open on one place therefore cannot overwrite each other.
func (s *Store) SaveMailDraft(ctx context.Context, anchor MailDraftAnchor, content MailDraftContent, expected *int64) (MailDraft, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return MailDraft{}, err
	}
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
		if expected == nil {
			out, err = insertMailDraft(ctx, tx, author, anchor, content)
			return err
		}
		out, err = replaceMailDraft(ctx, tx, author, anchor, content, *expected)
		return err
	})
	return out, err
}

func insertMailDraft(ctx context.Context, tx pgx.Tx, author ids.UUID, anchor MailDraftAnchor, content MailDraftContent) (MailDraft, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	row := tx.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO mail_draft (id, author_id, anchor_type, anchor_id, to_addresses, cc_addresses,
		                        bcc_addresses, subject, body, html_body)
		VALUES ($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)
		ON CONFLICT (author_id, anchor_type, anchor_id) DO NOTHING
		RETURNING `+mailDraftColumns,
		arg(ids.NewV7()), arg(author), arg(string(anchor.Type)), arg(anchor.ID),
		arg(addressLine(content.To)), arg(addressLine(content.Cc)), arg(addressLine(content.Bcc)),
		arg(content.Subject), arg(content.Body), arg(nullableText(content.HTMLBody))), args...)
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
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	row := tx.QueryRow(ctx, fmt.Sprintf(`
		UPDATE mail_draft
		   SET to_addresses = $%d, cc_addresses = $%d, bcc_addresses = $%d,
		       subject = $%d, body = $%d, html_body = $%d,
		       version = version + 1, updated_at = now()
		 WHERE author_id = $%d AND anchor_type = $%d AND anchor_id = $%d AND version = $%d
		RETURNING `+mailDraftColumns,
		arg(addressLine(content.To)), arg(addressLine(content.Cc)), arg(addressLine(content.Bcc)),
		arg(content.Subject), arg(content.Body), arg(nullableText(content.HTMLBody)),
		arg(author), arg(string(anchor.Type)), arg(anchor.ID), arg(expected)), args...)
	saved, err := scanMailDraft(row)
	if errors.Is(err, apperrors.ErrNotFound) {
		// Moved by another composer, or discarded: the draft this caller read
		// is not the one there now.
		return MailDraft{}, apperrors.ErrVersionSkew
	}
	if err != nil {
		return MailDraft{}, err
	}
	before := map[string]any{"version": expected}
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

// discardComposedDraftTx removes the draft a message was composed in, inside
// the transaction that sends or schedules it.
//
// The draft goes ONLY with a committed send or schedule. Every refusal — a
// consent gate, a missing mailbox, one frozen for review — unwinds this
// transaction, so the rep is left in the composer with their draft intact. The
// id must be a human caller's own draft for this message's anchor; anything
// else is ignored rather than refused, because a stale draft id must never
// stop a message going out.
func discardComposedDraftTx(ctx context.Context, tx pgx.Tx, draftID ids.UUID, origin SendOrigin) error {
	actor, ok := principal.Actor(ctx)
	if draftID.IsZero() || !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return nil
	}
	types, anchorIDs := origin.draftAnchors()
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	_, err := deleteDrafts(ctx, tx, fmt.Sprintf(`
		id = $%d AND author_id = $%d
		AND (anchor_type, anchor_id) IN (SELECT * FROM unnest($%d::text[], $%d::uuid[]))`,
		arg(draftID), arg(actor.UserID), arg(types), arg(anchorIDs)), args)
	return err
}

// draftAnchors names where a draft for this message could have been opened:
// the conversation a reply answers, or a record an account-started message is
// filed under.
func (o SendOrigin) draftAnchors() (types []string, anchorIDs []ids.UUID) {
	if o.isReply() {
		return []string{string(crmcontracts.MailDraftAnchorTypeActivity)}, []ids.UUID{o.anchor.UUID}
	}
	for _, link := range o.links {
		types = append(types, link.EntityType)
		anchorIDs = append(anchorIDs, link.EntityID)
	}
	return types, anchorIDs
}

// deleteDrafts removes drafts and audits each one, so a discard by hand and a
// discard by sending leave the same trail.
//
//craft:ignore naked-any the placeholder arguments of a predicate its callers derive
func deleteDrafts(ctx context.Context, tx pgx.Tx, predicate string, args []any) ([]MailDraft, error) {
	rows, err := tx.Query(ctx, `DELETE FROM mail_draft WHERE `+predicate+` RETURNING `+mailDraftColumns, args...)
	if err != nil {
		return nil, fmt.Errorf("mail draft: discarding: %w", err)
	}
	discarded, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MailDraft, error) {
		return scanMailDraft(row)
	})
	if err != nil {
		return nil, err
	}
	for _, draft := range discarded {
		if _, err := storekit.AuditEvent(ctx, tx, "delete", entityMailDraft, draft.ID, draftImage(draft)); err != nil {
			return nil, err
		}
	}
	return discarded, nil
}

// ensureDraftAnchorVisible asks whether the caller may still see what the
// draft is about. A reply needs the conversation's content, so an activity is
// probed through its content gate rather than the discover one.
func ensureDraftAnchorVisible(ctx context.Context, tx pgx.Tx, anchor MailDraftAnchor) error {
	table, ok := mailDraftAnchorTables[anchor.Type]
	if !ok {
		return &InvalidMailDraftError{Field: fieldAnchorType, Reason: "is not a place a message can be written from"}
	}
	if anchor.Type == crmcontracts.MailDraftAnchorTypeActivity {
		return auth.EnsureActivityContentVisibleLive(ctx, tx, anchor.ID)
	}
	return auth.EnsureVisibleLive(ctx, tx, table, anchor.ID)
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

func validateDraft(anchor MailDraftAnchor, content MailDraftContent) error {
	if _, ok := mailDraftAnchorTables[anchor.Type]; !ok {
		return &InvalidMailDraftError{Field: fieldAnchorType, Reason: "is not a place a message can be written from"}
	}
	for field, line := range map[string][]string{"to": content.To, "cc": content.Cc, "bcc": content.Bcc} {
		if len(line) > maxDraftAddresses {
			return &InvalidMailDraftError{Field: field, Reason: fmt.Sprintf("holds more than %d addresses", maxDraftAddresses)}
		}
	}
	return nil
}

// draftImage is what the audit trail keeps: where and which version, never
// what the rep wrote.
//
//craft:ignore naked-any the audit seam serializes this image to jsonb
func draftImage(d MailDraft) map[string]any {
	return map[string]any{fieldAnchorType: string(d.Anchor.Type), "anchor_id": d.Anchor.ID, "version": d.Version}
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
	)
	err := row.Scan(&out.ID, &anchorType, &out.Anchor.ID, &out.Content.To, &out.Content.Cc,
		&out.Content.Bcc, &out.Content.Subject, &out.Content.Body, &html, &out.Version,
		&out.CreatedAt, &out.UpdatedAt)
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
	return out, nil
}
