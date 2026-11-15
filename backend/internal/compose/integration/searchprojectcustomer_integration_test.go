// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Search names a project's company the way the project's own read does. It is
// the first live customer edge, named only to a caller who may read that edge.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// projectCustomerFixture holds two projects whose stored company_id is the
// same company. Only one of them still has it as a customer.
type projectCustomerFixture struct {
	alpha, beta             ids.UUID
	stillAlpha, movedToBeta ids.UUID
}

func (q *queryEnv) seedProjectCustomers(t *testing.T) projectCustomerFixture {
	t.Helper()
	var f projectCustomerFixture
	f.alpha = q.SeedID(t, `INSERT INTO company (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Alpha Kunde', 'manual', 'human:x')`, q.Rep1)
	f.beta = q.SeedID(t, `INSERT INTO company (id, owner_id, display_name, source, captured_by)
		VALUES ($1, $2, 'Beta Kunde', 'manual', 'human:x')`, q.Rep1)
	project := func(name, key string) ids.UUID {
		return q.SeedID(t, `INSERT INTO project (id, owner_id, name, key, company_id, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, 'manual', 'human:x')`, q.Rep1, name, key, f.alpha)
	}
	edge := func(project, company ids.UUID, archived string) {
		q.SeedID(t, `INSERT INTO relationship (id, kind, project_id, company_id, role, source, captured_by, archived_at)
			VALUES ($1, 'project_company', $2, $3, 'customer', 'manual', 'human:x', `+archived+`)`, project, company)
	}
	f.stillAlpha = project("Kesselwartung", "KESSEL")
	edge(f.stillAlpha, f.alpha, "NULL")
	// Alpha was taken off this project and Beta became its customer. The
	// stored company_id still names Alpha.
	f.movedToBeta = project("Turbinenwartung", "TURBINE")
	edge(f.movedToBeta, f.alpha, "now()")
	edge(f.movedToBeta, f.beta, "NULL")
	return f
}

// withoutEdges reads every record type, but holds no relationship.read.
func (q *queryEnv) withoutEdges() context.Context {
	grants := queryGrants()
	delete(grants, "relationship")
	ctx := principal.WithWorkspaceID(context.Background(), q.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

func TestSearchFiltersAProjectByItsReadableFirstCustomer(t *testing.T) {
	q := setupQuery(t)
	f := q.seedProjectCustomers(t)
	byCompany := func(ctx context.Context, company ids.UUID) map[ids.UUID]bool {
		return idSet(q.run(ctx, t, `{"version": "v1", "target": "project",
			"where": [{"field": "company_id", "op": "eq", "value": "`+company.String()+`"}]}`))
	}
	if got := byCompany(q.admin(), f.alpha); !got[f.stillAlpha] || got[f.movedToBeta] || len(got) != 1 {
		t.Errorf("projects of Alpha = %v, want only the one Alpha is still customer of", got)
	}
	if got := byCompany(q.admin(), f.beta); !got[f.movedToBeta] || len(got) != 1 {
		t.Errorf("projects of Beta = %v, want the project Beta became customer of", got)
	}
	for _, company := range []ids.UUID{f.alpha, f.beta} {
		if got := byCompany(q.withoutEdges(), company); len(got) != 0 {
			t.Errorf("a caller without relationship.read learned %v by filtering on company %s", got, company)
		}
	}
}

func TestSearchHopsBetweenAProjectAndItsReadableFirstCustomer(t *testing.T) {
	q := setupQuery(t)
	f := q.seedProjectCustomers(t)
	toCompany := `{"version": "v1", "target": "project",
		"traverse": {"relation": "company", "where": [{"field": "display_name", "op": "eq", "value": "Alpha Kunde"}]}}`
	toProjects := `{"version": "v1", "target": "company",
		"traverse": {"relation": "projects", "where": [{"field": "name", "op": "eq", "value": "Turbinenwartung"}]}}`

	if got := idSet(q.run(q.admin(), t, toCompany)); !got[f.stillAlpha] || len(got) != 1 {
		t.Errorf("projects reaching Alpha = %v, want only the one Alpha is still customer of", got)
	}
	if got := idSet(q.run(q.admin(), t, toProjects)); !got[f.beta] || len(got) != 1 {
		t.Errorf("customers of Turbinenwartung = %v, want Beta alone", got)
	}
	for _, doc := range []string{toCompany, toProjects} {
		if got := idSet(q.run(q.withoutEdges(), t, doc)); len(got) != 0 {
			t.Errorf("a caller without relationship.read learned %v through the hop %s", got, doc)
		}
	}
}

func TestASearchHitNamesOnlyTheReadableFirstCustomer(t *testing.T) {
	q := setupQuery(t)
	q.seedProjectCustomers(t)
	snippet := func(ctx context.Context) string {
		t.Helper()
		page, err := q.Store.Search(ctx, search.Input{Query: "Turbinenwartung", Types: []string{"project"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Hits) != 1 {
			t.Fatalf("hits = %+v, want the one project", page.Hits)
		}
		return page.Hits[0].Snippet
	}
	if got := snippet(q.admin()); got != "TURBINE · Beta Kunde" {
		t.Errorf("snippet = %q, want the key and the current customer", got)
	}
	if got := snippet(q.withoutEdges()); got != "TURBINE" {
		t.Errorf("snippet for a caller without relationship.read = %q, want the key alone", got)
	}
}

// The query_workspace tool runs the same plan executor, so its filter answers
// the same project for the same caller.
func TestTheQueryToolFiltersAProjectByItsReadableFirstCustomer(t *testing.T) {
	q := setupQuery(t)
	f := q.seedProjectCustomers(t)
	registry := compose.NewRegistry(q.Pool, compose.SendPath{})
	plan := `{"plan":{"version": "v1", "target": "project",
		"where": [{"field": "company_id", "op": "eq", "value": "` + f.beta.String() + `"}]}}`

	if got := rowIDs(queryPayload(t, invokeQuery(q.admin(), t, registry, plan).Data)); !got[f.movedToBeta] || len(got) != 1 {
		t.Errorf("query_workspace projects of Beta = %v, want the project Beta became customer of", got)
	}
	if got := rowIDs(queryPayload(t, invokeQuery(q.withoutEdges(), t, registry, plan).Data)); len(got) != 0 {
		t.Errorf("query_workspace told a caller without relationship.read about %v", got)
	}
}
