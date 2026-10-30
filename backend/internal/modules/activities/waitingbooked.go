// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// bookedMeetingSQL is when the soonest booked meeting with this message's
// sender starts, or NULL. It turns a waiting card into "meeting booked".
//
// Booked means the sender's contact is on the meeting, linked or attending. It
// means the meeting is not over (occurredday.EndSQL; canceled and no-show
// count as over). It was also booked or moved since the message arrived. The
// write shape says so: created after the ask, or its start changed after it.
//
// The meeting is workspace-audience and unrestricted, and fenced by the
// reader's own discover clause on `booked`. This prints a date, and a date off
// a meeting the reader may not discover would be the disclosure itself.
//
// OFFSET 0 keeps the planner walking from the sender to their meetings. The
// audit probe rides idx_audit_entity_narrow.
func bookedMeetingSQL(asOf, discover string) string {
	return `(SELECT min(booked.occurred_at)
	   FROM activity_participant booked_asker
	  CROSS JOIN LATERAL (
	        SELECT booked_link.activity_id FROM activity_link booked_link
	         WHERE booked_link.contact_id = booked_asker.contact_id
	        UNION
	        SELECT booked_att.activity_id FROM activity_participant booked_att
	         WHERE booked_att.contact_id = booked_asker.contact_id
	        OFFSET 0) booked_walk
	  CROSS JOIN LATERAL (
	        SELECT booked.occurred_at FROM activity booked
	         WHERE booked.id = booked_walk.activity_id
	           AND booked.kind = '` + KindMeeting + `'
	           AND booked.archived_at IS NULL
	           AND booked.restricted_at IS NULL` + auth.AudienceWorkspaceOnly("booked") + `
	           AND (` + discover + `)
	           AND coalesce(booked.meeting_status, '') NOT IN ('canceled', 'no_show')
	           AND ` + occurredday.EndSQL("booked") + ` > ` + asOf + `
	           AND (booked.created_at > a.occurred_at -- occurred_at raw: booked after the ask, an ordering of rows
	             OR EXISTS (SELECT 1 FROM audit_log booked_move
	                 WHERE booked_move.entity_type = 'activity'
	                   AND booked_move.entity_id = booked.id
	                   AND booked_move.after ? 'occurred_at'
	                   -- The START moved, not merely a field beside it: the
	                   -- move writer images occurred_at on every change, a
	                   -- length-only edit included.
	                   AND (booked_move.before ->> 'occurred_at')
	                       IS DISTINCT FROM (booked_move.after ->> 'occurred_at')
	                   AND booked_move.occurred_at > a.occurred_at -- occurred_at raw: moved after the ask, an ordering of rows
	                   AND booked_move.occurred_at <= ` + asOf + `))
	        OFFSET 0) booked
	  WHERE booked_asker.activity_id = a.id AND booked_asker.role = 'from'
	    AND booked_asker.contact_id IS NOT NULL)`
}
