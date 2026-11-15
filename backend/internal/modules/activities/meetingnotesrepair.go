// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const unlinkedMeetingNotes = `t.kind = 'meeting' AND t.source_system = 'transcript'
 AND t.source = 'meeting_notes' AND t.captured_by = 'connector:gdocs' AND t.source_activity_id IS NULL
 AND t.archived_at IS NULL AND t.restricted_at IS NULL
 `

const liveNotesMeeting = `m.kind = 'meeting' AND m.source_system IS DISTINCT FROM 'transcript'
 AND m.archived_at IS NULL AND m.restricted_at IS NULL`

type notesToRepair struct {
	Transcript  ids.ActivityID
	MeetingText string
	URL         string
}

// RepairMeetingNotesTx connects stored automatic notes through the same outcome engine as a landing.
func (s *Store) RepairMeetingNotesTx(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := activitiesToInspectTx(ctx, tx, in)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	args := []any{}
	arg := func(value any) int { args = append(args, value); return len(args) }
	pageArg := fmt.Sprintf("$%d", arg(page))
	scope, err := auth.ActivityContentClause(ctx, "t", arg)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id, t.raw -> 'google_doc' ->> 'meeting_id', t.source_id FROM activity t
 WHERE t.id = ANY(`+pageArg+`::uuid[]) AND `+unlinkedMeetingNotes+` AND `+scope+` AND t.raw -> 'google_doc' ->> 'meeting_id' IS NOT NULL ORDER BY t.id`, args...)
	if err != nil {
		return datatask.BatchOut{}, fmt.Errorf("activities: reading notes to connect: %w", err)
	}
	notes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[notesToRepair])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, note := range notes {
		connected, err := s.connectMeetingNotesTx(ctx, tx, note)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if connected {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, page, changed)
}

func (s *Store) connectMeetingNotesTx(ctx context.Context, tx pgx.Tx, note notesToRepair) (bool, error) {
	target, err := ids.ParseAs[ids.ActivityKind](note.MeetingText)
	if err != nil {
		return false, nil //nolint:nilerr // Invalid historical metadata cannot prove a meeting, so the batch advances without linking it.
	}
	if err := storekit.LockWriteIdentity(ctx, tx, "activity_source", transcriptSourceSystem+":"+note.URL); err != nil {
		return false, err
	}
	var meetingID ids.ActivityID
	err = tx.QueryRow(ctx, `SELECT m.id FROM activity t JOIN activity m ON m.id = @recorded_meeting
 WHERE t.id = @transcript AND `+unlinkedMeetingNotes+` AND `+liveNotesMeeting+` AND t.source_id = @url AND t.raw -> 'google_doc' ->> 'meeting_id' = @stored_meeting FOR UPDATE OF m,t`, pgx.NamedArgs{"recorded_meeting": target, "transcript": note.Transcript, "url": note.URL, "stored_meeting": note.MeetingText}).Scan(&meetingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: checking notes to connect: %w", err)
	}
	input := LogActivityInput{recordsMeeting: &meetingID, keepRecordedOutcome: true}
	meeting, err := admitRecordedMeeting(ctx, tx, &input)
	if err != nil {
		return false, err
	}
	at := s.now()
	transcript, err := updateActivityInTx(ctx, tx, note.Transcript, UpdateActivityInput{sourceActivityID: &meetingID.UUID, recordsMeeting: true, recordedAt: &at})
	if err != nil {
		return false, err
	}
	if input.recordedMeetingWritable && owesOutcome(meeting) {
		marked, err := markRecordedMeetingHeld(ctx, tx, meeting, transcript, LogActivityInput{recordedAt: &at})
		if err != nil {
			return false, err
		}
		if err := s.askLeadsToQualifyTx(ctx, tx, marked); err != nil {
			return false, err
		}
	}
	return true, nil
}
