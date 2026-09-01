// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Filing a captured MEETING under the people who attended it, for the ordering
// where the meeting synced before the attendee was a contact.
//
// Its own file because it is its own concept: the rest of the cohort repair is
// defined by an ADDRESS — counterparty_email, person_email — and a meeting has
// no counterparty at all. This arm is defined by attendance instead.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// maxMeetingLinksPerActivity mirrors activities.maxActivityLinks: an activity
// may be filed under at most 25 records. Spelled again rather than imported
// because a module never imports a sibling — capture.maxDerivedMeetingLinks is
// the third spelling, for the same reason and against the same number.
const maxMeetingLinksPerActivity = 25

// linkAttendedMeetings files a meeting under a person who was IN it.
//
// This is the arm linkCapturedCohort cannot be. That one keys on
// counterparty_email, which is how mail names its other party — and a meeting
// has no counterparty at all, because attendance is a LIST and the calendar
// mapper leaves the field unset. Keying the repair on a column meetings never
// carry is why every synced meeting in a workspace could sit unlinked while the
// sweep reported nothing owed.
//
// So this arm reads what a meeting DOES carry: the participant rows.
// capture.linkResolvedMeetingParticipants does exactly this at capture time, for
// an attendee who was already a contact. Here it is for the other ordering — the
// meeting synced first and the person arrived later — and it is the same three
// rules, because a repair that filed a meeting the capture path would have
// refused is not a repair.
//
// It must run AFTER promoteParticipantsToPerson in the same transaction. The
// promotion is what turns this person's address-only attendee row into a
// person-resolved one, and this arm reads person-resolved rows. Reversed, a
// meeting whose attendee was only ever an address is offered to no arm at all.
//
// PERSON links only, never the company: a meeting may not link straight to an
// organization, and the account reaches it through the attendee's employment.
//
// The ATTENDEE's own merge redirect is resolved, not just the person the repair
// was asked about. A participant row written before the merge repointed them
// still names the retired id; matching on it alone would miss that meeting
// forever, while the selector offered the retired id and the liveness join then
// dropped it — the meeting stays unlinked and the sweep reports a drained
// backlog. capture.meetingParticipantsWithPeople resolves it the same way, for
// the same reason.
//
// An archived meeting is left alone. Linking one would file a record a reader
// cannot open, and emit an activity.updated for it.
//
// The 25-link ceiling is activities.maxActivityLinks — an activity may be filed
// under at most 25 records, whatever wrote them. It is re-spelled rather than
// imported because a module never imports a sibling; what must not drift is the
// NUMBER. A meeting already at the cap is left alone rather than failed: the
// cohort scan carries this same guard, so a row this refuses is not offered
// again on every pass, and the sweep still drains.
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
