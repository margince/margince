// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which stop a seat may take back. The lift and the list of stops both ask
// here, so the button a reader sees and the door behind it agree.
//
// The ladder in commsauthz answers most of it. A marketing objection is stamped
// at the subject's level whoever typed it (levelFor). An admin may correct one
// a colleague typed by mistake. One the contact pressed on a mailed link stays
// theirs: the product, not a seat, wrote its first row.

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// kindProcessingRestriction is the Art. 18 stop. Lifting one owes the contact
// notice before it happens, and Margince has no such notice, so no seat lifts
// one here at any level.
const kindProcessingRestriction = "processing_restriction"

// stopFacts is what the lift rule reads off one stop.
type stopFacts struct {
	kind    string
	decided commsauthz.AuthorityLevel
	// origin and originSource are the captured_by and source of the first row
	// of the stop's carry chain. A merge copies a stop and stamps the merger,
	// so the copy's own captured_by would make the contact's press look typed.
	origin       string
	originSource string
	// pinned is a stop held on an address rather than on the contact. It
	// clears when the address works again, not through this lift.
	pinned bool
}

// lifter is who asks to lift a stop.
type lifter struct {
	level commsauthz.AuthorityLevel
	// human is a colleague signed in to a seat. A correction overrules the
	// contact's level, so an agent on their passport does not make one.
	human bool
}

// liftVerdict answers whether the caller may lift a stop, and why not.
type liftVerdict struct {
	may bool
	// correction is an admin taking back an objection a colleague recorded.
	correction bool
	blockedBy  crmcontracts.ContactStopLiftBlockedBy
}

// judgeLift is the one answer to "may this caller lift this stop".
func judgeLift(caller lifter, stop stopFacts) liftVerdict {
	switch {
	case stop.pinned:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByAddress}
	// Before the ladder: no level lifts a restriction without the notice.
	case stop.kind == kindProcessingRestriction:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByRestriction}
	case caller.level.CanOverrule(stop.decided):
		return liftVerdict{may: true}
	case caller.level == commsauthz.LevelAdmin && typedObjection(stop) && !caller.human:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByPermission}
	case caller.level == commsauthz.LevelAdmin && typedObjection(stop):
		return liftVerdict{may: true, correction: true}
	case stop.decided == commsauthz.LevelSubject:
		return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedBySubjectDecision}
	}
	return liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByLevel}
}

// typedObjection reports a subject-level objection a seat typed through
// Suppress. It is an allow-list. An agent, a connector, an import, the product
// and a copy whose original is gone all count as the contact's own act.
func typedObjection(stop stopFacts) bool {
	return stop.kind == commsauthz.ReasonObjection &&
		stop.decided == commsauthz.LevelSubject &&
		strings.HasPrefix(stop.origin, principal.HumanIDPrefix) &&
		recordedThroughSuppress(stop.originSource)
}

// recordedThroughSuppress reports a source suppressionSource wrote. A carried
// copy's source starts with carriedSourceMark, so a copy whose original was
// deleted (carried_from set to NULL) fails here.
func recordedThroughSuppress(source string) bool {
	return source == seatSourcePrefix || strings.HasPrefix(source, seatSourcePrefix+": ")
}

// carriedSourceMark opens the source of every stop a merge copied.
const carriedSourceMark = "carried: "

// originColumnSQL reads one column of the first row of the carry chain that
// ends at the suppression aliased `alias`. A carried row always points at an
// older row, so the walk ends.
func originColumnSQL(alias, column string) string {
	return `(WITH RECURSIVE chain AS (
		    SELECT c.carried_from, c.` + column + ` AS value, 0 AS depth
		      FROM communication_suppression c WHERE c.id = ` + alias + `.id
		    UNION ALL
		    SELECT p.carried_from, p.` + column + `, chain.depth + 1
		      FROM communication_suppression p JOIN chain ON p.id = chain.carried_from)
		  SELECT value FROM chain ORDER BY depth DESC LIMIT 1)`
}
