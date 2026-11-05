// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The ledger's REVIEW-QUEUE side: the transitions that involve a human rather
// than a worker.
//
// They are guarded differently from the claim lifecycle in pending.go, and the
// difference is the point. A claimed row is held by a worker and every write to
// it presents that worker's claim token; an `unsure` row is held by nobody and
// is waiting for a contact, so its writes CAS on the status instead. Mixing the
// two guards is how a row ends up either unwritable or writable by the wrong
// party.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// LinkProposal points an `unsure` row at the review-queue offer staged for it,
// so a later pass finds the existing offer instead of staging a second one. A
// dead link (the previous offer expired) is overwritten — the pairing that
// matters is row-to-LIVE-offer, and refusing to re-link would strand the row
// the moment its first proposal aged out.
//
// Guarded on the row still being `unsure`: one a human has already decided must
// never have a fresh proposal attached to it.
// declinedProposalLink answers a link the `unsure` guard turned away.
//
// The row was decided between staging the offer and attaching it, so the offer
// describes a question nobody is asking any more. Answering nil leaves a
// proposal no row points at, and the review queue cannot show what was
// proposed.
func declinedProposalLink(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var status string
	switch err := tx.QueryRow(ctx,
		`SELECT status FROM capture_pending_counterparty WHERE id = $1`, id).Scan(&status); {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%w: capture disposition %s", apperrors.ErrNotFound, id)
	case err != nil:
		return fmt.Errorf("capture: reading the disposition that refused the proposal link %s: %w", id, err)
	default:
		return fmt.Errorf("%w: capture disposition %s was decided as %q before the proposal was attached",
			apperrors.ErrConflict, id, status)
	}
}

func (s *PendingStore) LinkProposal(ctx context.Context, id, proposalID ids.UUID) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE capture_pending_counterparty
			   SET proposal_id = $2, updated_at = now()
			 WHERE id = $1 AND status = 'unsure'`, id, proposalID)
		if err == nil && tag.RowsAffected() == 0 {
			return declinedProposalLink(ctx, tx, id)
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: linking the review proposal for %s: %w", id, err)
	}
	return nil
}

// AwaitingReview lists `unsure` rows with no LIVE review-queue offer — the
// staging backlog. A row reaches this state by exhausting the model's attempts,
// so what it needs next is a human, and until a live offer exists nobody can
// give it one.
//
// "No live offer", not "no offer": a staged proposal expires after a day if
// nobody acts on it, and a row whose only offer has expired is exactly as
// undecidable as one that never had a proposal. Keying on proposal_id alone
// would strand it permanently — invisible to the review queue, still counting
// against the workspace's open-question ceiling, and clearable only by hand.
// A workspace that takes a weekend off would silently fill its own cap.
//
// A DECIDED offer is the opposite case and must not come back: re-staging one a
// human already answered would ask them the same question every hour forever.
// Expired means unanswered; decided means answered, whichever way.
func (s *PendingStore) AwaitingReview(ctx context.Context, limit int) ([]PendingCounterparty, error) {
	var out []PendingCounterparty
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.id, p.email, coalesce(p.domain, ''), coalesce(left(p.display_name, $2), ''),
			       p.activity_id, p.owner_id
			  FROM capture_pending_counterparty p
			 WHERE p.status = 'unsure'
			   -- Not a sender whose mail is held. The review queue is a shared
			   -- surface: staging one puts its address and display name in
			   -- front of colleagues who cannot read the message it came from,
			   -- and asks them to decide about a correspondent they were never
			   -- meant to know about. The sender stays unsure and the owner can
			   -- still answer for them on their own Senders page.
			   AND EXISTS (
			     SELECT 1 FROM activity a
			      WHERE a.id = p.activity_id AND a.audience = 'workspace'
			        AND a.restricted_at IS NULL)
			   -- Only a mailbox owner who can be asked: the offer is theirs
			   -- alone, and staging refuses it for anybody else.
			   AND EXISTS (
			     SELECT 1 FROM app_user u
			      WHERE u.id = p.owner_id AND `+workforce.AskableMemberSQL("u")+`)
			   AND NOT EXISTS (
			     SELECT 1 FROM approval a
			      WHERE a.id = p.proposal_id
			        AND (a.decided_at IS NOT NULL
			             OR (a.status = 'pending' AND a.expires_at > now())))
			 ORDER BY p.resolved_at, p.created_at
			 LIMIT $1`, limit, MaxCapturedNameChars)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p PendingCounterparty
			if err := rows.Scan(&p.ID, &p.Email, &p.Domain, &p.DisplayName,
				&p.ActivityID, &p.OwnerID); err != nil {
				return err
			}
			out = append(out, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: reading the review backlog: %w", err)
	}
	return out, nil
}

// ResolveReviewed closes an `unsure` row that a human decided, on the caller's
// transaction. Unlike Resolve it carries no claim token — the authority here is
// the redeemed approval, not a worker's lease, and an `unsure` row is held by
// nobody. The CAS on `unsure` is what makes a replayed redemption a no-op.
func (s *PendingStore) ResolveReviewed(ctx context.Context, tx pgx.Tx, id ids.UUID, status, reason string) error {
	return s.ResolveReviewedAs(ctx, tx, id, status, "", reason)
}

// ResolveReviewedAs is ResolveReviewed that also records the sender kind the
// human's decision implies. An empty kind leaves whatever the model recorded.
func (s *PendingStore) ResolveReviewedAs(ctx context.Context, tx pgx.Tx, id ids.UUID, status, kind, reason string) error {
	if err := lockPendingChecksTx(ctx, tx, id); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE capture_pending_counterparty
		   SET status = $2, disposition_reason = NULLIF($3, ''),
		       kind = COALESCE(NULLIF($4, ''), kind),
		       resolved_at = now(), next_attempt_at = NULL, updated_at = now()
		 WHERE id = $1 AND status = 'unsure'`, id, status, reason, kind)
	if err != nil {
		return fmt.Errorf("capture: resolving reviewed disposition %s: %w", id, err)
	}
	if tag.RowsAffected() == 0 {
		return declinedReview(ctx, tx, id, status)
	}
	return nil
}

// declinedReview answers a resolution the CAS on `unsure` turned away.
//
// Two different things land here and they are not the same answer. A redeemed
// approval replayed asks for the decision the row already carries, and the
// no-op is what makes the replay safe. A SECOND reviewer asks for a different
// one, and answering nil there drops a human's decision while their screen
// says saved.
//
// So the stored disposition decides: the same status is a replay, a different
// one is a conflict, and a row that is gone is gone.
func declinedReview(ctx context.Context, tx pgx.Tx, id ids.UUID, wanted string) error {
	var stored string
	switch err := tx.QueryRow(ctx,
		`SELECT status FROM capture_pending_counterparty WHERE id = $1`, id).Scan(&stored); {
	case errors.Is(err, pgx.ErrNoRows):
		return fmt.Errorf("%w: capture disposition %s", apperrors.ErrNotFound, id)
	case err != nil:
		return fmt.Errorf("capture: reading the disposition that refused the review %s: %w", id, err)
	case stored == wanted:
		return nil
	default:
		return fmt.Errorf("%w: capture disposition %s was already reviewed as %q, not %q",
			apperrors.ErrConflict, id, stored, wanted)
	}
}

// Retire ends a claimed row at `unsure`: the model never cleared the floor in
// its allowed attempts, so the question passes to a human. It stamps the
// attempt count and the stop time, so the row says why it ended. measured is
// the last answer, which is the evidence the human now judges.
func (s *PendingStore) Retire(
	ctx context.Context, p PendingCounterparty, reason string, measured VerdictMeasurement,
) error {
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := storekit.LockSeatAddressChecks(ctx, tx, p.Email); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			UPDATE capture_pending_counterparty
			   SET status = 'unsure', disposition_reason = NULLIF($2, ''),
			       attempts = $4, resolved_at = now(),
			       confidence = $5, served_model = NULLIF($6, ''),
			       next_attempt_at = NULL, claimed_until = NULL, claimed_by = NULL,
			       updated_at = now()
			 WHERE id = $1 AND status = 'pending' AND claimed_by = $3`,
			p.ID, reason, p.Claim, PendingMaxAttempts, measured.confidence(), measured.Model)
		if err == nil && tag.RowsAffected() == 0 {
			return declinedRetire(ctx, tx, p.ID, p.Claim)
		}
		return err
	})
	if err != nil {
		return fmt.Errorf("capture: retiring disposition %s: %w", p.ID, err)
	}
	return nil
}
