// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a queued message does when its recipient changes the answer before it
// is due: a stop holds marketing at once, and a reply pauses a follow-up until
// the rep has read it. The fire path still re-asks the engine when the message
// comes due; these hold it earlier, and nothing here sends.

import (
	"context"
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
	// HeldRecipientReplied: a recipient wrote to us after a deal follow-up or
	// a quote to them was scheduled, so the rep reads the reply first.
	HeldRecipientReplied = "recipient_replied"
)

// HoldMarketingToContact holds every pending scheduled marketing message
// addressed to one of the contact's addresses, and answers how many it held.
//
// purposeKey narrows a withdrawal to the marketing sends it covers: a send for
// that purpose, or one that names no purpose. Empty holds every marketing
// send, which is what a suppression binds. Rows are locked as they are read,
// so a message firing at this moment is held or sent, never both.
func (s *Store) HoldMarketingToContact(ctx context.Context, contact ids.ContactID, purposeKey string) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	held := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT ss.id
			  FROM scheduled_send ss
			 WHERE ss.status = 'scheduled'
			   AND ss.payload->>'context' = $1
			   AND ($3 = '' OR coalesce(ss.payload->>'marketing_purpose', '') IN ('', $3))
			   AND EXISTS (
			       SELECT 1
			         FROM jsonb_array_elements_text(
			                coalesce(ss.payload->'recipients', '[]'::jsonb)
			                || coalesce(ss.payload->'cc', '[]'::jsonb)
			                || coalesce(ss.payload->'bcc', '[]'::jsonb)) AS addressed(address)
			         JOIN contact_email ce ON lower(ce.email) = lower(trim(addressed.address))
			        WHERE ce.contact_id = $2)
			 ORDER BY ss.id
			   FOR UPDATE OF ss`, string(commsauthz.CategoryMarketing), contact.UUID, purposeKey)
		if err != nil {
			return fmt.Errorf("scheduled send: finding marketing to a stopped contact: %w", err)
		}
		pending, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("scheduled send: finding marketing to a stopped contact: %w", err)
		}
		for _, id := range pending {
			if err := s.holdInTx(ctx, tx, id, HeldRecipientStopped); err != nil {
				return err
			}
		}
		held = len(pending)
		return nil
	})
	return held, err
}

// pursuitPausedByReply reports whether a recipient wrote to us after a deal
// follow-up or a quote to them was scheduled. Only an authenticated inbound
// mail counts, so a forged sender cannot pause a message on anyone's behalf.
func pursuitPausedByReply(ctx context.Context, tx pgx.Tx, in SendEmailInput, created time.Time) (bool, error) {
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
	var replied bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM activity a
			  JOIN activity_participant p ON p.activity_id = a.id AND p.role = 'from'
			 WHERE a.kind = 'email' AND a.archived_at IS NULL
			   AND `+correspondence.AuthenticatedInboundSQL("a")+`
			   AND lower(p.address) = ANY($1)
			   AND a.occurred_at > $2)`, addresses, created).Scan(&replied)
	if err != nil {
		return false, fmt.Errorf("scheduled send: reading replies since scheduling: %w", err)
	}
	return replied, nil
}
