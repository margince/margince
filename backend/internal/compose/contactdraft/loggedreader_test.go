// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
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

// The draft asks for the contact's newest logged touch in the project it is
// narrowed to, and a failed read stops it.
func TestAContactDraftReadsTheNewestLoggedTouchOfItsContact(t *testing.T) {
	contactID, contact := contactAt("ada@customer.test")
	project := ids.From[ids.ProjectKind](ids.NewV7())
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:u-1"})
	reader := &askedLogged{}
	svc := contactdraft.NewService(pathContact{contact}, nil).WithLoggedTouches(reader)

	if _, _, err := svc.Draft(human, contactID, contactdraft.Request{}); err != nil {
		t.Fatalf("drafting: %v", err)
	}
	if reader.on.RecordType != "contact" || reader.on.ID != contactID.UUID || reader.on.ProjectID != nil {
		t.Errorf("the draft asked about %+v, want the contact", reader.on)
	}
	// This contact is on no project, so the draft refuses later; the read is narrowed first.
	if _, _, err := svc.Draft(human, contactID, contactdraft.Request{ProjectID: &project}); err == nil {
		t.Fatal("a project the contact is not on was drafted for")
	}
	if reader.on.ProjectID == nil || *reader.on.ProjectID != project {
		t.Errorf("the project draft asked about %+v, want the read narrowed to the project", reader.on)
	}
	reader.err = errors.New("down")
	if _, _, err := svc.Draft(human, contactID, contactdraft.Request{}); err == nil {
		t.Error("a failed logged-touch read still produced a draft")
	}
}
