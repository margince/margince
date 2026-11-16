// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Search reads a project's companies from their live edges, never from the
// stored project.company_id. A filter or hop by company matches any live edge
// the caller may read. A hit's excerpt names the first customer a project read
// names. A caller without relationship.read learns no pair at all.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// projectCompanyFixture is four projects whose stored company_id and live
// edges disagree in the ways that matter.
type projectCompanyFixture struct {
	alpha, beta, gamma, private ids.UUID
	// kessel has Alpha as its customer.
	kessel ids.UUID
	// turbine started with Alpha, which was taken off; Beta is its customer.
	turbine ids.UUID
	// pumpe has Private, then Beta, as customers. Private is capture-private
	// to Rep3, so Rep1 at own scope may not read it.
	pumpe ids.UUID
	// ventil has Gamma as customer and Alpha as partner; Gamma is archived.
	ventil ids.UUID
}

// writer seeds as Rep1, a seat the fixture created, with the admin grants.
func (q *queryEnv) writer() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), q.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + q.Rep1.String(), UserID: q.Rep1,
		Permissions: AdminPerms,
	})
}

func (q *queryEnv) seedProjectCompanies(t *testing.T) projectCompanyFixture {
	t.Helper()
	company := func(name string, owner ids.UUID) ids.UUID {
		return q.SeedID(t, `INSERT INTO company (id, owner_id, display_name, source, captured_by)
			VALUES ($1, $2, $3, 'manual', 'human:x')`, owner, name)
	}
	var f projectCompanyFixture
	f.alpha = company("Alpha Kunde", q.Rep1)
	f.beta = company("Beta Kunde", q.Rep1)
	f.gamma = company("Gamma Kunde", q.Rep1)
	f.private = company("Privat Kunde", q.Rep3)

	store := compose.ProjectsStore(q.Pool)
	edges := contacts.NewStore(q.DB())
	ctx := q.writer()
	owner := ids.From[ids.UserKind](q.Rep1)
	project := func(name, key string, customer ids.UUID) ids.UUID {
		created, err := store.CreateProject(ctx, projects.CreateProjectInput{
			Name: name, Key: &key, CompanyID: companyIDOf(customer), OwnerID: &owner, Source: "manual",
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	attach := func(project, company ids.UUID, role string) {
		if _, err := edges.SetProjectCompany(ctx, contacts.SetProjectCompanyInput{
			ProjectID: ids.From[ids.ProjectKind](project), CompanyID: companyIDOf(company), Role: role,
		}); err != nil {
			t.Fatal(err)
		}
	}
	f.kessel = project("Kesselwartung", "KESSEL", f.alpha)
	f.turbine = project("Turbinenwartung", "TURBINE", f.alpha)
	attach(f.turbine, f.beta, "customer")
	if err := edges.RemoveProjectCompany(ctx, ids.From[ids.ProjectKind](f.turbine), companyIDOf(f.alpha)); err != nil {
		t.Fatal(err)
	}
	f.pumpe = project("Pumpenwartung", "PUMPE", f.private)
	attach(f.pumpe, f.beta, "customer")
	f.ventil = project("Ventilwartung", "VENTIL", f.gamma)
	attach(f.ventil, f.alpha, "partner")
	// The archive writer refuses a company a live project names, so the row is
	// archived directly, as a company archived before its project would be.
	if _, err := q.Owner.Exec(context.Background(), `UPDATE company SET archived_at = now() WHERE id = $1`, f.gamma); err != nil {
		t.Fatal(err)
	}
	// Capture privacy, applied after the edges exist as MakeCapturePrivate does:
	// no writer creates a private company Rep1 could attach.
	if _, err := q.Owner.Exec(context.Background(), `UPDATE company SET visibility = 'owner' WHERE id = $1`, f.private); err != nil {
		t.Fatal(err)
	}
	return f
}

// reader reads every record type at the given row scope, without the named
// object grants.
func (q *queryEnv) reader(scope principal.RowScope, without ...string) context.Context {
	return q.readerAs(q.Rep1, q.Team1, scope, without...)
}

func (q *queryEnv) readerAs(user, team ids.UUID, scope principal.RowScope, without ...string) context.Context {
	grants := queryGrants()
	for _, object := range without {
		delete(grants, object)
	}
	ctx := principal.WithWorkspaceID(context.Background(), q.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		TeamIDs:     []ids.UUID{team},
		Permissions: principal.Permissions{Objects: grants, RowScope: scope},
	})
}

// projectReaders are the callers every case is asked as.
func (q *queryEnv) projectReaders() map[string]context.Context {
	return map[string]context.Context{
		"all":            q.reader(principal.RowScopeAll),
		"own":            q.reader(principal.RowScopeOwn),
		"no edges":       q.reader(principal.RowScopeAll, "relationship"),
		"no companies":   q.reader(principal.RowScopeAll, "company"),
		"no edges (own)": q.reader(principal.RowScopeOwn, "relationship"),
		"rep3":           q.readerAs(q.Rep3, q.Team2, principal.RowScopeOwn),
	}
}

func assertSameIDs(t *testing.T, what string, got map[ids.UUID]bool, want ...ids.UUID) {
	t.Helper()
	wanted := map[ids.UUID]bool{}
	for _, id := range want {
		wanted[id] = true
	}
	if len(got) != len(wanted) {
		t.Errorf("%s = %v, want %v", what, got, want)
		return
	}
	for id := range wanted {
		if !got[id] {
			t.Errorf("%s = %v, want %v", what, got, want)
			return
		}
	}
}

func TestSearchFiltersAProjectByAnyReadableLiveCompany(t *testing.T) {
	t.Parallel()
	q := setupQuery(t)
	f := q.seedProjectCompanies(t)
	readers := q.projectReaders()
	filter := func(reader, op string, company ids.UUID) map[ids.UUID]bool {
		operand := `"value": "` + company.String() + `"`
		if op == "in" {
			operand = `"values": ["` + company.String() + `"]`
		}
		return idSet(q.run(readers[reader], t, `{"version": "v1", "target": "project",
			"where": [{"field": "company_id", "op": "`+op+`", `+operand+`}]}`))
	}
	assertSameIDs(t, "projects with Alpha", filter("all", "eq", f.alpha), f.kessel, f.ventil)
	assertSameIDs(t, "projects in [Alpha]", filter("all", "in", f.alpha), f.kessel, f.ventil)
	assertSameIDs(t, "projects with Beta", filter("own", "eq", f.beta), f.turbine, f.pumpe)
	assertSameIDs(t, "projects with archived Gamma", filter("all", "eq", f.gamma))
	assertSameIDs(t, "projects with Private, as its owner", filter("rep3", "eq", f.private), f.pumpe)
	assertSameIDs(t, "projects with Private, outside capture privacy", filter("all", "eq", f.private))
	assertSameIDs(t, "projects without Beta", filter("all", "neq", f.beta), f.kessel, f.ventil)
	// Private is not readable to Rep1, so excluding it excludes nothing: the
	// answer is the one a project without Private would give.
	assertSameIDs(t, "projects without Private", filter("all", "neq", f.private),
		f.kessel, f.turbine, f.pumpe, f.ventil)
	for _, reader := range []string{"no edges", "no companies", "no edges (own)"} {
		for _, op := range []string{"eq", "neq", "in"} {
			for _, company := range []ids.UUID{f.alpha, f.beta, f.private} {
				assertSameIDs(t, reader+" "+op+" "+company.String(), filter(reader, op, company))
			}
		}
	}
}

func TestSearchHopsBetweenAProjectAndAnyReadableLiveCompany(t *testing.T) {
	t.Parallel()
	q := setupQuery(t)
	f := q.seedProjectCompanies(t)
	readers := q.projectReaders()
	toCompany := func(reader, name string) map[ids.UUID]bool {
		return idSet(q.run(readers[reader], t, `{"version": "v1", "target": "project",
			"traverse": {"relation": "company", "where": [{"field": "display_name", "op": "eq", "value": "`+name+`"}]}}`))
	}
	toProjects := func(reader, name string) map[ids.UUID]bool {
		return idSet(q.run(readers[reader], t, `{"version": "v1", "target": "company",
			"traverse": {"relation": "projects", "where": [{"field": "name", "op": "eq", "value": "`+name+`"}]}}`))
	}
	assertSameIDs(t, "projects reaching Alpha", toCompany("all", "Alpha Kunde"), f.kessel, f.ventil)
	assertSameIDs(t, "projects reaching Private", toCompany("all", "Privat Kunde"))
	assertSameIDs(t, "companies of Pumpenwartung", toProjects("all", "Pumpenwartung"), f.beta)
	assertSameIDs(t, "companies of Turbinenwartung", toProjects("all", "Turbinenwartung"), f.beta)
	assertSameIDs(t, "companies of Ventilwartung", toProjects("all", "Ventilwartung"), f.alpha)
	for _, reader := range []string{"no edges", "no edges (own)"} {
		assertSameIDs(t, reader+": projects reaching Alpha", toCompany(reader, "Alpha Kunde"))
		assertSameIDs(t, reader+": companies of Turbinenwartung", toProjects(reader, "Turbinenwartung"))
	}
	// Without company.read neither hop is in the caller's vocabulary, in
	// either direction, so the plan is refused rather than answered.
	for _, doc := range []string{
		`{"version": "v1", "target": "project", "traverse": {"relation": "company",
			"where": [{"field": "display_name", "op": "eq", "value": "Alpha Kunde"}]}}`,
		`{"version": "v1", "target": "company", "traverse": {"relation": "projects",
			"where": [{"field": "name", "op": "eq", "value": "Turbinenwartung"}]}}`,
	} {
		if result, err := q.answer(readers["no companies"], doc); err == nil {
			t.Errorf("a caller without company.read was answered %v for %s", idSet(result), doc)
		}
	}
}

func TestASearchHitNamesOnlyTheReadableFirstCustomer(t *testing.T) {
	t.Parallel()
	q := setupQuery(t)
	q.seedProjectCompanies(t)
	readers := q.projectReaders()
	snippet := func(reader, name string) string {
		t.Helper()
		page, err := q.Store.Search(readers[reader], search.Input{Query: name, Types: []string{"project"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Hits) != 1 {
			t.Fatalf("%s hits for %s = %+v, want the one project", reader, name, page.Hits)
		}
		return page.Hits[0].Snippet
	}
	for _, tc := range []struct{ reader, project, want string }{
		{"all", "Turbinenwartung", "TURBINE · Beta Kunde"},
		// The first customer is capture-private; Beta must not stand in.
		{"all", "Pumpenwartung", "PUMPE"},
		// The only customer is archived and Alpha is a partner.
		{"all", "Ventilwartung", "VENTIL"},
		{"no edges", "Turbinenwartung", "TURBINE"},
		{"no companies", "Turbinenwartung", "TURBINE"},
	} {
		if got := snippet(tc.reader, tc.project); got != tc.want {
			t.Errorf("%s snippet of %s = %q, want %q", tc.reader, tc.project, got, tc.want)
		}
	}
}

// The query_workspace tool runs the same plan executor.
func TestTheQueryToolFiltersAProjectByAnyReadableLiveCompany(t *testing.T) {
	t.Parallel()
	q := setupQuery(t)
	f := q.seedProjectCompanies(t)
	readers := q.projectReaders()
	registry := compose.NewRegistry(q.Pool, compose.SendPath{})
	plan := func(op string) string {
		return `{"plan":{"version": "v1", "target": "project",
			"where": [{"field": "company_id", "op": "` + op + `", "value": "` + f.alpha.String() + `"}]}}`
	}
	answer := func(reader, op string) map[ids.UUID]bool {
		return rowIDs(queryPayload(t, invokeQuery(readers[reader], t, registry, plan(op)).Data))
	}
	assertSameIDs(t, "query_workspace projects with Alpha", answer("all", "eq"), f.kessel, f.ventil)
	assertSameIDs(t, "query_workspace projects without Alpha", answer("all", "neq"), f.turbine, f.pumpe)
	assertSameIDs(t, "query_workspace projects with Alpha, no edges", answer("no edges", "eq"))
	assertSameIDs(t, "query_workspace projects without Alpha, no edges", answer("no edges", "neq"))
}
