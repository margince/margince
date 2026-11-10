// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package relationshipmark is the vocabulary of contact.relationship_mark and
// the one place the "not marked" predicate is spelled.
//
// A mark says the workspace's relationship to a contact is not an ordinary one.
// Every reader that turns contacts into relationship signals applies NotMarkedSQL.
// A second mark added later then reaches all of them at once.
package relationshipmark

import (
	"fmt"
	"slices"
)

// Mark is a value of contact.relationship_mark.
//
// Held by: TestTheRelationshipMarkVocabularyIsTheCatalogsCheck
// (backend/gates/relationshipmarkvocabulary_test.go), which fails when this set
// and the column's CHECK stop naming the same values.
type Mark string

// VendorMarketing is a named sender who only pitches something to the business
// and whom nobody in the workspace has answered.
const VendorMarketing Mark = "vendor_marketing"

// Values lists every mark the column accepts. NULL is not among them: it means
// the contact is not marked.
func Values() []Mark {
	return []Mark{VendorMarketing}
}

// Valid reports whether m is a mark the column accepts.
func Valid(m Mark) bool {
	return slices.Contains(Values(), m)
}

// NotMarkedSQL is the predicate that the contact aliased alias carries no mark.
// The alias is a compile-time SQL alias, never request input.
func NotMarkedSQL(alias string) string {
	return fmt.Sprintf("%s.relationship_mark IS NULL", alias)
}

// ContactNotMarkedSQL is the predicate that the contact whose id is the SQL
// expression idExpr carries no mark. It is for a join that holds a contact id
// and no contact row. The expression is compile-time SQL, never request input.
func ContactNotMarkedSQL(idExpr string) string {
	return fmt.Sprintf("EXISTS (SELECT 1 FROM contact mk WHERE mk.id = %s AND %s)", idExpr, NotMarkedSQL("mk"))
}

// SenderMayRaiseWorkSQL excludes marked senders even when mail also links a company.
//
// The marked addresses are one uncorrelated ARRAY, which Postgres reads once
// per statement. Joined per message instead, a sender's address matched
// contact_email through an expression no index holds, so every message
// scanned the whole table.
func SenderMayRaiseWorkSQL(activity string) string {
	return `NOT EXISTS (SELECT 1 FROM activity_participant marked_sender
      WHERE marked_sender.activity_id = ` + activity + `.id AND marked_sender.role = 'from'
        AND lower(btrim(marked_sender.address)) = ANY (ARRAY(
          SELECT lower(btrim(marked_email.email)) FROM contact marked_contact
            JOIN contact_email marked_email ON marked_email.contact_id = marked_contact.id AND marked_email.archived_at IS NULL
           WHERE marked_contact.archived_at IS NULL AND NOT (` + NotMarkedSQL("marked_contact") + `))))`
}
