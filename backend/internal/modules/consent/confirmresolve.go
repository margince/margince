// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Opening a confirm link, and recording what that actually was.
//
// The resolver lives here rather than beside the mint because what it writes is
// the subject of linkfetch.go: a fetch is always recorded, an opening only when
// the request presents as a human navigating to the page. Those two columns
// are evidence, and keeping the rule and the write in sight of each other is
// what stops the next author restoring the old one-line version.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ResolveConfirmToken answers whose record a confirm link opens. Unknown,
// expired, already-spent and belonging-to-an-archived-subject read as absent,
// all four identically, so the surface never becomes an oracle for which it was.
//
// The liveness test is here rather than in the card read, so both verbs get it
// from one statement. An ordinary archive does not delete these rows — only
// Art. 17 erasure and the retention anonymizer do — so a rep archiving a contact
// who holds a live link would otherwise leave the next click answering 500,
// and a submit would burn the link before refusing.
//
// Resolution runs outside row-level security for the same reason the preference
// resolver does: the surface it serves has no session, and the token IS the
// authorization.
//
// IT RECORDS WHAT ACTUALLY HAPPENED, which is two different facts. Every
// resolution counts a fetch (first_fetched_at, fetch_count); only one the
// request itself presents as a human navigating to the page stamps opened_at.
//
// That column is evidence — the middle of the ask-to-click chain a later reader
// follows from the row, with a named data subject as its subject — and it used
// to be written by every GET. Most GETs of a link in a mail are not humans:
// scanners, proxies and preview generators fetch them before the recipient sees
// the message, and each one wrote a line saying somebody opened their consent
// link. See linkfetch.go for what the request is asked and why the doubt falls
// towards recording a human.
func (s *Store) ResolveConfirmToken(
	ctx context.Context, token string, by FetchKind,
) (ConfirmRef, error) {
	var ref ConfirmRef
	// The kind is read HERE and not only at the spend, because the read is a
	// disclosure of its own: a consent link's page must show the subscription
	// question and not the contact's record card. Gating the write and leaving
	// the read open would hand whoever holds a consent link everything the
	// record page shows, which is wider than the mail that carried it.
	var purposeID *ids.PurposeID
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			UPDATE confirm_token ct
			   SET first_fetched_at = coalesce(ct.first_fetched_at, $2),
			       fetch_count = ct.fetch_count + 1,
			       opened_at = CASE WHEN $3 THEN coalesce(ct.opened_at, $2) ELSE ct.opened_at END
			WHERE ct.token_hash = $1 AND ct.consumed_at IS NULL AND ct.expires_at > $2
			  AND EXISTS (SELECT 1 FROM contact p
			               WHERE p.id = ct.contact_id AND p.archived_at IS NULL)
			RETURNING ct.contact_id, ct.id, ct.delivered_to, ct.kind, ct.purpose_id`,
			hashPublicToken(token), s.now().UTC(), by == FetchByAHuman).Scan(
			&ref.ContactID, &ref.TokenID, &ref.DeliveredTo, &ref.Kind, &purposeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if purposeID != nil {
			ref.PurposeID = *purposeID
		}
		return err
	})
	if err != nil {
		return ConfirmRef{}, err
	}
	return ref, nil
}
