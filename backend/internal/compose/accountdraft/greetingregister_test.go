// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"context"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAStoredRegisterOnTheRecipientReachesTheAccountDraft(t *testing.T) {
	for register, want := range map[crmcontracts.Company360ContactGreetingRegister]string{
		crmcontracts.Company360ContactGreetingRegisterFormal:   "Guten Tag Priya Raman,",
		crmcontracts.Company360ContactGreetingRegisterFamiliar: "Hallo Priya,",
	} {
		t.Run(string(register), func(t *testing.T) {
			company := ids.From[ids.CompanyKind](ids.NewV7())
			human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:u-1"})
			contact := ids.NewV7()
			view := viewWithActivities(t, activityWith(t, "Angebot", "Wir melden uns wegen der offenen Punkte."))
			view.Contacts = &struct {
				Data []crmcontracts.Company360Contact `json:"data"`
				Page crmcontracts.PageInfo            `json:"page"`
			}{Data: []crmcontracts.Company360Contact{{
				ContactId: openapi_types.UUID(contact), FullName: "Priya Raman", GreetingRegister: &register,
			}}}
			draft, _, err := NewService(fixedAccount{view: view}, nil).Draft(human, company, Request{ContactID: contact.String()})
			if err != nil {
				t.Fatalf("drafting: %v", err)
			}
			if !strings.HasPrefix(draft.Body, want) {
				t.Errorf("the no-model draft opens %q, want %q", draft.Body, want)
			}
		})
	}
}
