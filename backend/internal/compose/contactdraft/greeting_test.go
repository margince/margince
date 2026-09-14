// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft_test

// Which name a familiar greeting uses.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func viewOf(contact crmcontracts.Contact) crmcontracts.Contact360 {
	return crmcontracts.Contact360{Contact: contact}
}

func contactNamed(full string, first, last *string) crmcontracts.Contact {
	return crmcontracts.Contact{Id: openapi_types.UUID(ids.NewV7()), FullName: full, FirstName: first, LastName: last}
}

func ptr(s string) *string { return &s }

// With no stored first name, the display name is still split.
//
// Most of what capture writes carries a full name and nothing else, so the
// fallback is the common path rather than a corner.
// The stored first name is what a familiar greeting uses.
//
// Not a regression test — recipientOf already preferred the stored name before
// this file existed, by overwriting the split further down. What is pinned here
// is that the preference survives the two being merged into one function, so a
// later reader cannot delete the "redundant" half and reintroduce the split.
func TestTheGreetingUsesTheStoredFirstName(t *testing.T) {
	in := contactdraft.FromView(
		viewOf(contactNamed("Marcus Greven", nil, nil)),
		contactdraft.Request{},
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
	in := contactdraft.FromView(
		viewOf(contactNamed("Marcus Greven", ptr("   "), nil)),
		contactdraft.Request{},
	)
	if in.Recipient.FirstName != "Marcus" {
		t.Errorf("greeting name = %q, want the fallback: a blank column is not a name", in.Recipient.FirstName)
	}
}
