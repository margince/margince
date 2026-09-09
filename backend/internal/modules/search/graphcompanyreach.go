// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/employment"
)

// companyArms is the three links themselves — the account an activity is filed
// against, the account its deal belongs to, and the employer of the contact it
// is about.
//
// The deal arm deliberately does not exclude archived or lost deals: a set
// stricter than the predicate would show a message on the timeline whose
// account never gets a signal about it.
var companyArms = `FROM activity_link l
		    LEFT JOIN deal d ON d.id = l.deal_id
		    LEFT JOIN relationship r ON r.person_id = l.person_id AND r.kind = 'employment'
		      AND ` + employment.IsCurrentSQL("r.ended_at") + ` AND r.archived_at IS NULL`

// participantEmployerArm is the fourth arm: the employer of somebody who is on
// the event as a participant rather than as a link. Without it a meeting whose
// only person is on the invitation reaches no company at all, since a meeting
// may carry no direct company link.
//
// activities.participantEmployerArm is the same text, held equal to this one by
// TestTheAccountReachWalkIsOneAnswer. It is a constant of its own rather than
// part of companyArms because the two modules also share companyArms with a producer
// that deliberately stops at three arms (activities.CompanyReachSet says why).
var participantEmployerArm = `EXISTS (
		    SELECT 1 FROM activity_participant ap
		      JOIN relationship emp ON emp.person_id = ap.person_id AND emp.kind = 'employment'
		        AND ` + employment.IsCurrentSQL("emp.ended_at") + ` AND emp.archived_at IS NULL
		    WHERE ap.activity_id = a.id AND emp.company_id = %s)`

// activityReachesCompany is "this activity belongs to the account", for a query
// that aliases activity as a.
//
// A COMPANY IS NOT SOMEBODY YOU CAN MEET. A meeting or a call is refused a
// direct company link (migration 1788000100), so a flat
// `activity_link.company_id` match — which is what this walk used to be —
// reads an account's timeline with every meeting missing from it. The same was
// already true of captured mail, which capture files against the PERSON it was
// with: an account's busiest correspondence carried no company link at all
// and this walk never saw it.
//
// activities.CompanyLinkedActivityExists is the same arms for the timeline list,
// the account view and the roll-up. A module never imports a sibling
// (ADR-0054), so this is its deliberate copy, held against it by
// TestTheAccountReachWalkIsOneAnswer rather than by anybody remembering.
//
// It stays an EXISTS rather than a join against a reach set: EXISTS stops at
// the first arm that matches, and this is a hot read.
//
// companyPos is the bind position carrying the company id; every arm reads the
// same one.
func activityReachesCompany(companyPos int) string {
	operand := fmt.Sprintf("$%d", companyPos)
	linked := fmt.Sprintf(`EXISTS (
		    SELECT 1 %s
		    WHERE l.activity_id = a.id
		      AND (l.company_id = %[2]s OR d.company_id = %[2]s OR r.company_id = %[2]s))`,
		companyArms, operand)
	return "(" + linked + "\n\t\t    OR " + fmt.Sprintf(participantEmployerArm, operand) + ")"
}
