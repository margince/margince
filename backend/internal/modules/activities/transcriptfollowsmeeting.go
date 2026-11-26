// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A transcript that records a meeting names it in source_activity_id. The
// meeting's readers are its readers. It has the same audience and reason and
// the same selected members. Its guests are read from the meeting itself
// (auth.guestRowsSQL); the copy here names who attended for its readings.

// recordingTranscriptSQL is a live transcript, alias t, of a meeting, alias m.
const recordingTranscriptSQL = `t.source_system = '` + transcriptSourceSystem + `' AND t.source_activity_id = m.id
	AND m.kind = '` + KindMeeting + `' AND t.archived_at IS NULL AND t.restricted_at IS NULL`

// meetingColleaguesSQL is the colleagues meeting m admits as guests: a row
// that does not stand there must not stand on the transcript either.
var meetingColleaguesSQL = `SELECT mp.user_id FROM activity_participant mp WHERE mp.activity_id = m.id
	AND mp.user_id IS NOT NULL AND ` + auth.ParticipantStandsSQL("mp", "m")

// transcriptReadersDriftSQL holds when transcript t no longer has the readers
// of meeting m.
var transcriptReadersDriftSQL = `(t.audience IS DISTINCT FROM m.audience OR t.audience_reason IS DISTINCT FROM m.audience_reason
	OR EXISTS ((SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = t.id)
	           EXCEPT ALL SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = m.id)
	OR EXISTS ((SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = m.id)
	           EXCEPT ALL SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = t.id)
	OR EXISTS (SELECT user_id FROM activity_participant WHERE activity_id = t.id AND user_id IS NOT NULL
	           EXCEPT ` + meetingColleaguesSQL + `)
	OR EXISTS (` + meetingColleaguesSQL + ` EXCEPT SELECT user_id FROM activity_participant WHERE activity_id = t.id))`

// copyMeetingReadersTx gives transcript the readers of meeting.
func copyMeetingReadersTx(ctx context.Context, tx pgx.Tx, transcript, meeting ids.ActivityID) error {
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, transcript.UUID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE activity t SET audience = f.audience, audience_reason = f.audience_reason
		  FROM activity f WHERE t.id = $1 AND t.archived_at IS NULL AND f.id = $2`, transcript, meeting); err != nil {
		return fmt.Errorf("activities: copying a meeting's audience: %w", err)
	}
	rows, err := tx.Query(ctx,
		`SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = $1`, meeting)
	if err != nil {
		return fmt.Errorf("activities: reading a meeting's audience members: %w", err)
	}
	members, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (AudienceMember, error) {
		var member AudienceMember
		return member, row.Scan(&member.SubjectType, &member.SubjectID)
	})
	if err != nil {
		return fmt.Errorf("activities: reading a meeting's audience members: %w", err)
	}
	if err := replaceAudienceMembers(ctx, tx, transcript, members); err != nil {
		return err
	}
	if _, err := carryMissingParticipantsTx(ctx, tx, meeting, transcript); err != nil {
		return err
	}
	// A colleague who left the meeting leaves the transcript's attendance too,
	// as does a copied row that does not stand on the meeting.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_participant p USING activity m WHERE p.activity_id = $1 AND m.id = $2
		   AND p.user_id IS NOT NULL AND p.user_id NOT IN (`+meetingColleaguesSQL+`)`,
		transcript, meeting); err != nil {
		return fmt.Errorf("activities: removing the colleagues a meeting no longer has: %w", err)
	}
	return nil
}

// followRecordedMeetingTx gives each live transcript of meeting the meeting's
// readers as they are now.
func followRecordedMeetingTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID) error {
	rows, err := tx.Query(ctx, `SELECT t.id FROM activity t JOIN activity m ON m.id = $1
		 WHERE `+recordingTranscriptSQL+` AND `+transcriptReadersDriftSQL+` ORDER BY t.id`, meeting)
	if err != nil {
		return fmt.Errorf("activities: reading the transcripts of a meeting: %w", err)
	}
	transcripts, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return fmt.Errorf("activities: reading the transcripts of a meeting: %w", err)
	}
	if err := storekit.LockMeetingAttendeePage(ctx, tx, transcripts); err != nil {
		return err
	}
	for _, transcript := range transcripts {
		if err := followMeetingReadersTx(ctx, tx, ids.From[ids.ActivityKind](transcript), meeting); err != nil {
			return err
		}
	}
	return nil
}

// DriftedPage is what one page of FollowDriftedTranscriptsTx did.
type DriftedPage struct {
	Changed int
	// Next is the cursor of the following page, zero after the last page.
	// Each page starts past the one before, so a pair that stays drifted
	// cannot hold a sweep on one page.
	Next ids.ActivityID
}

// FollowDriftedTranscriptsTx gives up to limit transcripts after the cursor
// whose meeting's readers changed the readers they now have. It catches every
// writer of a meeting's guests, including those outside this module.
func FollowDriftedTranscriptsTx(ctx context.Context, tx pgx.Tx, after ids.ActivityID, limit int) (DriftedPage, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return DriftedPage{}, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id, m.id FROM activity t JOIN activity m ON m.id = t.source_activity_id
		 WHERE t.id > $2 AND `+recordingTranscriptSQL+` AND `+transcriptReadersDriftSQL+` ORDER BY t.id LIMIT $1`,
		limit, after)
	if err != nil {
		return DriftedPage{}, fmt.Errorf("activities: finding the transcripts whose meeting's readers changed: %w", err)
	}
	drifted, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (recordedPair, error) {
		var p recordedPair
		return p, row.Scan(&p.transcript, &p.meeting)
	})
	if err != nil {
		return DriftedPage{}, fmt.Errorf("activities: finding the transcripts whose meeting's readers changed: %w", err)
	}
	var page DriftedPage
	if len(drifted) == limit {
		page.Next = drifted[len(drifted)-1].transcript
	}
	transcripts := make([]ids.UUID, 0, len(drifted))
	for _, p := range drifted {
		transcripts = append(transcripts, p.transcript.UUID)
	}
	if err := storekit.LockMeetingAttendeeWrites(ctx, tx, transcripts...); err != nil {
		return DriftedPage{}, err
	}
	for _, p := range drifted {
		locked, err := lockRecordedPairTx(ctx, tx, p)
		if err != nil {
			return DriftedPage{}, err
		}
		if !locked {
			continue
		}
		if err := followMeetingReadersTx(ctx, tx, p.transcript, p.meeting); err != nil {
			return DriftedPage{}, err
		}
		page.Changed++
	}
	return page, nil
}

// recordedPair is a transcript and the meeting it records.
type recordedPair struct{ transcript, meeting ids.ActivityID }

// lockRecordedPairTx locks the meeting, then the transcript, in the order an
// audience write takes them, so neither can change while the readers are
// copied. A row another writer holds is skipped, and the next pass takes it.
// It answers false when the pair is held or no longer drifts.
func lockRecordedPairTx(ctx context.Context, tx pgx.Tx, p recordedPair) (bool, error) {
	var locked bool
	err := tx.QueryRow(ctx, `WITH meeting AS (SELECT id FROM activity WHERE id = $2 FOR SHARE SKIP LOCKED)
		SELECT EXISTS (SELECT 1 FROM activity t JOIN activity m ON m.id = t.source_activity_id
		 WHERE t.id = $1 AND m.id IN (SELECT id FROM meeting) AND `+recordingTranscriptSQL+` AND `+transcriptReadersDriftSQL+`
		 FOR UPDATE OF t SKIP LOCKED)`, p.transcript, p.meeting).Scan(&locked)
	if err != nil {
		return false, fmt.Errorf("activities: locking a transcript and its meeting: %w", err)
	}
	return locked, nil
}

// keepTranscriptReadersTx restores the readers after an audience write on id:
// a transcript takes its meeting's back, and a meeting gives its transcripts
// its new ones. A transcript whose meeting another writer holds is left to
// the sweep rather than waited on, as waiting would take the locks out of order.
func keepTranscriptReadersTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	var meeting *ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT m.id FROM activity t JOIN activity m ON m.id = t.source_activity_id
		 WHERE t.id = $1 AND `+recordingTranscriptSQL+` FOR SHARE OF m SKIP LOCKED`, id).Scan(&meeting)
	if errors.Is(err, pgx.ErrNoRows) {
		return followRecordedMeetingTx(ctx, tx, id)
	}
	if err != nil {
		return fmt.Errorf("activities: reading the meeting a transcript records: %w", err)
	}
	return followMeetingReadersTx(ctx, tx, id, *meeting)
}

// refuseTranscriptAudienceTx refuses an audience set on a meeting's
// transcript: its readers are the meeting's, so the change goes there.
func refuseTranscriptAudienceTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, in SetAudienceInput) error {
	var recording bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity t JOIN activity m ON m.id = t.source_activity_id
		 WHERE t.id = $1 AND `+recordingTranscriptSQL+`)`, id).Scan(&recording)
	if err != nil {
		return fmt.Errorf("activities: reading whether an activity is a meeting's transcript: %w", err)
	}
	if recording {
		return &InvalidAudienceError{Audience: in.Audience, Reason: "meeting notes are read by whoever reads the meeting; change the meeting's audience"}
	}
	return nil
}

// followMeetingReadersTx copies the readers and writes the audit row and the
// activity.updated event, whose audience field is what retracts derived data.
func followMeetingReadersTx(ctx context.Context, tx pgx.Tx, transcript, meeting ids.ActivityID) error {
	before, live, err := transcriptReadersImage(ctx, tx, transcript)
	if err != nil || !live {
		return err
	}
	if err := copyMeetingReadersTx(ctx, tx, transcript, meeting); err != nil {
		return err
	}
	after, _, err := transcriptReadersImage(ctx, tx, transcript)
	if err != nil {
		return err
	}
	changedBefore, changedAfter := storekit.ChangedColumns(before, after)
	if len(changedAfter) == 0 {
		return nil
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "activity", transcript.UUID, changedBefore, changedAfter)
	if err != nil {
		return err
	}
	var audience string
	if err := tx.QueryRow(ctx, `SELECT audience FROM activity WHERE id = $1`, transcript).Scan(&audience); err != nil {
		return fmt.Errorf("activities: reading a transcript's audience: %w", err)
	}
	changed := crmcontracts.PublicEventActivityChangedFieldsAudience(audience)
	return storekit.EmitEvent(ctx, tx, auditID, transcript.UUID, crmcontracts.PublicEventActivityUpdated{
		ChangedFields: crmcontracts.PublicEventActivityChangedFields{Audience: &changed},
	})
}

// transcriptReadersImage is readAudienceImage plus the colleagues among the
// guests. live is false for a transcript that is held or archived, which keeps
// the readers it has.
func transcriptReadersImage(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (image map[string]any, live bool, err error) {
	err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity WHERE id = $1 AND restricted_at IS NULL AND archived_at IS NULL)`,
		id).Scan(&live)
	if err != nil {
		return nil, false, fmt.Errorf("activities: reading whether a transcript is live: %w", err)
	}
	if !live {
		return nil, false, nil
	}
	image, err = readAudienceImage(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT user_id::text FROM activity_participant WHERE activity_id = $1 AND user_id IS NOT NULL`, id)
	if err != nil {
		return nil, false, fmt.Errorf("activities: reading a transcript's colleagues: %w", err)
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, false, fmt.Errorf("activities: reading a transcript's colleagues: %w", err)
	}
	slices.Sort(users)
	image["participant_users"] = users
	return image, true, nil
}
