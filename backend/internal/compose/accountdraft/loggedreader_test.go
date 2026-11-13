// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"context"
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type askedLogged struct {
	on  draftcore.LoggedOn
	err error
}

func (a *askedLogged) NewestLoggedTouch(_ context.Context, on draftcore.LoggedOn, _ time.Time) ([]crmcontracts.Activity, error) {
	a.on = on
	return nil, a.err
}

// The account draft asks for the company's newest logged touch, and a failed
// read stops it.
func TestAnAccountDraftReadsTheNewestLoggedTouchOfItsCompany(t *testing.T) {
	company := ids.From[ids.CompanyKind](ids.NewV7())
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:u-1"})
	contact := ids.NewV7()
	view := viewWithActivities(t, activityWith(t, "Angebot", "Wir melden uns wegen der offenen Punkte."))
	view.Contacts = &struct {
		Data []crmcontracts.Company360Contact `json:"data"`
		Page crmcontracts.PageInfo            `json:"page"`
	}{Data: []crmcontracts.Company360Contact{{ContactId: openapi_types.UUID(contact), FullName: "Priya Raman"}}}
	reader := &askedLogged{}
	svc := NewService(fixedAccount{view: view}, nil).WithLoggedTouches(reader)

	if _, _, err := svc.Draft(human, company, Request{ContactID: contact.String()}); err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if reader.on.RecordType != "company" || reader.on.ID != company.UUID {
		t.Errorf("the draft asked about %+v, want the company", reader.on)
	}
	reader.err = errors.New("down")
	if _, _, err := svc.Draft(human, company, Request{ContactID: contact.String()}); err == nil {
		t.Error("a failed logged-touch read still produced a draft")
	}
}
