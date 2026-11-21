// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The linked passes select only the types legacyAuditKinds lists, so every
// listed type must build a subject query and no other type may.
func TestTheLinkedPassSelectsExactlyTheTypesItCanCorrect(t *testing.T) {
	buyerSeat := "buyer:" + ids.NewV7().String()
	for _, kind := range legacyAuditKinds() {
		entry := legacyAuditEntry{kind: kind, actorType: "buyer", actorID: buyerSeat}
		if query, _ := legacyAuditSubjectQuery(entry); query == "" {
			t.Errorf("the linked pass selects %s audit rows but builds no subject query for them", kind)
		}
	}
	for _, kind := range []string{"contact", "company", "deal", "lead", "audit_log"} {
		if slices.Contains(legacyAuditKinds(), kind) {
			t.Errorf("the linked pass selects %s audit rows, which it has no subject query for", kind)
		}
	}
	if len(legacyAuditKinds()) != 6 {
		t.Errorf("the linked pass selects %d types, want the 6 it has subject queries for: %v", len(legacyAuditKinds()), legacyAuditKinds())
	}
}
