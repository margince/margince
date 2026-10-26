// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/shared/kernel/correspondence"

// sharedCorrespondentSQL is the test that row is in anchor's thread and was with
// someone anchor was with. That is the same counterparty, a contact on both, or
// one message's counterparty on the other's headers. A seat is never a
// counterparty and carries no contact, so our own mailbox joins no conversations.
func sharedCorrespondentSQL(row, anchor string) string {
	return `((` + correspondence.ConversationSQL(row, anchor) + `)
     OR (` + correspondence.ThreadSQL(row, anchor) + `
       AND (EXISTS (SELECT 1 FROM activity_participant shared_row
              JOIN activity_participant shared_anchor ON shared_anchor.contact_id = shared_row.contact_id
             WHERE shared_row.activity_id = ` + row + `.id AND shared_anchor.activity_id = ` + anchor + `.id)
         OR ` + counterpartyNamedOnSQL(row, anchor) + `
         OR ` + counterpartyNamedOnSQL(anchor, row) + `)))`
}

// counterpartyNamedOnSQL is whether message's counterparty is an address on
// other's headers. counterparty_email is stored folded; a header address may not be.
func counterpartyNamedOnSQL(message, other string) string {
	return `EXISTS (SELECT 1 FROM activity_participant named
          WHERE named.activity_id = ` + other + `.id AND lower(named.address) = ` + message + `.counterparty_email)`
}
