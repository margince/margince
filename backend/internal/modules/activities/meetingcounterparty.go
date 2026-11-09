// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
)

// HeldMeetingCounterparties is the body of a derived table: one
// (activity_id, company_id) row per held meeting and current employer of each
// outside participant. Deal Scout reads it. Only a recorded held outcome
// counts (meetingoutcome.HeldSQL); a past meeting with no status is awaiting
// its outcome, not held.
//
// Outside means no seat, no colleague and no own-company employer. Without a
// transaction, colleagueParticipantSQL runs without the proved-mailbox arm.
func HeldMeetingCounterparties() string {
	return `SELECT DISTINCT ap.activity_id, emp.company_id
		  FROM activity_participant ap
		  JOIN activity m ON m.id = ap.activity_id AND m.kind = 'meeting'
		   AND ` + meetingoutcome.HeldSQL("m") + `
		  JOIN contact pc ON pc.id = ap.contact_id AND pc.archived_at IS NULL
		  JOIN relationship emp ON emp.contact_id = ap.contact_id AND emp.kind = 'employment'
		    AND ` + employment.IsCurrentSQL("emp.ended_at") + ` AND emp.archived_at IS NULL
		  JOIN company co ON co.id = emp.company_id AND NOT co.is_anchor AND co.archived_at IS NULL
		 WHERE ap.user_id IS NULL AND NOT ` + calendarResourceParticipantSQL(scopeUnbounded) + ` AND NOT ` + colleagueParticipantSQL("", scopeUnbounded)
}
