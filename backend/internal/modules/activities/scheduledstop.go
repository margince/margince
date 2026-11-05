// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a queued message does when its recipient changes the answer before it
// is due: a stop holds marketing at once, and a reply pauses a follow-up until
// the rep has read it. The fire path still re-asks the engine when the message
// comes due; these hold it earlier, and nothing here sends.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Two more hold reasons beside the ones in scheduledsend.go, each naming what
// the rep has to decide about.
const (
	// HeldRecipientStopped: a recipient objected, withdrew or stopped
	// accepting mail after a marketing message to them was scheduled.
	HeldRecipientStopped = "recipient_stopped"
	// HeldRecipientReplied: new mail came in from a recipient after a deal
	// follow-up or a quote to them was scheduled, so the rep reads it first.
	HeldRecipientReplied = "recipient_replied"
)

// PendingMarketing is one queued marketing message a stop reaches, with the
// rep it belongs to, so its hold can be raised in their inbox.
type PendingMarketing struct {
	ID          ids.UUID
	ScheduledBy ids.UUID
}

// PendingMarketingToContact lists every pending scheduled marketing message
// addressed to one of the contact's live addresses. The caller holds each
// through HoldScheduledSend, the one hold that also raises the rep's card. An
// address the contact no longer holds is not theirs: it may since belong to
// somebody else, whose mail this stop does not reach.
//
// purposeKey narrows a withdrawal to the marketing sends it covers. A message's
// purpose is read the way the engine reads it (commsauthz CanonicalPurpose),
// so a legacy purpose key counts. A message naming no purpose, or two that
// disagree, is held too: the fire path would judge it against this stop. Empty
// lists every marketing send, which is what a suppression binds.
func (s *Store) PendingMarketingToContact(ctx context.Context, contact ids.ContactID, purposeKey string) ([]PendingMarketing, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	var pending []PendingMarketing
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT ss.id, ss.scheduled_by,
			       coalesce(ss.payload->>'marketing_purpose', ''), coalesce(ss.payload->>'consent_purpose', '')
			  FROM scheduled_send ss
			 WHERE ss.status = 'scheduled'
			   AND ss.payload->>'context' = $1
			   AND EXISTS (
			       SELECT 1
			         FROM jsonb_array_elements_text(
			                coalesce(ss.payload->'recipients', '[]'::jsonb)
			                || coalesce(ss.payload->'cc', '[]'::jsonb)
			                || coalesce(ss.payload->'bcc', '[]'::jsonb)) AS addressed(address)
			         JOIN contact_email ce ON lower(ce.email) = lower(trim(addressed.address))
			                              AND ce.archived_at IS NULL
			        WHERE ce.contact_id = $2)
			 ORDER BY ss.id`, string(commsauthz.CategoryMarketing), contact.UUID)
		if err != nil {
			return fmt.Errorf("scheduled send: finding marketing to a stopped contact: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var row PendingMarketing
			var marketing, legacy string
			if err := rows.Scan(&row.ID, &row.ScheduledBy, &marketing, &legacy); err != nil {
				return fmt.Errorf("scheduled send: finding marketing to a stopped contact: %w", err)
			}
			if stopCovers(purposeKey, marketing, legacy) {
				pending = append(pending, row)
			}
		}
		return rows.Err()
	})
	return pending, err
}

// stopCovers reports whether a stop for purposeKey reaches a message frozen
// with these two purpose fields.
func stopCovers(purposeKey, marketing, legacy string) bool {
	if purposeKey == "" {
		return true
	}
	claimed, agree := commsauthz.Request{MarketingPurpose: marketing, LegacyPurposeKey: legacy}.CanonicalPurpose()
	if !agree || claimed.LegacyPurposeKey == "" {
		return true
	}
	return commsauthz.NormalizedPurposeKey(claimed.LegacyPurposeKey) == commsauthz.NormalizedPurposeKey(purposeKey)
}

// pursuitPausedByReply reports whether a recipient wrote to us after a deal
// follow-up or a quote to them was scheduled.
//
// Only an authenticated inbound mail counts, so a forged sender cannot pause a
// message, and mail a sending system sent (an auto-reply, a bulk mailing) is
// not a person writing. Its time is when it reached us: the provider's arrival
// time, or the capture time, never the Date header the sender wrote. A
// scheduled reply listens on its own thread; a new message has no thread yet,
// so any new mail from a recipient counts.
func pursuitPausedByReply(ctx context.Context, tx pgx.Tx, in SendEmailInput, anchor ids.UUID, created time.Time) (bool, error) {
	if in.Context != commsauthz.CategoryActiveDealFollowup && in.Context != commsauthz.CategoryPrecontractQuote {
		return false, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return false, err
	}
	addresses := make([]string, 0, len(in.Recipients))
	for _, r := range in.Recipients {
		addresses = append(addresses, correspondence.Fold(r))
	}
	var thread *string
	if !anchor.IsZero() {
		if err := tx.QueryRow(ctx, `SELECT thread_key FROM activity WHERE id = $1`, anchor).Scan(&thread); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return false, fmt.Errorf("scheduled send: reading the thread a reply listens on: %w", err)
		}
	}
	var replied bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM activity a
			  JOIN activity_participant p ON p.activity_id = a.id AND p.role = 'from'
			 WHERE a.kind = 'email' AND a.archived_at IS NULL
			   AND `+correspondence.AuthenticatedInboundSQL("a")+`
			   AND NOT a.sender_is_machine AND NOT a.bulk_mail_attested
			   AND lower(p.address) = ANY($1)
			   AND ($3::text IS NULL OR a.thread_key = $3)
			   AND coalesce(
			         (SELECT min(coalesce(ci.provider_received_at, ci.imported_at))
			            FROM capture_import ci WHERE ci.activity_id = a.id),
			         a.created_at) > $2)`, addresses, created, thread).Scan(&replied)
	if err != nil {
		return false, fmt.Errorf("scheduled send: reading replies since scheduling: %w", err)
	}
	return replied, nil
}
