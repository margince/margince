// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// HeldMeetingCounterparties is the body of a derived table with one
// (activity_id, company_id) row per held meeting, by relstrength.MeetingHeldSQL,
// and the current employer of each outside participant on it. Deal Scout reads
// it: a meeting held with somebody at a company is evidence about that company.
//
// It is its own walk, not CompanyReachSet. That set deliberately files nothing
// through participants, because a signal filed against a Cc'd contact's
// employer is a claim nobody made. A meeting is different: the contacts who
// sat in it are the counterparty.
//
// "Outside" means a contact who is not a seat (user_id IS NULL) and whose
// employer is not the installation's own company. A meeting among colleagues
// reaches no company here.
func HeldMeetingCounterparties() string {
	return `SELECT DISTINCT ap.activity_id, emp.company_id
		  FROM activity_participant ap
		  JOIN activity m ON m.id = ap.activity_id AND m.kind = 'meeting'
		   AND ` + relstrength.MeetingHeldSQL("m", "now()") + `
		  JOIN contact pc ON pc.id = ap.contact_id AND pc.archived_at IS NULL
		  JOIN relationship emp ON emp.contact_id = ap.contact_id AND emp.kind = 'employment'
		    AND ` + employment.IsCurrentSQL("emp.ended_at") + ` AND emp.archived_at IS NULL
		  JOIN company co ON co.id = emp.company_id AND NOT co.is_anchor AND co.archived_at IS NULL
		 WHERE ap.user_id IS NULL`
}
