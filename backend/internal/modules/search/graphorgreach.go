// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Which activities an ACCOUNT's context walk reads.

import "fmt"

// orgArms is the three links themselves — the account an activity is filed
// against, the account its deal belongs to, and the employer of the contact it
// is about.
//
// The deal arm deliberately does not exclude archived or lost deals: a set
// stricter than the predicate would show a message on the timeline whose
// account never gets a signal about it.
const orgArms = `FROM activity_link l
		    LEFT JOIN deal d ON d.id = l.deal_id
		    LEFT JOIN relationship r ON r.person_id = l.person_id AND r.kind = 'employment'
		      AND r.ended_at IS NULL AND r.archived_at IS NULL`

// activityReachesOrg is "this activity belongs to the account", for a query
// that aliases activity as a.
//
// A COMPANY IS NOT SOMEBODY YOU CAN MEET. A meeting or a call is refused a
// direct organization link (migration 1788000100), so a flat
// `activity_link.organization_id` match — which is what this walk used to be —
// reads an account's timeline with every meeting missing from it. The same was
// already true of captured mail, which capture files against the PERSON it was
// with: an account's busiest correspondence carried no organization link at all
// and this walk never saw it.
//
// activities.OrgLinkedActivityExists is the same three arms for the timeline
// list, the account view and the roll-up. A module never imports a sibling
// (ADR-0054), so this is its deliberate copy, held against it by
// TestTheAccountReachWalkIsOneAnswer rather than by anybody remembering.
//
// It stays an EXISTS rather than a join against a reach set: EXISTS stops at
// the first arm that matches, and this is a hot read.
//
// orgPos is the bind position carrying the organization id; every arm reads the
// same one.
func activityReachesOrg(orgPos int) string {
	operand := fmt.Sprintf("$%d", orgPos)
	return fmt.Sprintf(`EXISTS (
		    SELECT 1 %s
		    WHERE l.activity_id = a.id
		      AND (l.organization_id = %[2]s OR d.organization_id = %[2]s OR r.organization_id = %[2]s))`,
		orgArms, operand)
}
