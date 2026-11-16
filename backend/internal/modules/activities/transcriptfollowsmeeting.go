// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A transcript that records a meeting names it in source_activity_id, and the
// meeting's readers are its readers: the same audience and reason, the same
// selected members, and the same colleagues among its guests.

// recordingTranscriptSQL is a live transcript, alias t, of a meeting, alias m.
const recordingTranscriptSQL = `t.source_system = '` + transcriptSourceSystem + `' AND t.source_activity_id = m.id
	AND m.kind = '` + KindMeeting + `' AND t.archived_at IS NULL AND t.restricted_at IS NULL`

// transcriptReadersDriftSQL holds when transcript t no longer has the readers
// of meeting m.
const transcriptReadersDriftSQL = `(t.audience IS DISTINCT FROM m.audience OR t.audience_reason IS DISTINCT FROM m.audience_reason
	OR EXISTS ((SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = t.id)
	           EXCEPT ALL SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = m.id)
	OR EXISTS ((SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = m.id)
	           EXCEPT ALL SELECT subject_type, subject_id FROM activity_audience_member WHERE activity_id = t.id)
	OR EXISTS (SELECT user_id FROM activity_participant WHERE activity_id = t.id AND user_id IS NOT NULL
	           EXCEPT SELECT user_id FROM activity_participant WHERE activity_id = m.id)
	OR EXISTS (SELECT user_id FROM activity_participant WHERE activity_id = m.id AND user_id IS NOT NULL
	           EXCEPT SELECT user_id FROM activity_participant WHERE activity_id = t.id))`

// copyMeetingReadersTx gives transcript the readers of meeting.
func copyMeetingReadersTx(ctx context.Context, tx pgx.Tx, transcript, meeting ids.ActivityID) error {
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
	// Only a colleague's row admits a reader, so only those leave with the meeting.
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_participant p WHERE p.activity_id = $1 AND p.user_id IS NOT NULL
		   AND NOT EXISTS (SELECT 1 FROM activity_participant m WHERE m.activity_id = $2 AND m.user_id = p.user_id)`,
		transcript, meeting); err != nil {
		return fmt.Errorf("activities: removing the colleagues a meeting no longer has: %w", err)
	}
	return nil
}

// followRecordedMeetingTx gives each live transcript of meeting the meeting's
// readers as they are now.
func followRecordedMeetingTx(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID) error {
	rows, err := tx.Query(ctx, `SELECT t.id FROM activity t JOIN activity m ON m.id = $1
		 WHERE `+recordingTranscriptSQL+` AND `+transcriptReadersDriftSQL+` ORDER BY t.id FOR UPDATE OF t`, meeting)
	if err != nil {
		return fmt.Errorf("activities: reading the transcripts of a meeting: %w", err)
	}
	transcripts, err := pgx.CollectRows(rows, pgx.RowTo[ids.ActivityID])
	if err != nil {
		return fmt.Errorf("activities: reading the transcripts of a meeting: %w", err)
	}
	for _, transcript := range transcripts {
		if err := followMeetingReadersTx(ctx, tx, transcript, meeting); err != nil {
			return err
		}
	}
	return nil
}

// FollowDriftedTranscriptsTx gives up to limit transcripts whose meeting's
// readers changed the readers they now have. It catches every writer of a
// meeting's guests, including those outside this module. It returns how many
// it changed.
func FollowDriftedTranscriptsTx(ctx context.Context, tx pgx.Tx, limit int) (int, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id, m.id FROM activity t JOIN activity m ON m.id = t.source_activity_id
		 WHERE `+recordingTranscriptSQL+` AND `+transcriptReadersDriftSQL+`
		 ORDER BY t.id LIMIT $1 FOR UPDATE OF t SKIP LOCKED`, limit)
	if err != nil {
		return 0, fmt.Errorf("activities: finding the transcripts whose meeting's readers changed: %w", err)
	}
	type pair struct{ transcript, meeting ids.ActivityID }
	drifted, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pair, error) {
		var p pair
		return p, row.Scan(&p.transcript, &p.meeting)
	})
	if err != nil {
		return 0, fmt.Errorf("activities: finding the transcripts whose meeting's readers changed: %w", err)
	}
	for _, p := range drifted {
		if err := followMeetingReadersTx(ctx, tx, p.transcript, p.meeting); err != nil {
			return 0, err
		}
	}
	return len(drifted), nil
}

// followMeetingReadersTx copies the readers and writes the audit row and the
// activity.updated event, whose audience field is what retracts derived data.
func followMeetingReadersTx(ctx context.Context, tx pgx.Tx, transcript, meeting ids.ActivityID) error {
	before, err := transcriptReadersImage(ctx, tx, transcript)
	if err != nil {
		return err
	}
	if err := copyMeetingReadersTx(ctx, tx, transcript, meeting); err != nil {
		return err
	}
	after, err := transcriptReadersImage(ctx, tx, transcript)
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

// transcriptReadersImage is readAudienceImage plus the colleagues among the guests.
func transcriptReadersImage(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (map[string]any, error) {
	image, err := readAudienceImage(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT user_id::text FROM activity_participant WHERE activity_id = $1 AND user_id IS NOT NULL`, id)
	if err != nil {
		return nil, fmt.Errorf("activities: reading a transcript's colleagues: %w", err)
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("activities: reading a transcript's colleagues: %w", err)
	}
	slices.Sort(users)
	image["participant_users"] = users
	return image, nil
}
