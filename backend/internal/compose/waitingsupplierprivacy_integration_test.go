// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWaitingSupplierEnrichmentWithholdsUnreadableRelationships(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Example supplier", &e.Rep1)
	roles := []string{"supplier"}
	if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{RelationshipTypes: &roles}); err != nil {
		t.Fatal(err)
	}
	contact := employedAt(t, e, company, "Example sender")
	rows := []activities.WaitingReply{{ContactID: contact}, {CompanyID: company}}
	for _, tc := range []struct {
		name                     string
		company, edge            bool
		scope                    principal.RowScope
		wantCompany, wantContact bool
	}{
		{"visible", true, true, principal.RowScopeAll, true, true},
		{"company withheld", false, true, principal.RowScopeAll, false, false},
		{"employment withheld", true, false, principal.RowScopeAll, true, false},
		{"outside row scope", true, true, principal.RowScopeOwn, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.scope == principal.RowScopeOwn {
				e.MakeCapturePrivate(t, "company", company, e.Rep1)
				e.MakeCapturePrivate(t, "contact", contact, e.Rep1)
			}
			ctx := e.As(e.Rep2, nil, principal.Permissions{RowScope: tc.scope, Objects: map[string]principal.ObjectGrant{
				"contact": {Read: true}, "company": {Read: tc.company}, "relationship": {Read: tc.edge},
			}})
			got, err := (attentionWaiting{contacts: e.Contacts}).waitingSuppliers(ctx, rows)
			if err != nil {
				t.Fatal(err)
			}
			if got[company] != tc.wantCompany || got[contact] != tc.wantContact {
				t.Fatalf("company=%t contact=%t, want %t %t", got[company], got[contact], tc.wantCompany, tc.wantContact)
			}
		})
	}
}
