// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package persondraft_test

// Which name a familiar greeting uses.

import (
	"testing"

	uuid "github.com/google/uuid"
	"github.com/margince/margince/backend/internal/compose/persondraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func viewOf(person crmcontracts.Person) crmcontracts.Person360 {
	return crmcontracts.Person360{Person: person}
}

func personNamed(full string, first, last *string) crmcontracts.Person {
	return crmcontracts.Person{Id: uuid.New(), FullName: full, FirstName: first, LastName: last}
}

func ptr(s string) *string { return &s }

// The STORED first name wins over splitting the display name.
//
// A display name is whatever a mail header carried, and capture writes plenty
// that are not "Given Family": "Pg Philipp" is one real record, and splitting
// it greets a person called Philipp as "Pg". The record already knew better —
// the first name sat in its own column, which this greeting did not read.
func TestTheGreetingPrefersTheStoredFirstName(t *testing.T) {
	in := persondraft.FromView(
		viewOf(personNamed("Pg Philipp", ptr("Philipp"), ptr("Pg"))),
		persondraft.Request{},
	)
	if in.Recipient.FirstName != "Philipp" {
		t.Errorf("greeting name = %q, want the stored first name: the split reads a display "+
			"name as if it were always Given Family", in.Recipient.FirstName)
	}
}

// With no stored first name, the display name is still split.
//
// Most of what capture writes carries a full name and nothing else, so the
// fallback is the common path rather than a corner.
func TestTheGreetingFallsBackToSplittingTheDisplayName(t *testing.T) {
	in := persondraft.FromView(
		viewOf(personNamed("Marcus Greven", nil, nil)),
		persondraft.Request{},
	)
	if in.Recipient.FirstName != "Marcus" {
		t.Errorf("greeting name = %q, want the first word of the display name", in.Recipient.FirstName)
	}
}

// A blank stored first name is not a name.
//
// An empty string in the column is what a partial import leaves behind, and
// greeting somebody "Hi ," is worse than greeting them by a split display name.
func TestABlankStoredFirstNameFallsBack(t *testing.T) {
	in := persondraft.FromView(
		viewOf(personNamed("Marcus Greven", ptr("   "), nil)),
		persondraft.Request{},
	)
	if in.Recipient.FirstName != "Marcus" {
		t.Errorf("greeting name = %q, want the fallback: a blank column is not a name", in.Recipient.FirstName)
	}
}
