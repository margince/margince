// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which contacts the sender ledger has already judged private.
//
// Rendered here rather than in the lane that asks, because this module owns
// capture_pending_counterparty and a module never imports a sibling. The caller
// takes it as a hole in its own statement, the way it takes the dismissal rule
// from contacts.

package capture

import "fmt"

// PrivateSenderClause excludes a contact THIS reader's own sender verdict called
// anything but a business counterparty.
//
// A reconnect lane is the case it exists for. `personal` and `advisor` are the
// mailbox owner's own life — a doctor, a lawyer, a school — and `newsletter`,
// `transactional` and `spam` are nobody at all. None of them is a relationship
// with revenue behind it, and putting one under a heading that promises new
// pipeline is how a rep learns to stop reading the heading.
//
// THE READER'S OWN, and that is the half that is easy to leave out. The ledger
// is per mailbox owner: the same address can be one rep's personal adviser and
// another's live customer. Read unscoped, the first rep's private verdict
// silently suppresses the second rep's reconnect — one colleague's privacy
// decision deciding another contact's pipeline.
//
// The owner's OWN correction outranks the machine, both ways. `business`
// readmits a sender the classifier called noise; `keep_out` withdraws one it
// called real. Reading the kind alone leaves a corrected contact excluded
// forever and shows one the owner has asked to be rid of.
//
// alias names the row carrying `contact_id`, and readerPos the placeholder
// holding the reader's own id. The kinds are spelled here rather than derived
// from the Kind constants deliberately: this is a statement about which of them
// are NOT business, and a new kind must be judged rather than silently
// inherited — TestEveryVerdictKindIsJudgedByTheReconnectLane holds it.
func PrivateSenderClause(alias, readerPos string) string {
	return fmt.Sprintf(`NOT EXISTS (
		SELECT 1 FROM contact_email pe
		  LEFT JOIN capture_pending_counterparty cp
		         ON cp.email = lower(pe.email) AND cp.owner_id = %[2]s
		  LEFT JOIN capture_sender_override so
		         ON so.address = lower(pe.email) AND so.user_id = %[2]s
		 WHERE pe.contact_id = %[1]s.contact_id
		   AND pe.archived_at IS NULL
		   AND (so.decision = '%[4]s'
		     OR (so.decision IS DISTINCT FROM '%[3]s'
		         AND cp.kind IN ('personal', 'advisor', 'newsletter', 'transactional', 'spam'))))`,
		alias, readerPos, OverrideBusiness, OverrideKeepOut)
}
