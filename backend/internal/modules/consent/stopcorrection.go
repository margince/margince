// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which stop a seat may take back. The lift and the list of stops both ask
// here, so the button a reader sees and the door behind it agree.
//
// The ladder in commsauthz answers most of it. A marketing objection is stamped
// at the subject's level whoever typed it (levelFor). An admin may correct one
// a colleague typed by mistake. One the person pressed on a mailed link stays
// theirs: the product, not a seat, wrote its first row.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// kindProcessingRestriction is the Art. 18 stop. Lifting one owes the person
// notice before it happens, and Margince has no such notice, so no seat lifts
// one here at any level.
const kindProcessingRestriction = "processing_restriction"

// stopFacts is what the lift rule reads off one stop.
type stopFacts struct {
	kind    string
	decided commsauthz.AuthorityLevel
	// origin is who captured the first row of the stop's carry chain. A merge
	// copies a stop and stamps the merger, so the copy's own captured_by would
	// make the person's own press look typed by a colleague.
	origin string
	// pinned is a stop held on an address rather than on the contact. It
	// clears when the address works again, not through this lift.
	pinned bool
}

// liftVerdict answers whether the caller may lift a stop, and why not.
type liftVerdict struct {
	may bool
	// correction is an admin taking back an objection a colleague recorded.
	correction bool
	blockedBy  crmcontracts.ContactStopLiftBlockedBy
}

// judgeLift is the one answer to "may this caller lift this stop".
func judgeLift(caller commsauthz.AuthorityLevel, stop stopFacts) liftVerdict {
	switch {
	case stop.pinned:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByAddress}
	// Before the ladder: no level lifts a restriction without the notice.
	case stop.kind == kindProcessingRestriction:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByRestriction}
	case caller.CanOverrule(stop.decided):
		return liftVerdict{may: true}
	case caller == commsauthz.LevelAdmin && typedObjection(stop):
		return liftVerdict{may: true, correction: true}
	case stop.decided == commsauthz.LevelSubject:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedBySubjectDecision}
	}
	return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByLevel}
}

// typedObjection reports a subject-level objection that a seat recorded.
func typedObjection(stop stopFacts) bool {
	return stop.kind == commsauthz.ReasonObjection &&
		stop.decided == commsauthz.LevelSubject &&
		!principal.SystemMintedID(stop.origin)
}

// originCapturedBySQL reads who captured the first row of the carry chain that
// ends at the suppression aliased `alias`. A carried row always points at an
// older row, so the walk ends.
func originCapturedBySQL(alias string) string {
	return `(WITH RECURSIVE chain AS (
		    SELECT c.carried_from, c.captured_by, 0 AS depth
		      FROM communication_suppression c WHERE c.id = ` + alias + `.id
		    UNION ALL
		    SELECT p.carried_from, p.captured_by, chain.depth + 1
		      FROM communication_suppression p JOIN chain ON p.id = chain.carried_from)
		  SELECT captured_by FROM chain ORDER BY depth DESC LIMIT 1)`
}
