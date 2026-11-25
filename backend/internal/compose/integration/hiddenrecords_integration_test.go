// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A company page section that row scope shortens says how many entries it
// withheld and which colleagues own them, and nothing a withheld entry holds.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// hiddenReaderPerms is a team-scoped rep, so the scope clause is rendered: an
// unbounded caller skips it and would pass against a read with no clause.
var hiddenReaderPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":      {Read: true},
		"company":      {Read: true},
		"project":      {Read: true},
		"relationship": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

// seedDepartedSeat is a colleague who left: their records still exist and
// still count as withheld, and nobody is told to ask them.
func seedDepartedSeat(t *testing.T) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO app_user (id, email, display_name, email_domain_folded, status)
		VALUES ($1, 'departed@authz.test', 'Departed Colleague', 'authz.test', 'deactivated')`, id); err != nil {
		t.Fatalf("seeding a departed seat: %v", err)
	}
	return id
}

func seedOwnedProject(t *testing.T, e *Env, company ids.UUID, name string, owner ids.UUID) {
	t.Helper()
	if _, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: name, CompanyID: companyIDOf(company), OwnerID: userIDPtr(&owner), Source: "manual",
	}); err != nil {
		t.Fatalf("creating project %q: %v", name, err)
	}
}

func TestCompanyProjectsStateHowManyAreHiddenAndWhoHoldsThem(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedOwnedProject(t, e, company, "Visible rollout", e.Rep1)
	seedOwnedProject(t, e, company, "Hidden audit", e.Rep3)
	seedOwnedProject(t, e, company, "Hidden migration", e.Rep3)
	seedOwnedProject(t, e, company, "Orphaned pilot", seedDepartedSeat(t))

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, hiddenReaderPerms)
	page, err := companySurfaceService(e).Assemble(rep, companyIDOf(company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.Projects == nil || len(*page.Projects) != 1 || (*page.Projects)[0].Name != "Visible rollout" {
		t.Fatalf("projects = %+v, want only the rep's own", page.Projects)
	}
	if page.ProjectsHidden == nil {
		t.Fatal("projects_hidden is absent beside a listed projects section")
	}
	if page.ProjectsHidden.Count != 3 {
		t.Errorf("projects_hidden.count = %d, want 3: two of Rep3's and the departed seat's", page.ProjectsHidden.Count)
	}
	holders := page.ProjectsHidden.Holders
	if len(holders) != 1 || ids.UUID(holders[0].UserId) != e.Rep3 {
		t.Errorf("holders = %+v, want Rep3 once and never the departed seat", holders)
	}
	wire, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, withheld := range []string{"Hidden audit", "Hidden migration", "Orphaned pilot"} {
		if strings.Contains(string(wire), withheld) {
			t.Errorf("the page carries the withheld project name %q", withheld)
		}
	}
}

func TestAnUnboundedReaderIsToldNothingIsHidden(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	seedOwnedProject(t, e, company, "Rep3's work", e.Rep3)

	page, err := companySurfaceService(e).Assemble(e.Admin(), companyIDOf(company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.ProjectsHidden == nil || page.ProjectsHidden.Count != 0 || len(page.ProjectsHidden.Holders) != 0 {
		t.Errorf("projects_hidden = %+v, want count 0 and no holders for an unbounded reader", page.ProjectsHidden)
	}
}

func TestHiddenBillingContactsNameTheirOwnerNotThemselves(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Shared Account", &e.Rep1)
	mine := e.SeedContact(t, "Visible Payer", &e.Rep1)
	theirs := e.SeedContact(t, "Hidden Payer", &e.Rep3)
	seedBillingEdge(t, mine, company, "recipient")
	seedBillingEdge(t, theirs, company, "approver")

	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, billingReaderPerms)
	var hidden storekit.HiddenRows
	err := database.WithWorkspaceTx(rep, e.Pool, func(tx pgx.Tx) error {
		var err error
		hidden, err = e.Contacts.HiddenBillingContactsFor(rep, tx, ids.From[ids.CompanyKind](company))
		return err
	})
	if err != nil {
		t.Fatalf("reading hidden billing contacts: %v", err)
	}
	if hidden.Count != 1 || len(hidden.Holders) != 1 || hidden.Holders[0].UserID != e.Rep3 {
		t.Errorf("hidden = %+v, want one entry held by Rep3", hidden)
	}
}
