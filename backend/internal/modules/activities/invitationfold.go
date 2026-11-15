// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// foldedCalendarMailSQL is the calendar mail a meeting stands for in History:
// the organizer's invitation, update or cancellation, and an attendee's plain
// answer. A counter-proposal asks for a decision, so it stays a mail row.
var foldedCalendarMailSQL = "(a.kind = 'email' AND a.calendar_uid IS NOT NULL AND (" +
	calendarInvitationSQL + " OR a.calendar_method = 'REPLY'))"

// foldsInvitations is a record's History read: one row per mail, on one record.
func (in ListActivitiesInput) foldsInvitations() bool {
	return in.CollapseSeatCopies && in.EntityType != nil && in.EntityID != nil
}

// InvitationFoldSQL leaves out the calendar mail whose meeting is itself one
// of the candidates. The inner query names the meeting `a`, so the candidates'
// and content's own terms judge it. Mail that names an occurrence folds onto
// that occurrence only; mail for the whole series folds onto any of them.
// A caller whose content arm is not "TRUE" passes it to keep a readable mail
// from hiding behind a meeting the reader may not open.
func InvitationFoldSQL(candidates, content string) string {
	key := "ai.identity_key"
	return `NOT (` + foldedCalendarMailSQL + ` AND (` + content + `)
	AND (a.calendar_uid, coalesce(a.calendar_event->>'occurrence', '')) IN (
	SELECT ` + meetingUIDOfKeySQL(key) + `, slot.occurrence
	  FROM activity a JOIN activity_identity ai ON ai.activity_id = a.id
	 CROSS JOIN LATERAL (VALUES (''), (right(` + key + `, strpos(reverse(` + key + `), '/') - 1))) slot(occurrence)
	 WHERE ai.identity_kind = '` + IdentityKindMeeting + `' AND a.kind = 'meeting'
	   AND a.archived_at IS NULL AND (` + candidates + `) AND (` + content + `)))`
}

// WithInvitationSummaries stamps each readable calendar meeting on a folded
// page with who was invited and how they answered. It reads only the meeting's
// own participants, so it says nothing about mail the reader may not open.
func WithInvitationSummaries(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	at := map[ids.UUID]int{}
	meetings := []ids.UUID{}
	for i, row := range page {
		if row.Kind == crmcontracts.ActivityKindMeeting && contentReadable(row) {
			at[ids.UUID(row.Id)] = i
			meetings = append(meetings, ids.UUID(row.Id))
		}
	}
	if len(meetings) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT mt.id,
		       count(*) FILTER (WHERE p.id IS NOT NULL),
		       count(*) FILTER (WHERE p.response = 'accepted'),
		       count(*) FILTER (WHERE p.response = 'declined'),
		       count(*) FILTER (WHERE p.response = 'tentative')
		  FROM activity mt
		  LEFT JOIN activity_participant p ON p.activity_id = mt.id AND `+auth.ParticipantStandsSQL("p", "mt")+`
		 WHERE mt.id = ANY($1) AND EXISTS (
		       SELECT 1 FROM activity_identity ai WHERE ai.activity_id = mt.id AND ai.identity_kind = '`+IdentityKindMeeting+`')
		 GROUP BY mt.id`, meetings)
	if err != nil {
		return fmt.Errorf("activities: reading the invitations folded onto meetings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var summary crmcontracts.MeetingInvitationSummary
		if err := rows.Scan(&id, &summary.Invited, &summary.Accepted, &summary.Declined, &summary.Tentative); err != nil {
			return fmt.Errorf("activities: reading the invitations folded onto meetings: %w", err)
		}
		page[at[id]].Invitation = &summary
	}
	return rows.Err()
}
