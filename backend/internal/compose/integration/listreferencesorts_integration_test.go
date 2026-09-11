// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The columns the contacts, leads and projects lists draw that are not columns
// of their own row.
//
// Each is asserted the way that matters for a DERIVED sort: the order the list
// gives agrees with the values it prints. That is what makes the expression in
// the ORDER BY the same derivation as the one on screen rather than a second
// one that will drift — and every one of these reads the very expression the
// row is rendered from.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A project's phase orders by how LIVE the work is, not by the word.
//
// Alphabetically the four phases run closed, delivering, initiative, pursuing —
// which is the arrangement shuffled, and the case turns on the two disagreeing.
func TestTheProjectsListSortsPhasesByHowLiveTheWorkIs(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	org := e.SeedOrg(t, "Acme Systems", &e.Rep1)

	// Seeded in the arrangement's own order, so a pass cannot come from
	// insertion order either: the assertion below is the reverse of it.
	closed := seedProjectInPhase(t, e, org, "Finished work", "closed")
	initiative := seedProjectInPhase(t, e, org, "An idea", projectPhaseInitiative)
	pursuing := seedProjectInPhase(t, e, org, "Chasing it", "pursuing")
	delivering := seedProjectInPhase(t, e, org, "Under way", "delivering")

	assertIDOrder(t, projectIDsIn(ctx, t, e, "phase"),
		[]ids.UUID{delivering, pursuing, initiative, closed},
		"phase ascending — the work in motion first")
}

// A project's company orders by the customer's name.
func TestTheProjectsListSortsByTheCompanyItDraws(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	// Project names and company names deliberately disagree, so the answer
	// cannot come from the sort this list already had.
	zeta := seedProjectInPhase(t, e, e.SeedOrg(t, "Alma Werke", &e.Rep1), "Zeta rollout", "delivering")
	alma := seedProjectInPhase(t, e, e.SeedOrg(t, "Zeta Holding", &e.Rep1), "Alma rollout", "delivering")

	assertIDOrder(t, projectIDsIn(ctx, t, e, "organization_id"), []ids.UUID{zeta, alma},
		"company ascending — Alma Werke before Zeta Holding")
}

// A lead's source orders by the words the column prints, not by the key stored
// behind them.
func TestTheLeadsListSortsBySourceLabelAndNotItsKey(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	// Keys and labels deliberately sort the other way round: read the key and
	// the page comes back reversed.
	seedLeadSource(t, e, "aaa_key", "Zeta referral")
	seedLeadSource(t, e, "zzz_key", "Alma campaign")
	fromZeta := seedLeadFromSource(t, e, "First lead", "aaa_key")
	fromAlma := seedLeadFromSource(t, e, "Second lead", "zzz_key")

	assertIDOrder(t, leadIDsIn(ctx, t, e, "source"), []ids.UUID{fromAlma, fromZeta},
		"source ascending — Alma campaign before Zeta referral")
}

// A contact's company orders by the employer the row prints, walking the same
// edge the row walked.
func TestTheContactsListSortsByTheEmployerItDraws(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	zeta := e.SeedPerson(t, "Zeta Person", &e.Rep1)
	alma := e.SeedPerson(t, "Alma Person", &e.Rep1)
	employPerson(t, e, zeta, e.SeedOrg(t, "Alma Werke", &e.Rep1), nil)
	employPerson(t, e, alma, e.SeedOrg(t, "Zeta Holding", &e.Rep1), nil)
	// A person with no employer at all shows nothing and orders by nothing.
	unattached := e.SeedPerson(t, "Nobody's Person", &e.Rep1)

	rows, _, err := e.People.ListPeople(ctx, people.ListPeopleInput{Sort: strPtr("employer")})
	if err != nil {
		t.Fatalf("listing contacts by employer: %v", err)
	}
	got := make([]ids.UUID, len(rows))
	for i, p := range rows {
		got[i] = ids.UUID(p.Id)
	}
	assertIDOrder(t, got, []ids.UUID{zeta, alma, unattached},
		"employer ascending — Alma Werke, Zeta Holding, then nobody")

	// The order agrees with what the rows print, which is the claim that makes
	// the expression the same derivation rather than a second one.
	if rows[0].Employer == nil || rows[0].Employer.OrganizationName != "Alma Werke" {
		t.Errorf("the first row prints %v, and the page was ordered by the employer it comes from",
			rows[0].Employer)
	}
}

// A company this reader may not open orders the contacts list by NOTHING.
//
// Ordering by a value is reading it: a page ordered by an employer the row
// beside it leaves blank would disclose the name through the order — read it
// ascending and descending and the hidden company sits at whichever end.
//
// TWO guards hold this and either alone is enough — the employment edge's own
// read scope and the organization's row scope, both of which the sort inherits
// from the read it shares its statement with. So neutralising one changes
// nothing and only removing both moves the row, which is what this case was
// checked against. Worth saying, because a reader who deleted one of them and
// saw this test stay green would conclude it holds nothing.
func TestAnUnreadableEmployerOrdersTheContactsListByNothing(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	// "Alma" sorts first of the two by name. Hidden from this reader, it must
	// sort last instead.
	hidden := e.SeedOrg(t, "Alma Werke", &e.Rep3)
	secret := e.SeedPerson(t, "Zeta Person", &e.Rep1)
	visible := e.SeedPerson(t, "Alma Person", &e.Rep1)
	employPerson(t, e, secret, hidden, nil)
	employPerson(t, e, visible, e.SeedOrg(t, "Mercator", &e.Rep1), nil)
	// Made private AFTER the edge is filed, which is the real sequence.
	e.MakeCapturePrivate(t, "organization", hidden, e.Rep3)

	listed := func(spec string) []ids.UUID {
		t.Helper()
		rows, _, err := e.People.ListPeople(ctx, people.ListPeopleInput{Sort: &spec})
		if err != nil {
			t.Fatalf("listing contacts by %s: %v", spec, err)
		}
		out := make([]ids.UUID, len(rows))
		for i, p := range rows {
			out[i] = ids.UUID(p.Id)
		}
		return out
	}

	// Admitted first: the reader must SEE both people, so what the sort does
	// below is the company's visibility and not the person's.
	if got := listed("full_name"); len(got) != 2 {
		t.Fatalf("the reader sees %d contacts, want 2 — this case would prove nothing", len(got))
	}

	assertIDOrder(t, listed("employer"), []ids.UUID{visible, secret},
		"employer ascending — the unreadable one in the tail, not first")
	// And the same position the other way round. A name that really was
	// ordering the page would move to the other end.
	assertIDOrder(t, listed("-employer"), []ids.UUID{visible, secret},
		"employer descending — still the tail, because there is nothing to order by")
}

// A reader who may see NO employer at all is ordered by none.
//
// The employer is bounded three ways and a caller can lose it entirely — no
// relationship grant, or no organization grant. The column is then absent on
// every row, so the sort must be too: a page arranged by a company nobody on
// screen names would order the list by something the reader cannot check.
func TestAReaderWhoSeesNoEmployerIsOrderedByNone(t *testing.T) {
	e := Setup(t)

	zeta := e.SeedPerson(t, "Zeta Person", &e.Rep1)
	alma := e.SeedPerson(t, "Alma Person", &e.Rep1)
	employPerson(t, e, zeta, e.SeedOrg(t, "Alma Werke", &e.Rep1), nil)
	employPerson(t, e, alma, e.SeedOrg(t, "Zeta Holding", &e.Rep1), nil)

	// No `relationship` grant: the edge is what an employer is read through.
	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"person":       {Read: true},
			"organization": {Read: true},
		},
		RowScope: principal.RowScopeTeam,
	})
	rows, _, err := e.People.ListPeople(blind, people.ListPeopleInput{Sort: strPtr("employer")})
	if err != nil {
		t.Fatalf("listing contacts by employer without the edge grant: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("the reader sees %d contacts, want 2 — they may read people", len(rows))
	}
	for _, p := range rows {
		if p.Employer != nil {
			t.Fatalf("a reader without relationship.read was shown employer %v", p.Employer)
		}
	}
	// Both rows in the tail, so the page falls back to its tie-breaker. Newest
	// first is what that gives, and Alma Person was seeded last.
	assertIDOrder(t, []ids.UUID{ids.UUID(rows[0].Id), ids.UUID(rows[1].Id)},
		[]ids.UUID{alma, zeta}, "employer ascending, for a reader shown none")
}

// A lead's Next task header orders by the DEADLINE, and its Last activity by
// the last-touch clock — both derived, both read from the expression the row is
// printed from.
func TestTheLeadsListSortsByTheDerivedColumnsItDraws(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	seedLeadSource(t, e, "manual_key", "Manual")

	soon := seedLeadFromSource(t, e, "Due soon", "manual_key")
	later := seedLeadFromSource(t, e, "Due later", "manual_key")
	quiet := seedLeadFromSource(t, e, "Nothing owed", "manual_key")

	// Task deadlines in the reverse of the leads' creation order.
	taskOn(t, e, later, "Call them", time.Now().Add(72*time.Hour))
	taskOn(t, e, soon, "Send the quote", time.Now().Add(2*time.Hour))

	got := leadIDsIn(ctx, t, e, "next_task_due_at")
	assertIDOrder(t, got, []ids.UUID{soon, later, quiet},
		"next task ascending — the nearest deadline first, no task last")

	// The last-touch clock. The two tasks above are activity too, filed just
	// now, so the reply goes on the third lead AN HOUR AGO and the assertion
	// runs ascending: oldest touch first, which only the reply's instant can
	// put there.
	replyTo(t, e, quiet, time.Now().Add(-time.Hour))
	byTouch := leadIDsIn(ctx, t, e, "last_activity_at")
	if len(byTouch) != 3 || byTouch[0] != quiet {
		t.Errorf("last activity ascending put %v first, want the lead touched an hour ago", byTouch)
	}
}

// taskOn files an open task against a lead, through the real writer.
func taskOn(t *testing.T, e *Env, lead ids.UUID, subject string, due time.Time) {
	t.Helper()
	leadID := lead
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: leadID}},
	})
	if err != nil {
		t.Fatalf("filing %q: %v", subject, err)
	}
	_ = logged
}

// replyTo files an inbound email against a lead at one instant — the last-touch
// clock counts engagement, not the product's own filing.
func replyTo(t *testing.T, e *Env, lead ids.UUID, at time.Time) {
	t.Helper()
	subject, body := "Re: your note", "Sounds good."
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body,
		Direction: strPtr("inbound"), OccurredAt: &at,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead}},
	}); err != nil {
		t.Fatalf("filing the reply: %v", err)
	}
}

// A caller who may not read organizations at all is ordered by no company.
//
// The row scope answers WHICH organizations are visible and never whether this
// caller may read organizations in the first place. A seat holding project.read
// and no organization.read would otherwise have its page arranged by company
// names it is refused on every other surface — and read it both ways and the
// alphabet is recoverable from the order alone.
func TestAReaderWithoutTheCompanyGrantIsOrderedByNoCompany(t *testing.T) {
	e := Setup(t)

	// Named so the company order and the project order disagree: if the sort
	// still reached display_name the page would come back the other way round.
	zeta := seedProjectInPhase(t, e, e.SeedOrg(t, "Alma Werke", &e.Rep1), "Zeta rollout", "delivering")
	alma := seedProjectInPhase(t, e, e.SeedOrg(t, "Zeta Holding", &e.Rep1), "Alma rollout", "delivering")

	// Admitted first, as a reader who DOES hold the grant: the silence below is
	// the grant's doing and not a sort that never worked.
	assertIDOrder(t, projectIDsIn(e.Admin(), t, e, "organization_id"), []ids.UUID{zeta, alma},
		"company ascending, for a reader who may read companies")

	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})
	// Both rows in the tail under BOTH directions, so the page falls back to
	// its tie-breaker and the order carries nothing about the companies. Newest
	// first is what that gives, and the Alma rollout was seeded last.
	assertIDOrder(t, projectIDsIn(blind, t, e, "organization_id"), []ids.UUID{alma, zeta},
		"company ascending, for a reader who may not read companies")
	assertIDOrder(t, projectIDsIn(blind, t, e, "-organization_id"), []ids.UUID{alma, zeta},
		"company descending — the same order, because there is nothing to order by")
}

// projectPhaseInitiative is where a project starts, so it is the one phase the
// ladder below never advances to.
const projectPhaseInitiative = "initiative"

// seedProjectInPhase creates one project and moves it to a phase through the
// real writer.
func seedProjectInPhase(t *testing.T, e *Env, org ids.UUID, name, phase string) ids.UUID {
	t.Helper()
	orgID := ids.From[ids.OrganizationKind](org)
	p, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: name, OrganizationID: orgID, OwnerID: userIDPtr(&e.Rep1), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	// A project is BORN in initiative, so asking for that phase is asking for
	// no advance at all. Without this the loop below runs off the end of its
	// rungs and closes the project instead — which is how the first version of
	// this fixture made two closed projects and no initiative one, and the
	// assertion passed anyway.
	if phase == projectPhaseInitiative {
		return ids.UUID(p.Id)
	}
	// Advanced along the real ladder, one rung at a time: a phase written by
	// hand would order correctly over a column no writer ever puts that value
	// in.
	for _, rung := range []string{"pursuing", "delivering", "closed"} {
		if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), ids.From[ids.ProjectKind](ids.UUID(p.Id)),
			projects.AdvanceProjectPhaseInput{ToPhase: rung, Reason: strPtr("the fixture is done with it")}); err != nil {
			t.Fatalf("advancing %q to %s: %v", name, rung, err)
		}
		if rung == phase {
			break
		}
	}
	return ids.UUID(p.Id)
}

func projectIDsIn(ctx context.Context, t *testing.T, e *Env, spec string) []ids.UUID {
	t.Helper()
	rows, _, err := e.Projects.ListProjects(ctx, projects.ListProjectsInput{Sort: &spec})
	if err != nil {
		t.Fatalf("ListProjects(sort=%s): %v", spec, err)
	}
	out := make([]ids.UUID, len(rows))
	for i, p := range rows {
		out[i] = ids.UUID(p.Id)
	}
	return out
}

func leadIDsIn(ctx context.Context, t *testing.T, e *Env, spec string) []ids.UUID {
	t.Helper()
	rows, _, err := e.People.ListLeads(ctx, people.ListLeadsInput{Sort: &spec})
	if err != nil {
		t.Fatalf("ListLeads(sort=%s): %v", spec, err)
	}
	out := make([]ids.UUID, len(rows))
	for i, l := range rows {
		out[i] = ids.UUID(l.Id)
	}
	return out
}

func seedLeadSource(t *testing.T, e *Env, key, label string) {
	t.Helper()
	e.WsExec(t, `INSERT INTO lead_source (key, label) VALUES ($1, $2)`, key, label)
}

// seedLeadFromSource creates a lead filed under one catalog source.
func seedLeadFromSource(t *testing.T, e *Env, name, source string) ids.UUID {
	t.Helper()
	l, _, err := e.People.CreateLead(e.Admin(), people.CreateLeadInput{
		FullName: &name, Source: source, OwnerID: userIDPtr(&e.Rep1),
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	return ids.UUID(l.Id)
}
