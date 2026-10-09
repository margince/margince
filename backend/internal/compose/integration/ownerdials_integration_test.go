// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The owner dials on the deal and project lists, and the Owner sort on every
// owner-scoped list, against real SQL. Every list binds the dials through
// storekit.OwnershipClause.

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func dealIDsOf(t *testing.T, e *Env, ctx context.Context, in deals.ListDealsInput) []ids.UUID {
	t.Helper()
	page, _, err := e.Deals.ListDeals(ctx, in)
	if err != nil {
		t.Fatalf("listing deals: %v", err)
	}
	out := make([]ids.UUID, 0, len(page))
	for _, d := range page {
		out = append(out, ids.UUID(d.Id))
	}
	return out
}

func projectIDsOf(t *testing.T, e *Env, ctx context.Context, in projects.ListProjectsInput) []ids.UUID {
	t.Helper()
	page, _, err := e.Projects.ListProjects(ctx, in)
	if err != nil {
		t.Fatalf("listing projects: %v", err)
	}
	out := make([]ids.UUID, 0, len(page))
	for _, p := range page {
		out = append(out, ids.UUID(p.Id))
	}
	return out
}

func TestTheDealListNarrowsToOneTeamAndToTheUnownedQueue(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	// Rep1 and Rep2 share Team1; Rep3 sits in Team2.
	mine := e.SeedDeal(t, "Owned By Rep1", pipeline, open, &e.Rep1)
	teammates := e.SeedDeal(t, "Owned By Rep2", pipeline, open, &e.Rep2)
	e.SeedDeal(t, "Owned By Rep3", pipeline, open, &e.Rep3)
	unowned := e.SeedDeal(t, "Owned By Nobody", pipeline, open, &e.Rep1)
	e.WsExec(t, `UPDATE deal SET owner_id = NULL WHERE id = $1`, unowned)

	team := ids.From[ids.TeamKind](e.Team1)
	got := dealIDsOf(t, e, e.Admin(), deals.ListDealsInput{OwnerTeamID: &team})
	want := []ids.UUID{mine, teammates}
	byID := func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) }
	slices.SortFunc(got, byID)
	slices.SortFunc(want, byID)
	if !slices.Equal(got, want) {
		t.Fatalf("owner_team_id returned %v, want exactly the two deals Team1's members own (%v)", got, want)
	}

	yes := true
	if queue := dealIDsOf(t, e, e.Admin(), deals.ListDealsInput{Unassigned: &yes}); !slices.Equal(queue, []ids.UUID{unowned}) {
		t.Fatalf("unassigned=true returned %v, want only the unowned deal %v", queue, unowned)
	}

	owner := ids.From[ids.UserKind](e.Rep1)
	for _, in := range []deals.ListDealsInput{
		{OwnerID: &owner, Unassigned: &yes},
		{OwnerID: &owner, OwnerTeamID: &team},
		{OwnerTeamID: &team, Unassigned: &yes},
	} {
		if _, _, err := e.Deals.ListDeals(e.Admin(), in); err == nil {
			t.Fatal("two owner dials at once were accepted on the deal list; want a validation refusal")
		}
	}
}

// Every seat reads every project. So the bound a dial must stay inside is the
// rest of the where chain: the rows this caller's unfiltered list shows. The
// archived project is the row a dial that replaced the chain would return.
func TestTheProjectOwnerDialsOnlyEverSubtractFromTheCallersList(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Owner Dial Co", nil)
	mine := seedProject(e.Admin(), t, e, "Owned By Rep1", company, &e.Rep1)
	theirs := seedProject(e.Admin(), t, e, "Owned By Rep3", company, &e.Rep3)
	shelved := seedProject(e.Admin(), t, e, "Archived, Owned By Rep2", company, &e.Rep2)
	e.WsExec(t, `UPDATE project SET archived_at = now() WHERE id = $1`, shelved.ID.UUID)
	unowned := seedProject(e.Admin(), t, e, "Owned By Nobody", company, &e.Rep1)
	e.WsExec(t, `UPDATE project SET owner_id = NULL WHERE id = $1`, unowned.ID.UUID)

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})
	visible := projectIDsOf(t, e, rep, projects.ListProjectsInput{})
	own := ids.From[ids.TeamKind](e.Team1)
	other := ids.From[ids.TeamKind](e.Team2)
	yes := true

	for _, tc := range []struct {
		name string
		in   projects.ListProjectsInput
		want ids.UUID
	}{
		{"owner_team_id=<the caller's team>", projects.ListProjectsInput{OwnerTeamID: &own}, mine.ID.UUID},
		{"owner_team_id=<another team>", projects.ListProjectsInput{OwnerTeamID: &other}, theirs.ID.UUID},
		{"unassigned=true", projects.ListProjectsInput{Unassigned: &yes}, unowned.ID.UUID},
	} {
		got := projectIDsOf(t, e, rep, tc.in)
		if !slices.Equal(got, []ids.UUID{tc.want}) {
			t.Errorf("%s returned %v, want only %v", tc.name, got, tc.want)
		}
		for _, id := range got {
			if !slices.Contains(visible, id) {
				t.Errorf("%s returned %v, which this caller's unfiltered list does not show", tc.name, id)
			}
		}
	}

	if _, _, err := e.Projects.ListProjects(rep, projects.ListProjectsInput{OwnerTeamID: &own, Unassigned: &yes}); err == nil {
		t.Fatal("owner_team_id AND unassigned were accepted on the project list; want a validation refusal")
	}
}

// The Owner column prints a name, so sorting by it orders by that name. The
// seats are named against their id order, so an id sort comes out reversed.
// The deal list's own case is in dealsortvocabulary_integration_test.go.
func TestSortingByOwnerOrdersByTheOwnersName(t *testing.T) {
	e := Setup(t)
	owners := []ids.UUID{e.Rep1, e.Rep2, e.Rep3}
	slices.SortFunc(owners, func(a, b ids.UUID) int { return bytes.Compare(a[:], b[:]) })
	seats := []struct {
		owner ids.UUID
		name  string
	}{{owners[0], "Zed"}, {owners[1], "Mia"}, {owners[2], "Anna"}}
	for _, seat := range seats {
		e.WsExec(t, `UPDATE app_user SET display_name = $2 WHERE id = $1`, seat.owner, seat.name)
	}
	byOwner := "owner_id"
	want := []string{"Anna", "Mia", "Zed"}

	var company ids.UUID
	for _, seat := range seats {
		company = e.SeedCompany(t, seat.name, &seat.owner)
	}
	companies, _, err := e.Contacts.ListCompanies(e.Admin(), contacts.ListCompaniesInput{Sort: &byOwner})
	if err != nil {
		t.Fatalf("sorting companies by owner: %v", err)
	}
	got := make([]string, 0, len(companies))
	for _, c := range companies {
		got = append(got, c.DisplayName)
	}
	if !slices.Equal(got, want) {
		t.Errorf("companies sorted by owner came out %v, want %v", got, want)
	}

	for _, seat := range seats {
		seedProject(e.Admin(), t, e, seat.name, company, &seat.owner)
	}
	projectPage, _, err := e.Projects.ListProjects(e.Admin(), projects.ListProjectsInput{Sort: &byOwner})
	if err != nil {
		t.Fatalf("sorting projects by owner: %v", err)
	}
	got = got[:0]
	for _, p := range projectPage {
		got = append(got, p.Name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("projects sorted by owner came out %v, want %v", got, want)
	}
}

// An archived team keeps its memberships but stops resolving scope, so the
// team filter must name nobody through it either. Archived through the real
// team writer.
func TestTheTeamFilterNamesNobodyThroughAnArchivedTeam(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	e.SeedDeal(t, "Owned By Rep1", pipeline, open, &e.Rep1)
	team := ids.From[ids.TeamKind](e.Team1)
	if got := dealIDsOf(t, e, e.Admin(), deals.ListDealsInput{OwnerTeamID: &team}); len(got) != 1 {
		t.Fatalf("before the archive, owner_team_id returned %d deals, want Rep1's one", len(got))
	}

	archived := true
	admin := identity.Identity{
		UserID:      ids.From[ids.UserKind](e.AdminUser),
		WorkspaceID: ids.From[ids.WorkspaceKind](e.WS),
		SeatType:    "full",
		Roles:       []string{"admin"},
		Permissions: AdminPerms,
	}
	if _, err := identity.NewServiceFor(e.DB()).UpdateTeam(e.Admin(), admin, e.Team1,
		identity.UpdateTeamInput{Archived: &archived}); err != nil {
		t.Fatalf("archiving Team1: %v", err)
	}
	if got := dealIDsOf(t, e, e.Admin(), deals.ListDealsInput{OwnerTeamID: &team}); len(got) != 0 {
		t.Fatalf("owner_team_id=<an archived team> returned %v, want nothing", got)
	}
}

// SeatNames withholds an archived seat's name, and the sort key travels in
// the page cursor. So an archived owner sorts as no name at all: last.
// No product path archives a seat, so the row is stamped directly.
func TestAnArchivedOwnerSortsLastWithNoName(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Aaron' WHERE id = $1`, e.Rep1)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Zoe' WHERE id = $1`, e.Rep2)
	archived := e.SeedCompany(t, "Owned by Aaron", &e.Rep1)
	live := e.SeedCompany(t, "Owned by Zoe", &e.Rep2)
	e.WsExec(t, `UPDATE app_user SET archived_at = now() WHERE id = $1`, e.Rep1)

	byOwner := "owner_id"
	page, _, err := e.Contacts.ListCompanies(e.Admin(), contacts.ListCompaniesInput{Sort: &byOwner})
	if err != nil {
		t.Fatalf("sorting companies by owner: %v", err)
	}
	got := make([]ids.UUID, 0, len(page))
	for _, c := range page {
		got = append(got, ids.UUID(c.Id))
	}
	if !slices.Equal(got, []ids.UUID{live, archived}) {
		t.Fatalf("sorted by owner: %v, want the live owner's company first and the archived owner's last", got)
	}
}
