// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"log/slog"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedProduct writes one catalog product and answers its id.
func seedProduct(t *testing.T, e *integration.Env, name string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO product (id, name, unit_price_minor, currency, source, captured_by, component_kind)
		VALUES ($1, $2, 100000, 'EUR', 'manual', 'human:x', 'software')`, id, name)
	return id
}

func productOf(lead crmcontracts.Lead) *ids.UUID {
	if lead.ProductId == nil {
		return nil
	}
	id := ids.UUID(*lead.ProductId)
	return &id
}

// A lead names the product it is interested in when it is written. It may
// change or clear it, and cannot name a product the catalog retired.
func TestALeadNamesTheProductItIsInterestedIn(t *testing.T) {
	e := integration.Setup(t)
	suite := seedProduct(t, e, "Suite")
	onboarding := seedProduct(t, e, "Onboarding")
	retired := seedProduct(t, e, "Retired plan")
	e.WsExec(t, `UPDATE product SET archived_at = now() WHERE id = $1`, retired)
	name := "Asking about the suite"

	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: &name, Status: "new", Source: "manual", ProductID: &suite,
	})
	if err != nil {
		t.Fatalf("creating a lead with a product: %v", err)
	}
	if got := productOf(lead); got == nil || *got != suite {
		t.Fatalf("the new lead names product %v, want %v", got, suite)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))

	changed, err := e.Contacts.UpdateLead(e.Admin(), id, contacts.UpdateLeadInput{ProductID: &onboarding})
	if err != nil {
		t.Fatalf("changing the lead's product: %v", err)
	}
	if got := productOf(changed); got == nil || *got != onboarding {
		t.Errorf("the lead names product %v after the change, want %v", got, onboarding)
	}

	cleared, err := e.Contacts.UpdateLead(e.Admin(), id, contacts.UpdateLeadInput{Clear: []string{"product_id"}})
	if err != nil {
		t.Fatalf("clearing the lead's product: %v", err)
	}
	if got := productOf(cleared); got != nil {
		t.Errorf("the lead still names product %v after it was cleared", *got)
	}

	_, err = e.Contacts.UpdateLead(e.Admin(), id, contacts.UpdateLeadInput{ProductID: &retired})
	if refusal, ok := errors.AsType[*httperr.DetailedError](err); !ok || len(refusal.Fields) != 1 || refusal.Fields[0].Field != "product_id" {
		t.Errorf("naming a retired product answered %v, want a refusal naming product_id", err)
	}
	stored, err := e.Contacts.GetLead(e.Admin(), id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got := productOf(stored); got != nil {
		t.Errorf("a refused product was stored: %v", *got)
	}
}

// A web form names the product its leads ask about, as it names their owner.
// It refuses a product the catalog retired when it is saved, not on a post.
func TestAWebFormNamesTheProductItsLeadsAskAbout(t *testing.T) {
	e := integration.Setup(t)
	suite := seedProduct(t, e, "Suite")
	retired := seedProduct(t, e, "Retired plan")
	e.WsExec(t, `UPDATE product SET archived_at = now() WHERE id = $1`, retired)
	perms := integration.AdminWithSignals
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["integrations"] = principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	ctx := e.As(e.AdminUser, nil, perms)
	forms := webforms.NewStore(e.DB(), slog.Default())

	form, _, err := forms.CreateForm(ctx, webforms.CreateInput{
		Name: "Suite enquiries", Sender: webforms.SenderBrowser,
		AllowedOrigins: []string{"https://visitor.example"}, ProductID: &suite,
	})
	if err != nil {
		t.Fatalf("creating a form with a product: %v", err)
	}
	if form.ProductID == nil || *form.ProductID != suite {
		t.Fatalf("the form names product %v, want %v", form.ProductID, suite)
	}
	if got := webFormLeadInput(webforms.Landing{ProductID: form.ProductID}).ProductID; got == nil || *got != suite {
		t.Errorf("a lead the form lands names product %v, want %v", got, suite)
	}

	_, err = forms.UpdateForm(ctx, form.ID, webforms.UpdateInput{ProductID: &retired})
	if refusal, ok := errors.AsType[*webforms.BadInputError](err); !ok || refusal.Field != "product_id" {
		t.Errorf("naming a retired product on the form answered %v, want a refusal naming product_id", err)
	}
	cleared, err := forms.UpdateForm(ctx, form.ID, webforms.UpdateInput{Clear: []string{"product_id"}})
	if err != nil {
		t.Fatalf("clearing the form's product: %v", err)
	}
	if cleared.ProductID != nil {
		t.Errorf("the form still names product %v after it was cleared", *cleared.ProductID)
	}
}
