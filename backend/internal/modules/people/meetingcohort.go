// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The cohort repair's MEETING arm.
//
// linkCapturedCohort beside it finds work by counterparty_email, and a calendar
// meeting has none: attendance is a LIST, so the mapper leaves that field unset
// and the only thing naming an attendee is a participant row. For a while the
// difference was invisible, because a meeting usually has mail about it and the
// page reads the mail instead — a meeting nobody emailed about was reachable
// from nowhere.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxMeetingLinksPerActivity bounds how many people one meeting is filed under.
//
// An invitation can carry a room full of addresses, and every one of them that
// later becomes a contact would otherwise put another link on the same activity
// — a company all-hands would end up on two hundred records, which is not what
// any of them means by "my meetings".
//
// It is a ceiling on the ACTIVITY rather than on the pass, which is why it is
// not cohortRepairBatch: that one bounds how much work a single repair does and
// the next pass continues, while this one is a statement about the meeting
// itself and the next pass must reach the same answer.
const maxMeetingLinksPerActivity = 25

// linkAttendedMeetings files the connector-captured meetings this person
// attended under them, and answers the activities it linked.
//
// PER PERSON, not per activity. The mail arm refuses an activity that any
// person is already linked to, because attaching a message to a second party
// would relabel somebody's mail. A meeting is the opposite shape — everyone in
// it belongs on it — so the guard here asks whether THIS person is already
// filed under it, or the second attendee would be locked out by the first.
//
// The attendee row is resolved through merged_into_id: a database written
// before the merge repointed participant rows still holds attendee rows naming
// a retired id, and reading them literally files the meeting under a record
// nobody can open.
//
// It runs AFTER the promotion, never before: it reads the person-resolved
// attendee rows that promotion has just written.
func linkAttendedMeetings(ctx context.Context, tx pgx.Tx, personID ids.PersonID) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		SELECT a.id, 'person', $1
		  FROM activity a
		 WHERE a.kind = 'meeting'
		   AND a.captured_by LIKE 'connector:%'
		   AND a.restricted_at IS NULL
		   AND a.archived_at IS NULL
		   AND EXISTS (
		       SELECT 1 FROM activity_participant ap
		         JOIN person att ON att.id = ap.person_id
		        WHERE ap.activity_id = a.id
		          AND coalesce(att.merged_into_id, att.id) = $1)
		   AND NOT EXISTS (
		       SELECT 1 FROM activity_link l
		        WHERE l.activity_id = a.id AND l.person_id = $1)
		   AND (SELECT count(*) FROM activity_link cap
		         WHERE cap.activity_id = a.id) < $3
		 ORDER BY a.occurred_at DESC, a.id
		 LIMIT $2
		ON CONFLICT DO NOTHING
		RETURNING activity_id`, personID, cohortRepairBatch, maxMeetingLinksPerActivity)
	if err != nil {
		return nil, fmt.Errorf("people: filing a person's attended meetings: %w", err)
	}
	defer rows.Close()
	var linked []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("people: filing a person's attended meetings: %w", err)
		}
		linked = append(linked, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: filing a person's attended meetings: %w", err)
	}
	return linked, nil
}
