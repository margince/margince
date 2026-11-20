// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft_test

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// promptTap records what the model is asked and answers nothing usable, which is
// enough: the test reads the prompt, not the draft.
type promptTap struct{ asked []string }

func (p *promptTap) Complete(_ context.Context, req model.Request) (model.Response, error) {
	for _, message := range req.Messages {
		p.asked = append(p.asked, message.Content)
	}
	return model.Response{Text: `{"subject":"Hello","body":"Hello,\n\nA note.\n\nBest"}`}, nil
}

func humanCaller() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:u-1", UserID: ids.NewV7(),
	})
}

func withRegister(contact crmcontracts.Contact, register crmcontracts.ContactGreetingRegister) crmcontracts.Contact {
	contact.GreetingRegister = &register
	last := "Lovelace"
	contact.LastName = &last
	return contact
}

func TestAStoredRegisterOnTheContactReachesTheContactDraft(t *testing.T) {
	pathID, path := contactAt("ada@customer.test")
	path = withRegister(path, crmcontracts.ContactGreetingRegisterFormal)
	svc := contactdraft.NewService(pathContact{path}, nil)

	draft, _, err := svc.Draft(humanCaller(), pathID, contactdraft.Request{})
	if err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if !strings.HasPrefix(draft.Body, "Dear Ada Lovelace,") {
		t.Errorf("the no-model draft opens %q, want the formal opening", draft.Body)
	}
}

func TestAFamiliarPrimaryWithAFormalCoRecipientDraftsFormally(t *testing.T) {
	pathID, path := contactAt("ada@customer.test")
	path = withRegister(path, crmcontracts.ContactGreetingRegisterFamiliar)
	otherID, other := contactAt("grace@customer.test")
	other = withRegister(other, crmcontracts.ContactGreetingRegisterFormal)
	tap := &promptTap{}
	svc := contactdraft.NewService(pathContact{path}, tap).
		WithCoRecipients(readableContacts{otherID: other})

	if _, _, err := svc.Draft(humanCaller(), pathID, contactdraft.Request{AlsoTo: []ids.ContactID{otherID}}); err != nil {
		t.Logf("the stubbed model answer was not accepted: %v", err)
	}
	prompt := strings.Join(tap.asked, "\n")
	if !strings.Contains(prompt, `"greeting_register":"formal"`) || strings.Contains(prompt, `"greeting_register":"familiar"`) {
		t.Errorf("a group whose registers differ was not drafted formally: %s", prompt)
	}
}
