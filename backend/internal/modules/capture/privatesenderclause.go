// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which contacts the sender ledger has already judged private.
//
// Rendered here rather than in the lane that asks, because this module owns
// capture_pending_counterparty and a module never imports a sibling. The caller
// takes it as a hole in its own statement, the way it takes the dismissal rule
// from people.

package capture

import "fmt"

// PrivateSenderClause excludes a person the sender verdict called anything but
// a business counterparty.
//
// A reconnect lane is the case it exists for. `personal` and `advisor` are the
// mailbox owner's own life — a doctor, a lawyer, a school — and `newsletter`,
// `transactional` and `spam` are nobody at all. None of them is a relationship
// with revenue behind it, and putting one under a heading that promises new
// pipeline is how a rep learns to stop reading the heading.
//
// alias names the row carrying `person_id`. The kinds are spelled here rather
// than derived from the Kind constants deliberately: this is a statement about
// which of them are NOT business, and a new kind must be judged rather than
// silently inherited — TestEveryVerdictKindIsJudgedByTheReconnectLane holds it.
func PrivateSenderClause(alias string) string {
	return fmt.Sprintf(`NOT EXISTS (
		SELECT 1 FROM capture_pending_counterparty cp
		  JOIN person_email pe ON lower(pe.email) = cp.email
		 WHERE pe.person_id = %s.person_id
		   AND cp.kind IN ('personal', 'advisor', 'newsletter', 'transactional', 'spam'))`, alias)
}
