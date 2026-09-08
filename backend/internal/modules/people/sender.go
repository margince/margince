// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import "fmt"

// The identifier columns quick-find matches a pasted address or domain
// against. Named here beside the sender predicate because both answer the same
// question about a record — which real-world handle names it.
const (
	emailColumn  = "email"
	domainColumn = "domain"
	// The FK each identifier table names its record by.
	personFK = "person_id"
	orgFK    = "organization_id"
)

// SenderPredicate renders "this person WROTE this message", as opposed to the
// message merely reaching them.
//
// A thread is linked to everybody it concerns, so activity_link membership
// answers "does this message reach this person" and nothing more. Reading it as
// authorship is what put another sender's title, employer and address onto a
// contact: a mail From Marcus To Judith is linked to both, and the signature at
// its foot is Marcus's whichever of them you asked about.
//
// THE ADDRESS IS THE AUTHORITY WHENEVER THE ROW CARRIES ONE. The backfill that
// filled participants for historical mail (activities.backfillParticipants)
// takes its person_id from the FIRST activity_link and its address from
// activity.counterparty_email, so one row can name one person while carrying
// another's address. Trusting person_id there would re-admit the confusion this
// predicate exists to refuse, so it is read only for a row with no address —
// the shape live capture writes when it resolved a party by channel account
// alone.
//
// A message with NO 'from' participant is not admitted by a fallback to
// activity.counterparty_email. Those rows are machine notifications — drive
// shares, e-signature receipts — whose sender is a robot, and a fallback would
// hand their footers to whichever contact the notification happened to name.
//
// person and activity are SQL expressions the caller supplies: a bind
// parameter ("$3") or a correlated column ("p.id"), and the activity's alias.
//
// A format string rather than a builder function: the census that measures
// unbounded record references reads one function at a time, and a helper
// holding this SQL would count as a read of `person` that reaches no row scope
// — which is true of the fragment and false of every statement it appears in.
func SenderPredicate(person, activity string) string {
	return fmt.Sprintf(senderPredicateSQL, person, activity)
}

// AuthoredPredicate renders the weaker question a reader's own timeline asks:
// "does anything recorded say somebody ELSE wrote this message".
//
// SenderPredicate above demands proof of authorship, and the enrichment pass
// must keep demanding it — a signature block mined off an unproven message is
// the defect that predicate exists to refuse, and absence of evidence there has
// to read as no.
//
// A last-touch fold asks the opposite way round. Only capture writes
// participants (capture/participant.go, and the backfill behind it), so a
// message a rep LOGGED BY HAND carries none at all: no 'from' row exists to
// name the author, and the activity_link the rep created is the whole of what
// anybody recorded about who this message was with. Judged by SenderPredicate,
// every hand-logged inbound message stops counting as "they wrote" — which on
// an installation with no mailbox connected is every message there is.
//
// So the arm is added for a message with NO participant row whatsoever, and
// deliberately not for a message that has some. A captured mail always carries
// its parties, machine notifications included: those have a 'from' row naming a
// robot, they fail the strict test on its merits, and this predicate leaves
// them refused. The confusion stays refused too — Marcus's mail to Judith
// carries Marcus as its sender, so it is Marcus who wrote last and not Judith,
// which is the case TestLastInboundNamesTheAuthorAndNotEverybodyOnTheThread
// holds in both directions.
func AuthoredPredicate(person, activity string) string {
	return fmt.Sprintf(`(%s OR NOT EXISTS (
		SELECT 1 FROM activity_participant ap WHERE ap.activity_id = %s.id))`,
		SenderPredicate(person, activity), activity)
}

const senderPredicateSQL = `EXISTS (
		SELECT 1 FROM activity_participant ap
		 WHERE ap.activity_id = %[2]s.id AND ap.role = 'from'
		   AND CASE WHEN ap.address IS NOT NULL
		            THEN EXISTS (SELECT 1 FROM person_email pe
		                          WHERE pe.person_id = %[1]s AND pe.archived_at IS NULL
		                            AND pe.email = ap.address)
		            ELSE ap.person_id = %[1]s END)`
