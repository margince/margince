// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const legalFormExport = `First Name,Last Name,URL,Email Address,Company,Position,Connected On
Ada,Other,https://www.linkedin.com/in/adaother,,Acme GmbH,CTO,15 Mar 2024
Bea,Same,https://www.linkedin.com/in/beasame,,acme inc,CFO,15 Mar 2024
`

// A connection lands at an account only when the legal forms do not name two
// entities. "Acme GmbH" is not placed at "Acme Inc", while "acme inc" is.
func TestALinkedInEmployerWithAnotherLegalFormIsNotPlaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	company, err := e.store.CreateCompany(ctx, CreateCompanyInput{DisplayName: "Acme Inc", Source: "manual"})
	if err != nil {
		t.Fatalf("creating the company: %v", err)
	}
	if _, err := e.store.ImportLinkedInConnections(ctx, strings.NewReader(legalFormExport)); err != nil {
		t.Fatalf("importing: %v", err)
	}
	if _, err := e.store.MatchLinkedInConnections(ctx, e.rep); err != nil {
		t.Fatalf("matching: %v", err)
	}
	placed := func(name string) *ids.UUID {
		var at *ids.UUID
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT matched_company_id FROM linkedin_connection WHERE full_name = $1`, name).Scan(&at)
		}); err != nil {
			t.Fatalf("reading %s's placement: %v", name, err)
		}
		return at
	}
	if at := placed("Ada Other"); at != nil {
		t.Errorf("a connection at Acme GmbH was placed at %v, the Acme Inc account", *at)
	}
	if at := placed("Bea Same"); at == nil || *at != ids.UUID(company.Id) {
		t.Errorf("a connection at acme inc was placed at %v, want the Acme Inc account", at)
	}
}
