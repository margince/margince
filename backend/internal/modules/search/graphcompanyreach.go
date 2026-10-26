// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/relationshipmark"
)

// companyArms is the three links themselves — the account an activity is filed
// against, the account its deal belongs to, and the employer of the contact it
// is about.
//
// The deal arm deliberately does not exclude archived or lost deals: a set
// stricter than the predicate would show a message on the timeline whose
// account never gets a signal about it.
//
// The employment arm is bounded by when the message was sent. The set form has
// no outer activity to read that from, so the arms join the activity.
var companyArms = `FROM activity_link l
		    JOIN activity la ON la.id = l.activity_id
		    LEFT JOIN deal d ON d.id = l.deal_id
		    LEFT JOIN relationship r ON r.contact_id = l.contact_id AND r.kind = 'employment'
		      AND ` + employment.IsCurrentSQL("r.ended_at") + ` AND r.archived_at IS NULL
		      AND ` + employment.InPlaceAtSQL("r", "la.occurred_at") + `
		      AND ` + relationshipmark.ContactNotMarkedSQL("r.contact_id")

// participantEmployerArm is the fourth arm: the employer of somebody who is on
// the event as a participant rather than as a link. Without it a meeting whose
// only contact is on the invitation reaches no company at all, since a meeting
// may carry no direct company link.
//
// activities.participantEmployerArm is the same text, held equal to this one by
// TestTheAccountReachWalkIsOneAnswer. It is a constant of its own rather than
// part of companyArms because the two modules also share companyArms with a producer
// that deliberately stops at three arms (activities.CompanyReachSet says why).
var participantEmployerArm = `EXISTS (
		    SELECT 1 FROM activity_participant ap
		      JOIN relationship emp ON emp.contact_id = ap.contact_id AND emp.kind = 'employment'
		        AND ` + employment.IsCurrentSQL("emp.ended_at") + ` AND emp.archived_at IS NULL
		        AND ` + employment.InPlaceAtSQL("emp", "a.occurred_at") + `
		        AND ` + relationshipmark.ContactNotMarkedSQL("emp.contact_id") + `
		    WHERE ap.activity_id = a.id AND emp.company_id = %s)`

// participantEntry is where the participant branch starts: every activity an
// employee of the account is on, for participantEmployerArm to decide.
var participantEntry = `SELECT a.id, e0.company_id FROM relationship e0
		    JOIN activity_participant p0 ON p0.contact_id = e0.contact_id
		    JOIN activity a ON a.id = p0.activity_id
		    WHERE e0.company_id = %s AND e0.kind = 'employment'
		      AND ` + relationshipmark.ContactNotMarkedSQL("e0.contact_id") + ` AND `

// activityReachesCompany is "this activity belongs to the account", for a query
// that aliases activity as a.
//
// A COMPANY IS NOT SOMEBODY YOU CAN MEET. A meeting or a call is refused a
// direct company link (migration 1788000100), so a flat
// `activity_link.company_id` match — which is what this walk used to be —
// reads an account's timeline with every meeting missing from it. The same was
// already true of captured mail, which capture files against the CONTACT it was
// with: an account's busiest correspondence carried no company link at all
// and this walk never saw it.
//
// activities.CompanyLinkedActivityExists is the same arms for the timeline list,
// the account view and the roll-up. A module never imports a sibling
// (ADR-0054), so this is its deliberate copy, held against it by
// TestTheAccountReachWalkIsOneAnswer rather than by anybody remembering.
//
// It is a membership test against one branch per arm, the shape the activities
// copy has. Correlated `EXISTS` joined by `OR` leave Postgres walking activity
// newest-first, so a quiet account costs the whole table.
//
// companyPos is the bind position carrying the company id; every arm reads the
// same one.
func activityReachesCompany(companyPos int) string {
	operand := fmt.Sprintf("$%d", companyPos)
	linkBranch := func(column string) string {
		return fmt.Sprintf("SELECT l.activity_id, %[1]s %[2]s\n\t\t    WHERE %[1]s = %[3]s", column, companyArms, operand)
	}
	participantBranch := fmt.Sprintf(participantEntry+participantEmployerArm, operand, "e0.company_id")
	return "a.id IN (SELECT reach.id FROM (" + linkBranch("l.company_id") +
		"\n\t\t    UNION " + linkBranch("d.company_id") + "\n\t\t    UNION " + linkBranch("r.company_id") +
		"\n\t\t    UNION " + participantBranch + ") reach(id, company_id))"
}
