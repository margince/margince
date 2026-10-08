// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Mail a seat sent from another address of theirs, captured before capture read
// it as outbound. Those rows still read as received from the seat's own
// address, so the waiting lane offers the seat's reply as a customer message,
// and the answered check never counts it. Capture judges each stored original
// (capture.ReclaimStoredOwnSentMailTx) and activities rewrites the row through
// its audited claim; this pass offers the rows and records each verdict.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownSentMailRepairActor names this pass on the audit rows its claims write.
const ownSentMailRepairActor = "system:own_sent_mail_repair"

// ownSentMailRepairPerTick bounds one batch. Each row parses one original's
// header, so it is sized like the participant replay.
const ownSentMailRepairPerTick = participantReplayBatch

// ownSentMailCandidate is one received email the seat's own Gmail or Graph
// connection captured, not yet judged by this pass.
type ownSentMailCandidate struct {
	row          capture.StoredOwnSentMail
	rawCaptureID ids.UUID
}

func (c ownSentMailCandidate) activity() ids.ActivityID { return c.row.Activity }
func (c ownSentMailCandidate) original() ids.UUID       { return c.rawCaptureID }

// repairOwnSentMailBatch judges up to limit stored received emails and answers
// how many it settled.
//
// Idempotent twice over: a judged row carries a marker and is not offered
// again, and activities.ClaimOwnSentMailTx changes nothing on a row that no
// longer reads as received from the seat.
func repairOwnSentMailBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	if pass, ok := principal.Actor(ctx); ok {
		pass.ID = ownSentMailRepairActor
		ctx = principal.WithActor(ctx, pass)
	}
	return drainStoredOriginals(ctx, pool, limit, log, storedOriginalPass[ownSentMailCandidate]{
		name:  "own sent mail repair",
		unit:  "emails",
		offer: selectOwnSentMailCandidates,
		settle: func(ctx context.Context, tx pgx.Tx, c ownSentMailCandidate, payload []byte) (string, error) {
			original, err := decodeStoredOriginal(payload)
			if err != nil {
				return capture.OwnSentMailUnreadable, nil //nolint:nilerr // unreadable is the recorded verdict, not a fault
			}
			// No owner address: capture excludes every address of the seat's,
			// the mailbox's included, which is all the owner would exclude.
			parties, err := mailmap.ParticipantsOf(original, "")
			if err != nil {
				return capture.OwnSentMailUnreadable, nil //nolint:nilerr // unreadable is the recorded verdict, not a fault
			}
			c.row.Participants = parties.Participants
			return capture.ReclaimStoredOwnSentMailTx(ctx, tx, activities.ClaimOwnSentMailTx, c.row, original)
		},
		mark: markOwnSentMailRepaired,
	})
}

// selectOwnSentMailCandidates offers every live, unrestricted received email
// one seat's own Gmail or Graph connection captured. No narrower prefilter:
// which addresses are the seat's is capture's question, and a SQL copy of it
// that missed a spelling would leave rows unrepaired with nothing to say so.
//
// The connection is read out of captured_by as the participant replay reads
// it, and so is the stored original: the link first, the natural key for a row
// carrying none. A stamp from before provenance named the seat is a bare
// `connector:gmail`, and stands for the seat when that seat alone imported it.
//
// A `not_the_seats` verdict is judged again once the seat holds an address or
// a mailbox it did not hold when judged, because that verdict was about the
// addresses the seat held then. Every other verdict reads only the original
// and the row, which do not change.
func selectOwnSentMailCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]ownSentMailCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, ci.user_id, a.counterparty_email, rc.id
		  FROM activity a
		  JOIN capture_import ci ON ci.activity_id = a.id
		  JOIN raw_capture rc
		    ON rc.id = a.raw_capture_id
		    OR (a.raw_capture_id IS NULL
		        AND rc.source_system = a.source_system AND rc.source_id = a.source_id)
		  LEFT JOIN activity_own_sent_mail_repair r ON r.activity_id = a.id
		 WHERE a.kind = 'email' AND a.direction = 'inbound'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND NOT a.has_calendar_part AND coalesce(a.counterparty_email, '') <> ''
		   AND split_part(a.captured_by, ':', 1) = 'connector'
		   AND split_part(a.captured_by, ':', 2) = ANY($2)
		   AND (split_part(a.captured_by, ':', 3) = ci.user_id::text
		     OR (split_part(a.captured_by, ':', 3) = '' AND NOT EXISTS (
		         SELECT 1 FROM capture_import other
		          WHERE other.activity_id = a.id AND other.user_id <> ci.user_id)))
		   AND (r.activity_id IS NULL
		     OR (r.outcome = $3 AND (
		         EXISTS (SELECT 1 FROM capture_owner_identity oi
		                  WHERE oi.user_id = ci.user_id AND oi.created_at > r.settled_at)
		      OR EXISTS (SELECT 1 FROM capture_connection cc
		                  WHERE cc.user_id = ci.user_id AND cc.created_at > r.settled_at))))
		 ORDER BY a.id
		 LIMIT $1`, limit, capture.ProviderFiledMailTransports(), capture.OwnSentMailNotTheSeats)
	if err != nil {
		return nil, fmt.Errorf("compose: selecting received mail a seat may have sent: %w", err)
	}
	defer rows.Close()
	var out []ownSentMailCandidate
	for rows.Next() {
		var c ownSentMailCandidate
		if err := rows.Scan(&c.row.Activity, &c.row.Seat, &c.row.Sender, &c.rawCaptureID); err != nil {
			return nil, fmt.Errorf("compose: reading received mail a seat may have sent: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading received mail a seat may have sent: %w", err)
	}
	return out, nil
}

// markOwnSentMailRepaired records this pass's verdict on one email, so no later
// pass offers it again.
//
// A claimed email also loses its participant replay marker. The replay reads a
// message's further parties as the seat's own statement only when the row is
// attested outbound, which the claim has just made it, so a replay run under
// the received reading is run again.
func markOwnSentMailRepaired(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_own_sent_mail_repair (activity_id, outcome)
		VALUES ($1, $2)
		ON CONFLICT (activity_id) DO UPDATE SET outcome = excluded.outcome, settled_at = now()`,
		activityID, outcome); err != nil {
		return fmt.Errorf("compose: recording whether a received email was the seat's own: %w", err)
	}
	if outcome != capture.OwnSentMailClaimed {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_participant_replay WHERE activity_id = $1`, activityID); err != nil {
		return fmt.Errorf("compose: offering a claimed email to the participant replay again: %w", err)
	}
	return nil
}
