// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCommitmentReferencesKeepSameNamedPeopleDistinct(t *testing.T) {
	seat, contact := openapi_types.UUID(ids.NewV7()), openapi_types.UUID(ids.NewV7())
	name := "Alex"
	parties := verifiedCommitmentParties(activities.MessageParties{
		From: []crmcontracts.EmailParty{{UserId: &seat, DisplayName: &name}},
		To:   []crmcontracts.EmailParty{{ContactId: &contact, DisplayName: &name}},
	})
	for _, ref := range []string{"seat:" + seat.String(), "contact:" + contact.String()} {
		got, ok := referencedPromiser(ref, parties)
		if !ok || got.Ref != ref {
			t.Fatalf("reference %q resolved to %+v, %v", ref, got, ok)
		}
	}
	for _, ref := range []string{"", "Alex", "seat:" + ids.NewV7().String()} {
		if _, ok := referencedPromiser(ref, parties); ok {
			t.Fatalf("unverified reference %q resolved", ref)
		}
	}
}

func TestCounterpartyIgnoresCopiesAndFoldsAddressAliases(t *testing.T) {
	customer, copied := ids.NewV7(), ids.NewV7()
	parties := []commitmentParty{
		{Role: "to", Contact: &customer, Address: "first@example.test"},
		{Role: "to", Contact: &customer, Address: "alias@example.test"},
		{Role: "cc", Contact: &copied},
	}
	got, ok := soleCommitmentCounterparty(parties)
	if !ok || got != customer {
		t.Fatalf("counterparty = %v, %v", got, ok)
	}
	parties = append(parties, commitmentParty{Role: "to", Contact: &copied})
	if _, ok := soleCommitmentCounterparty(parties); ok {
		t.Fatal("multiple direct contacts resolved to one")
	}
}

func TestParticipantPromptDoesNotAddUnquotedAddresses(t *testing.T) {
	parties := []commitmentParty{
		{Ref: "seat:one", Role: "attendee", Name: "Mira Chen", Address: "mira@example.test"},
		{Ref: "contact:two", Role: "attendee", Name: "Mira Chen", Address: "buyer@example.test"},
	}
	prompt := commitmentPartyPrompt(parties, "Mira Chen: I will send pricing.")
	if strings.Contains(prompt, "@") || !strings.Contains(prompt, "Mira Chen") {
		t.Fatalf("unquoted address exported or name omitted: %s", prompt)
	}
	prompt = commitmentPartyPrompt(parties, "MIRA@example.test: I will send pricing.")
	if !strings.Contains(prompt, "mira@example.test") || strings.Contains(prompt, "buyer@example.test") {
		t.Fatalf("quoted identity was not minimized: %s", prompt)
	}
}

func TestAQuotedAddressCannotBePartOfAnotherAddress(t *testing.T) {
	for _, source := range []string{"amira@example.test", "mira@example.testing", "mira@example.test.evil", "mira@example.test@other.test"} {
		if quotedCommitmentAddress(source, "mira@example.test") {
			t.Errorf("another address was treated as the participant: %s", source)
		}
	}
	if !quotedCommitmentAddress("<MIRA@example.test>: send pricing", "mira@example.test") {
		t.Fatal("the exact quoted address was lost")
	}
}
