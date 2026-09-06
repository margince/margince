// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Binding the invited COLLEAGUES of meetings captured before a calendar's
// attendee list was trusted.
//
// Live capture now resolves an attendee who is one of our own users to their
// user_id (capture.ParticipantListAttested). Every meeting captured before that
// carries its attendees by address alone, and an address-only row does not make
// its attendee a member of the meeting's audience — so a colleague who was IN a
// meeting cannot read it once the row is held to its participants.
//
// The ordinary participant replay cannot do this. It settles one marker per
// activity and skips every row that already has one, and these rows have one:
// they were replayed under the OLD rule, which read their attendee list and
// deliberately bound no colleague from it. That marker records a completed
// parse, not a completed user resolution, so it is not the question this pass
// asks and cannot be the answer.
//
// Hence a marker of its own. It is not a redundant copy of the other: the two
// record different questions about the same row, and collapsing them would make
// "we parsed this" and "we resolved its attendees" indistinguishable the next
// time the resolution rule changes.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/gcal"
	"github.com/margince/margince/backend/internal/modules/capture/graphcal"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The outcomes this repair records. `attendees` wrote rows; the other two say
// why a meeting produced none, and both are UNRESOLVED for the purpose of
// judging the rollout — a meeting whose original will not parse has not been
// repaired, it has been given up on, and reporting the two together would let a
// pass that repaired nothing look finished.
const (
	repairBoundAttendees = "attendees"
	repairFoundNone      = "none"
	repairUnreadable     = "unreadable"
)

// meetingAttendeeRepairPerTick bounds one pass. The population is finite and
// shrinks by exactly what each tick settles, so a small batch drains it without
// holding a long transaction over the activity table.
const meetingAttendeeRepairPerTick = 200

// repairMeetingAttendeesBatch re-reads up to limit stored calendar originals and
// binds the colleagues among their attendees, returning how many meetings it
// settled.
//
// Idempotent by construction, in both directions. The write is
// capture.StampFurtherParticipants, whose insert is ON CONFLICT DO NOTHING
// against the participant uniqueness index, so running the pass twice adds
// nothing and changes no display name. And the marker means a settled meeting is
// not offered again, so a restart resumes rather than re-reading from the top.
func repairMeetingAttendeesBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("compose: the meeting attendee repair needs a positive batch limit, got %d", limit)
	}
	// One correlation id per batch, for the same reason the participant replay
	// takes one: naming an attendee is an audited write and storekit refuses to
	// emit its event without one, which would fail the whole batch and re-select
	// the same rows forever.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var settled int
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		candidates, err := selectMeetingRepairCandidates(ctx, tx, limit)
		if err != nil {
			return err
		}
		for _, c := range candidates {
			outcome, err := repairOneMeeting(ctx, tx, c)
			if err != nil {
				return err
			}
			if err := markMeetingRepaired(ctx, tx, c.activityID, outcome); err != nil {
				return err
			}
			settled++
		}
		if settled > 0 {
			log.DebugContext(ctx, "meeting attendee repair: settled a batch of captured meetings", "meetings", settled)
		}
		return nil
	})
	return settled, err
}

// selectMeetingRepairCandidates finds captured meetings whose stored original is
// still on file and whose attendees have not been resolved under the current
// rule.
//
// It reads the owner and the format out of captured_by exactly as the
// participant replay does, because the same provenance answers both questions
// and a second derivation is a second place for one of them to drift. The
// calendar connectors alone are selected: this pass exists for the attendee
// list, and a mail row's recipient list is governed by the attestation the
// replay already applied.
func selectMeetingRepairCandidates(ctx context.Context, tx pgx.Tx, limit int) ([]replayCandidate, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.kind, split_part(a.captured_by, ':', 2), rc.payload,
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
		 WHERE a.archived_at IS NULL
		   AND a.kind = 'meeting'
		   AND split_part(a.captured_by, ':', 2) = ANY($2)
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_meeting_attendee_repair r WHERE r.activity_id = a.id)
		 ORDER BY a.id
		 LIMIT $1`, limit, []string{sourceGCal, sourceGraphCal})
	if err != nil {
		return nil, fmt.Errorf("compose: selecting captured meetings whose attendees can be resolved: %w", err)
	}
	defer rows.Close()

	var out []replayCandidate
	for rows.Next() {
		var c replayCandidate
		if err := rows.Scan(&c.activityID, &c.kind, &c.source, &c.payload, &c.owner); err != nil {
			return nil, fmt.Errorf("compose: reading a meeting repair candidate: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: reading the meeting repair candidates: %w", err)
	}
	return out, nil
}

// repairOneMeeting parses one stored calendar original and re-stamps its
// attendees under the current attestation rule.
//
// A parse failure is a verdict rather than an error, as it is in the replay: the
// payload is provider output this parser may not decompose, and one such meeting
// must not stop the pass reaching the rest. It is recorded as unreadable, which
// the rollout check counts as UNRESOLVED — the honest answer, since nobody has
// been bound to that meeting.
func repairOneMeeting(ctx context.Context, tx pgx.Tx, c replayCandidate) (string, error) {
	if c.owner == "" {
		return repairUnreadable, nil
	}
	raw, decodeErr := decodeStoredOriginal(c.payload)
	if decodeErr != nil {
		return repairUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	var participants []connector.MessageParticipant
	var parseErr error
	switch c.source {
	case sourceGCal:
		participants, parseErr = gcal.ParticipantsOf(raw, c.owner)
	case sourceGraphCal:
		participants, parseErr = graphcal.ParticipantsOf(raw, c.owner)
	default:
		return repairUnreadable, nil
	}
	if parseErr != nil {
		return repairUnreadable, nil //nolint:nilerr // unreadable is the recorded outcome, not a fault
	}
	if len(participants) == 0 {
		return repairFoundNone, nil
	}
	// True, not c.partyListIsAttested(): the query above admits calendar
	// connectors alone, so the attestation is what selected these rows. Reading
	// it back off the candidate would ask the same question twice and let the
	// two answers drift.
	if err := capture.StampFurtherParticipants(ctx, tx, c.activityID, c.kind, true, participants); err != nil {
		return "", err
	}
	// The rows just written carry whatever name the original gave, so the people
	// they resolved to are named here — the same pairing the participant replay
	// makes, and for the same reason: the recovery pass beside it selects on
	// display_name IS NULL, which this stamp has just filled in.
	if err := people.FillParticipantNamesTx(ctx, tx, c.activityID); err != nil {
		return "", err
	}
	return repairBoundAttendees, nil
}

// markMeetingRepaired records that this meeting's attendees have been resolved
// under the current rule, so no later pass offers it again.
func markMeetingRepaired(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, outcome string) error {
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_meeting_attendee_repair (activity_id, outcome)
		VALUES ($1, $2)
		ON CONFLICT (activity_id) DO NOTHING`,
		activityID, outcome); err != nil {
		return fmt.Errorf("compose: recording that a meeting's attendees were resolved: %w", err)
	}
	return nil
}
