// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Closing a promise on evidence: a meeting whose guest accepted fulfils "we
// will invite <name>". The system settles it through moveClaimStatus, the
// write a person's settle makes, and the audit row names the evidence so the
// owner can undo it.

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

// Audit keys of an automatic settlement. The history and the undo read them.
const (
	claimSettledByKey    = "settled_by"
	claimSettledAuto     = "automatic"
	claimEvidenceKey     = "evidence_activity_id"
	claimSettleReasonKey = "settle_reason"
)

// keptByInvitationReason is the reason an accepted invitation records.
const keptByInvitationReason = "invitation_accepted"

// KeptByAcceptedGuest lists the open promises of ours that this meeting kept.
//
// A promise to invite a named person is kept when a live meeting has that
// person as a guest who accepted. The meeting must be made after the promise.
// The promise's own contact must be on the meeting too. The full name is
// matched on word boundaries, each part at least three letters. A promise a
// person reopened after an automatic settlement stays open.
func (s *Store) KeptByAcceptedGuest(ctx context.Context, meetingID ids.UUID) ([]ids.UUID, error) {
	var claims []ids.UUID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT c.id
			  FROM activity m
			  JOIN activity_participant ap ON ap.activity_id = m.id
			   AND ap.role = 'attendee' AND ap.response = 'accepted' AND ap.contact_id IS NOT NULL
			  JOIN contact g ON g.id = ap.contact_id AND g.archived_at IS NULL AND g.full_name LIKE '% %'
			  JOIN conversation_claim c ON c.kind = 'commitment_ours' AND c.status = 'open'
			   AND c.archived_at IS NULL AND c.contact_id <> g.id
			   AND c.body ~* '\yinvit'
			   AND c.body ~* ('\y' || regexp_replace(trim(g.full_name), '([^[:alnum:][:space:]])', '\\\1', 'g') || '\y')
			   AND NOT EXISTS (SELECT 1 FROM regexp_split_to_table(trim(g.full_name), '\s+') part WHERE length(part) < 3)
			   AND (EXISTS (SELECT 1 FROM activity_participant mine WHERE mine.activity_id = m.id AND mine.contact_id = c.contact_id)
			        OR EXISTS (SELECT 1 FROM activity_link mine WHERE mine.activity_id = m.id AND mine.contact_id = c.contact_id))
			  JOIN activity src ON src.id = c.source_activity_id AND src.archived_at IS NULL
			 WHERE m.id = $1 AND m.kind = 'meeting' AND m.archived_at IS NULL
			   AND m.meeting_status IS DISTINCT FROM 'canceled'
			   AND m.created_at >= src.occurred_at
			   AND NOT EXISTS (
			     SELECT 1 FROM audit_log reopened
			      WHERE reopened.entity_type = 'contact' AND reopened.entity_id = c.contact_id
			        AND reopened.after->>'`+claimIDKey+`' = c.id::text
			        AND reopened.before->>'claim_status' = 'done'
			        AND reopened.after->>'claim_status' = 'open')
			 ORDER BY c.id`, meetingID)
		if err != nil {
			return fmt.Errorf("find the promises a meeting kept: %w", err)
		}
		claims, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	return claims, err
}

// SettleKeptByEvidence marks one open promise done on the evidence of an
// activity. Only the system settles this way: a person or their agent settles
// through SettleConversationClaim.
//
// `seat` is the context of the internal seat that made the evidence. Its write
// access to the claim's current contact is checked under the claim's row lock,
// in the transaction that moves the status, so access removed since the event
// stops the settlement.
func (s *Store) SettleKeptByEvidence(ctx, seat context.Context, claimID, evidence ids.UUID) error {
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalSystem {
		return apperrors.ErrPermissionDenied
	}
	if err := auth.Require(seat, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if _, err := storekit.LockRow(ctx, tx, "conversation_claim", claimID, storekit.LiveOnly); err != nil {
			return err
		}
		var contactID ids.UUID
		if err := tx.QueryRow(ctx, `SELECT contact_id FROM conversation_claim WHERE id = $1`, claimID).Scan(&contactID); err != nil {
			return fmt.Errorf("read the claim's contact: %w", err)
		}
		if err := auth.EnsureWritableLive(seat, tx, "contact", contactID); err != nil {
			return err
		}
		return moveClaimStatus(ctx, tx, claimID, claimStatusOpen, claimStatusDone, map[string]any{
			claimSettledByKey:    claimSettledAuto,
			claimSettleReasonKey: keptByInvitationReason,
			claimEvidenceKey:     evidence.String(),
		})
	})
}

// ReopenAutoSettledClaim is the undo behind "I marked this kept": it opens a
// promise the system settled, under the gates a person's settle holds. A
// promise a person settled is not this verb's to open.
func (s *Store) ReopenAutoSettledClaim(ctx context.Context, claimID ids.UUID) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		var automatic bool
		if err := tx.QueryRow(ctx, `
			SELECT coalesce((
			  SELECT a.after->>'`+claimSettledByKey+`' = '`+claimSettledAuto+`'
			    FROM audit_log a
			   WHERE a.entity_type = 'contact' AND a.after->>'`+claimIDKey+`' = $1::text
			     AND a.after->>'claim_status' = 'done'
			   ORDER BY a.occurred_at DESC, a.id DESC LIMIT 1), false)`, claimID).Scan(&automatic); err != nil {
			return fmt.Errorf("read who settled the claim: %w", err)
		}
		if !automatic {
			return fmt.Errorf("this promise was settled by a person: %w", apperrors.ErrConflict)
		}
		return moveClaimStatus(ctx, tx, claimID, claimStatusDone, claimStatusOpen, nil)
	})
}

// MeetingSeat is the internal seat that made a meeting: its organizer, else the
// seat that captured it. Absent for a meeting an outside party invited us to.
func (s *Store) MeetingSeat(ctx context.Context, meetingID ids.UUID) (ids.UUID, bool, error) {
	var seat *ids.UUID
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT coalesce(
			  (SELECT p.user_id FROM activity_participant p
			    WHERE p.activity_id = m.id AND p.role = 'organizer' AND p.user_id IS NOT NULL
			    ORDER BY p.created_at, p.id LIMIT 1),
			  CASE WHEN m.captured_by ~ '^human:[0-9a-f-]{36}$' THEN substr(m.captured_by, 7)::uuid END)
			  FROM activity m WHERE m.id = $1`, meetingID).Scan(&seat)
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, false, fmt.Errorf("read who made the meeting: %w", err)
	}
	if seat == nil {
		return ids.UUID{}, false, nil
	}
	return *seat, true, nil
}
