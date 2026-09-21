// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Closing meetings captured before the calendar's RSVP was read.
//
// Live capture now closes a meeting the calendar says is off — cancelled by its
// organizer, or declined by the seat whose calendar it is. Every meeting
// captured before that carries `meeting_status` NULL, which every surface reads
// as booked, and no later sync will correct it: a provider stops listing an
// event once it is off, so the pull that would have carried the news has long
// since passed.
//
// The evidence is already here. Each meeting's raw original is stored verbatim,
// and the RSVP was always in it — nothing looked. So this re-reads what we hold
// rather than asking the provider again, which also means it works for a
// calendar since disconnected.
//
// It asks meetingmap the SAME question live capture asks (SettlementOf over the
// stored bytes). A pass that re-derived "is this meeting off" from the payload
// would be a second answer to a question that package owns, and the two would
// part company the first time a provider changed how it says no.

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/capture/meetingmap"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The outcomes this pass records. Each is a different fact about the meeting,
// and the migration's CHECK holds the vocabulary.
const (
	rsvpBackfillClosed     = "closed"
	rsvpBackfillStillOn    = "still_on"
	rsvpBackfillAnswered   = "answered"
	rsvpBackfillUnreadable = "unreadable"
)

// meetingRSVPBackfillPerTick bounds one pass. The population is finite and
// shrinks by exactly what each tick settles, so a small batch drains it without
// holding a long transaction over the activity table.
const meetingRSVPBackfillPerTick = 200

// rsvpCandidate is one captured meeting this pass has yet to judge, with
// everything the judgement needs read in the SAME query that found it.
//
// It is not replayCandidate: that shape carries the kind and the attestation the
// participant passes need, and none of the key or start this one does. Reading
// those back per meeting would be two more reads of `activity` — two more places
// to gate, and two more chances to forget one.
type rsvpCandidate struct {
	activityID ids.ActivityID
	// source is the CONNECTOR that captured the row, read from captured_by:
	// which vendor's format the stored payload is in.
	source  string
	payload []byte
	// key is the provider key the meeting was captured under — what the cancel
	// writer finds the row by.
	key connector.NaturalKey
	// startsAt is when the meeting was scheduled for, which is what a
	// cancellation is stamped with rather than the moment this pass ran.
	startsAt time.Time
	// owner is the address of the calendar this meeting came off, taken from
	// the connection's own account label.
	owner string
}

// backfillMeetingRSVPBatch re-reads up to limit stored calendar originals and
// closes the meetings their own payload says are off, returning how many
// meetings it settled.
//
// Idempotent by construction. The marker means a settled meeting is not offered
// again, so a restart resumes rather than re-reading from the top; and the write
// itself is CancelCapturedMeetingTx, which refuses a meeting that already
// carries an answer.
func backfillMeetingRSVPBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	return drainStoredOriginals(ctx, pool, limit, log, storedOriginalPass[rsvpCandidate]{
		name:   "meeting rsvp backfill",
		unit:   unitMeetings,
		offer:  selectMeetingRSVPCandidates,
		settle: backfillOneMeetingRSVP,
		mark:   markMeetingRSVPSettled,
	})
}

// selectMeetingRSVPCandidates finds captured meetings whose stored original is
// still on file and which this pass has not settled.
//
// It reads the owner and the format out of captured_by exactly as the attendee
// repair beside it does, because the same provenance answers both questions and
// a second derivation is a second place for one of them to drift.
//
// It does NOT filter on meeting_status. A meeting somebody already answered for
// is offered, settled as `answered`, and marked — so it is asked about once and
// never again. Filtering it out in SQL would leave it forever unmarked and
// re-selected on every tick, which is the one way this pass could fail to drain.
//
// It carries the natural key and the start OUT of this query rather than reading
// them back per meeting. Three reads of `activity` where one will do is three
// places to gate and three chances to forget one; the row is already open here.
//
// `restricted_at IS NULL` alongside the archived filter, because this reader is
// the pass's only look at the activity table and a row under a statutory hold is
// not a calendar's to reopen: such a row is already unavailable in every reader
// path, so moving its status changes nothing anybody could observe, and writing
// to one is the single thing a hold exists to prevent.
func selectMeetingRSVPCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]rsvpCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, split_part(a.captured_by, ':', 2), rc.payload,
		       coalesce(a.source_system, ''), coalesce(a.source_id, ''), a.occurred_at,
		       coalesce((
		         SELECT c.account_label
		           FROM capture_connection c
		          WHERE c.provider = split_part(a.captured_by, ':', 2)
		            AND (
		              c.user_id::text = split_part(a.captured_by, ':', 3)
		              OR (split_part(a.captured_by, ':', 3) = '' AND NOT EXISTS (
		                  SELECT 1 FROM capture_connection other
		                   WHERE other.provider = c.provider AND other.id <> c.id)))
		          LIMIT 1), '')
		  FROM activity a
		  JOIN raw_capture rc
		    ON rc.source_system = a.source_system AND rc.source_id = a.source_id
		 WHERE a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND a.kind = 'meeting'
		   AND split_part(a.captured_by, ':', 2) = ANY($2)
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_meeting_rsvp_backfill b WHERE b.activity_id = a.id)
		 ORDER BY a.id
		 LIMIT $1`, limit, []string{sourceGCal, sourceGraphCal})
	if err != nil {
		return nil, fmt.Errorf("compose: selecting captured meetings whose rsvp can be re-read: %w", err)
	}
	defer rows.Close()

	var out []rsvpCandidate
	for rows.Next() {
		var c rsvpCandidate
		if err := rows.Scan(&c.activityID, &c.source, &c.payload,
			&c.key.SourceSystem, &c.key.SourceID, &c.startsAt, &c.owner); err != nil {
			return nil, fmt.Errorf("compose: reading an rsvp backfill candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the rsvp backfill candidates: %w", err)
	}
	return out, nil
}

// backfillOneMeetingRSVP reads one stored calendar original and closes the
// meeting if its own payload says the meeting is off.
//
// A parse failure is a verdict rather than an error, as it is in the repair
// beside it: the payload is provider output this parser may not decompose, and
// one such meeting must not stop the pass reaching the rest.
//
// SettleDrop is `still_on` here, and the distinction matters. Drop means "this
// event was never worth capturing" — an internal meeting, a solo block — which
// says nothing about whether it is off. The row exists, so it WAS captured under
// whatever rule applied then, and this pass has no cancellation to write.
func backfillOneMeetingRSVP(ctx context.Context, tx pgx.Tx, c rsvpCandidate) (string, error) {
	if c.owner == "" {
		return rsvpBackfillUnreadable, nil
	}
	raw, decodeErr := decodeStoredOriginal(c.payload)
	if decodeErr != nil {
		return rsvpBackfillUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	var settlement meetingmap.Settlement
	var parseErr error
	switch c.source {
	case sourceGCal:
		settlement, parseErr = gcal.SettlementOf(raw, c.owner)
	case sourceGraphCal:
		settlement, parseErr = graphcal.SettlementOf(raw, c.owner)
	default:
		return rsvpBackfillUnreadable, nil
	}
	if parseErr != nil {
		return rsvpBackfillUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	if settlement != meetingmap.SettleCancel {
		return rsvpBackfillStillOn, nil
	}
	// The ONE writer, shared with live capture. It refuses a meeting that
	// already carries an answer — held, no_show or canceled — which is what
	// makes `answered` a real outcome here rather than a guess.
	//
	// The key and the start ride the candidate, read in the query that found it.
	// The start matters: a transition dated now() would record every historical
	// cancellation as having happened the day this pass ran.
	_, closed, err := activities.CancelCapturedMeetingTx(ctx, tx, c.key, c.startsAt)
	if err != nil {
		return "", err
	}
	if !closed {
		return rsvpBackfillAnswered, nil
	}
	return rsvpBackfillClosed, nil
}

// markMeetingRSVPSettled records that this pass has judged one meeting, so it is
// never offered again.
func markMeetingRSVPSettled(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_meeting_rsvp_backfill (activity_id, outcome)
		VALUES ($1, $2)
		ON CONFLICT (activity_id) DO NOTHING`, activityID, outcome); err != nil {
		return fmt.Errorf("compose: marking the rsvp backfill of %s: %w", activityID, err)
	}
	return nil
}

// activity names the row this candidate is about, satisfying
// storedOriginalCandidate.
func (c rsvpCandidate) activity() ids.ActivityID { return c.activityID }
