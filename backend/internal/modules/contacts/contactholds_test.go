// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The contact a lead was worked from holds keys that are the lead's by design,
// so a lead write and an import's test run refuse only over another contact.
func TestALeadsWorkedFromContactIsNoRival(t *testing.T) {
	workedFrom, other := ids.New[ids.ContactKind](), ids.New[ids.ContactKind]()
	for _, tc := range []struct {
		name       string
		holders    []ids.ContactID
		workedFrom *ids.ContactID
		want       bool
	}{
		{"only the worked-from contact", []ids.ContactID{workedFrom}, &workedFrom, false},
		{"another contact beside it", []ids.ContactID{workedFrom, other}, &workedFrom, true},
		{"a lead worked from no contact", []ids.ContactID{workedFrom}, nil, true},
		{"no contact holds the key", nil, &workedFrom, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rival, found := leadRivalContact(storekit.IdentityHolders{Contacts: tc.holders}, tc.workedFrom)
			if found != tc.want {
				t.Fatalf("rival found = %v, want %v", found, tc.want)
			}
			if found && tc.workedFrom != nil && rival == *tc.workedFrom {
				t.Errorf("the rival is the contact the lead was worked from")
			}
		})
	}
}
