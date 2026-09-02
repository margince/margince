// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/platform/mailrole"

// addressIsARoleMailbox settles a sender by its address, after the mailbox
// owner's own decision and before any model call.
//
// `support@`, `billing@` and a helpdesk vendor's ticket address name a queue
// rather than a person. The correspondence is real — somebody answers, and the
// mail stays visible — but there is no human to name, and the small local model
// this lane runs on answered `person` for exactly these often enough to put
// contacts called "Billing" and "support" in a founder's CRM.
//
// Deterministic on purpose. A question with a right answer that can be read off
// the address should not be spent on a model call whose answer varies, and the
// ledger then settles the address so later mail from the same queue costs
// nothing either.
//
// It sits BELOW the owner's override in judgeOne: a person who tells this
// product that a shared mailbox is a contact they want has answered the
// question, and a rule that overruled them would make the correction temporary.
//
// The vocabulary is platform/mailrole, shared with the tier ladder and with
// people's name parser, so the three doors give one answer for one address.
//
// Held by: TestOnlyOnePackageDeclaresRoleMailboxes (backend/gates/rolemailboxonelist_test.go)
func addressIsARoleMailbox(email string) bool {
	_, role := mailrole.Match(email)
	return role
}
