// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Two statements about one address, however it is spelled, are refused at
// every door that takes them, so no reader of the list can pick a different one.
func TestEveryDoorRefusesTwoCountriesForOneRecipient(t *testing.T) {
	de, at := "DE", "at"
	twice := &[]crmcontracts.RecipientCountry{
		{Address: "Anna@Example.test", Country: &de},
		{Address: " anna@example.test", Country: &at},
	}
	to := []openapi_types.Email{"anna@example.test"}
	doors := map[string]func() error{
		"reply": func() error {
			_, err := replySendInput(crmcontracts.SendEmailRequest{To: to, Subject: "S", Body: "B", RecipientCountries: twice})
			return err
		},
		"new conversation": func() error {
			_, err := DecodeCompanyEmailRequest(crmcontracts.SendCompanyEmailRequest{To: to, Subject: "S", Body: "B", RecipientCountries: twice})
			return err
		},
		"preview": func() error {
			_, err := previewInputFrom(to, nil, nil, nil, nil, twice)
			return err
		},
		"draft": func() error {
			_, err := mailDraftContentFrom(crmcontracts.MailDraftInput{RecipientCountries: twice})
			return err
		},
	}
	for name, door := range doors {
		var refused *CommunicationContextError
		if err := door(); !errors.As(err, &refused) || refused.field != recipientCountriesField {
			t.Errorf("%s: two countries for one recipient → %v, want the field refused", name, err)
		}
	}
	once, err := countryOverridesFrom(&[]crmcontracts.RecipientCountry{{Address: " Anna@Example.test", Country: &de}})
	if err != nil || len(once) != 1 || once[0] != (commsauthz.CountryOverride{Address: "anna@example.test", Country: "de"}) {
		t.Errorf("one statement decoded as %+v (%v), want it canonical", once, err)
	}
}
