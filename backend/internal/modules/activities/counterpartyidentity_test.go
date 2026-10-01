// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which contact the counterparty phrase names.

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func party(name string, contact *ids.UUID, address string) crmcontracts.EmailParty {
	p := crmcontracts.EmailParty{Address: address}
	if name != "" {
		p.DisplayName = &name
	}
	if contact != nil {
		id := openapi_types.UUID(*contact)
		p.ContactId = &id
	}
	return p
}

// THE ID BELONGS TO THE PARTY THE NAME CAME FROM.
//
// The phrase names one far side and counts the rest, so the id has to be that
// same party's — not the row's, and not the first party's when an earlier one
// had no name to give.
func TestTheCounterpartyIdNamesThePartyTheNameCameFrom(t *testing.T) {
	t.Parallel()

	first, second := ids.NewV7(), ids.NewV7()
	// The earlier party has an address and no name, so the phrase takes the
	// NAMED one — and the id must follow the phrase rather than the order.
	phrase, id := counterpartyOf([]crmcontracts.EmailParty{
		party("", &first, "quiet@partner.test"),
		party("Ana Sommer", &second, "ana@partner.test"),
	})
	if phrase == nil || *phrase != "Ana Sommer +1" {
		t.Fatalf("phrase %v, want the named party and a count of the rest", phrase)
	}
	if id == nil || ids.UUID(*id) != second {
		t.Errorf("the id names %v, want the contact the phrase was taken from", id)
	}
}

// A far side that resolved to no contact carries no id, rather than borrowing
// one: the phrase is then all a reader has, and a client keying a face on it
// is keying on words because there is nothing better.
func TestAnUnresolvedCounterpartyCarriesNoContact(t *testing.T) {
	t.Parallel()

	phrase, id := counterpartyOf([]crmcontracts.EmailParty{
		party("Stranger", nil, "nobody@elsewhere.test"),
	})
	if phrase == nil || *phrase != "Stranger" {
		t.Fatalf("phrase %v, want the stranger's own name", phrase)
	}
	if id != nil {
		t.Errorf("an unresolved party carries contact %v", id)
	}
}

// No participants at all is no counterparty and no id — an invented stranger
// would be worse than saying nothing.
func TestNoPartiesNamesNobody(t *testing.T) {
	t.Parallel()

	phrase, id := counterpartyOf(nil)
	if phrase != nil || id != nil {
		t.Errorf("an empty far side answered %v / %v", phrase, id)
	}
}
