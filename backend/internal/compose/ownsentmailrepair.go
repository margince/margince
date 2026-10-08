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
	"github.com/margince/margince/backend/internal/shared/ports/connector"
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
			return capture.ReclaimStoredOwnSentMailTx(ctx, tx, activities.ClaimOwnSentMailTx, ownSentMailParties, c.row, original)
		},
		mark: markOwnSentMailRepaired,
	})
}

// ownSentMailParties reads an original's further parties with no owner
// address: capture excludes every address of the seat's, the mailbox's
// included, which is all the owner would exclude.
func ownSentMailParties(original []byte) ([]connector.MessageParticipant, error) {
	parties, err := mailmap.ParticipantsOf(original, "")
	return parties.Participants, err
}

// selectOwnSentMailCandidates offers the live, unrestricted, non-bulk received
// emails captured before the cutoff by one seat's own Gmail or Graph
// connection, whose counterparty is an address that seat alone has proven, in
// every spelling a stored counterparty can take. The offer reads no original
// whose sender lacks standing; capture still judges each row.
//
// The connection is read out of captured_by as the participant replay reads
// it, and so is the stored original: the link first, the natural key for a row
// carrying none. A stamp from before provenance named the seat is a bare
// `connector:gmail`, and stands for the seat when that seat alone imported it.
//
// A `not_the_seats` verdict is offered again: the offer holds only rows whose
// sender is proven now, so a row it reaches again has regained its standing.
// Every other verdict reads only the original and the row.
func selectOwnSentMailCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]ownSentMailCandidate, error) {
	proved, err := capture.UnambiguouslyProvedAddressesTx(ctx, tx)
	if err != nil || len(proved) == 0 {
		return nil, err
	}
	var seats []ids.UUID
	var addresses []string
	for _, p := range proved {
		for _, spelling := range p.Spellings() {
			seats = append(seats, p.Seat)
			addresses = append(addresses, spelling)
		}
	}
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT a.id, ci.user_id, a.counterparty_email, rc.id
		  FROM activity a
		  JOIN capture_import ci ON ci.activity_id = a.id
		  JOIN unnest($4::uuid[], $5::text[]) AS proved(seat, address)
		    ON proved.seat = ci.user_id AND proved.address = a.counterparty_email
		  JOIN raw_capture rc
		    ON rc.id = a.raw_capture_id
		    OR (a.raw_capture_id IS NULL
		        AND rc.source_system = a.source_system AND rc.source_id = a.source_id)
		  LEFT JOIN activity_own_sent_mail_repair r ON r.activity_id = a.id
		 WHERE a.kind = 'email' AND a.direction = 'inbound'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND NOT a.has_calendar_part AND NOT a.bulk_mail_attested
		   AND a.created_at < (SELECT captured_before FROM activity_own_sent_mail_repair_cutoff)
		   AND split_part(a.captured_by, ':', 1) = 'connector'
		   AND split_part(a.captured_by, ':', 2) = ANY($2)
		   AND (split_part(a.captured_by, ':', 3) = ci.user_id::text
		     OR (split_part(a.captured_by, ':', 3) = '' AND NOT EXISTS (
		         SELECT 1 FROM capture_import other
		          WHERE other.activity_id = a.id AND other.user_id <> ci.user_id)))
		   AND (r.activity_id IS NULL OR r.outcome = $3)
		 ORDER BY a.id
		 LIMIT $1`, limit, capture.ProviderFiledMailTransports(), capture.OwnSentMailNotTheSeats, seats, addresses)
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
// A claimed email keeps its participant replay marker. Replaying it again would
// re-read further parties from an original that can outlive an erasure of one
// of them, and the replay writes those parties without the suppression list.
func markOwnSentMailRepaired(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_own_sent_mail_repair (activity_id, outcome)
		VALUES ($1, $2)
		ON CONFLICT (activity_id) DO UPDATE SET outcome = excluded.outcome, settled_at = now()`,
		activityID, outcome); err != nil {
		return fmt.Errorf("compose: recording whether a received email was the seat's own: %w", err)
	}
	return nil
}
