// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// How a draft leaves: the retention sweep, and the send that carried it out.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MailDraftRetention is how long a draft nobody saves again is kept. A draft whose
// anchor was deleted or left its author's scope has no other way out. Reading it
// answers not found, so its author cannot find it to discard it.
const MailDraftRetention = 90 * 24 * time.Hour

// PurgeStaleMailDrafts deletes up to limit drafts not saved within
// MailDraftRetention, oldest first, in one transaction, and answers how many
// went. The retention sweep calls it as the system; a backlog deeper than
// limit is left to its next pass, like ListOrphanedObjects.
func (s *Store) PurgeStaleMailDrafts(ctx context.Context, limit int) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	var purged int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		cutoff := arg(s.now().Add(-MailDraftRetention))
		// The cutoff is asked again of the row the DELETE locks. A draft saved while
		// the sweep waited on it is fresh again and stays.
		stale, err := deleteDrafts(ctx, tx, fmt.Sprintf(`updated_at < $%d AND id IN (
			SELECT id FROM mail_draft WHERE updated_at < $%d ORDER BY updated_at LIMIT $%d)`,
			cutoff, cutoff, arg(limit)), args)
		purged = len(stale)
		return err
	})
	if err != nil {
		return 0, err
	}
	return purged, nil
}

// discardComposedDraftTx removes the draft a message was composed in, inside
// the transaction that sends or schedules it.
//
// The draft goes only with a committed send or schedule. Every refusal unwinds
// this transaction, so the rep keeps their draft in the composer. Anything but a
// human caller's own draft for this anchor is ignored, because a stale draft id
// must never stop a message going out.
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

// draftAnchors names where a draft for this message could have been opened: the
// conversation a reply answers, or the record an account-started message is under.
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
