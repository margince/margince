// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A project link inherited from a deal must not outlive the deal it came
// with. It must not outrank a project a member named, nor hand the project's
// owner a say over the note.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Declining the deal takes the project it brought along, and keeps a project
// the member named themselves.
func TestDecliningTheDealTakesOnlyTheProjectItBrought(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	inherited := logDealNote(e.Admin(), t, e, onDeal(f.erpDeal))
	named := logDealNote(e.Admin(), t, e, onDeal(f.erpDeal), activities.ActivityLinkInput{EntityType: "project", EntityID: f.erp})

	for _, note := range []ids.UUID{inherited, named} {
		if _, err := e.Activities.DeclineDeal(e.Admin(), ids.From[ids.ActivityKind](note), &f.erpDeal); err != nil {
			t.Fatalf("declining the deal: %v", err)
		}
	}
	if projectLinks(t, e, inherited, f.erp) != 0 {
		t.Error("the project that came with the declined deal stayed")
	}
	if projectLinks(t, e, named, f.erp) != 1 {
		t.Error("the project the member named went with the declined deal")
	}
}

// Moving a note from one deal to another moves the project it inherited.
func TestReplacingTheDealMovesTheInheritedProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	note := logDealNote(e.Admin(), t, e, onDeal(f.erpDeal))

	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](note), activities.RelinkActivityInput{
		EntityType: "deal", EntityID: f.migrationDeal, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("moving the note to the other deal: %v", err)
	}
	if projectLinks(t, e, note, f.erp) != 0 || projectLinks(t, e, note, f.migration) != 1 {
		t.Errorf("after the move the note is on erp=%d migration=%d, want only the new deal's project",
			projectLinks(t, e, note, f.erp), projectLinks(t, e, note, f.migration))
	}
}

// The owner of a project a note inherited cannot change a note another rep
// wrote on their own deal.
func TestAnInheritedProjectGivesItsOwnerNoSayOverTheNote(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Acme", nil)
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "ERP rollout", CompanyID: companyIDOf(company), OwnerID: userIDPtr(&e.Rep3), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	pipeline, open, _ := DealFixture(t, e)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "ERP licences", PipelineID: pipeline, StageID: open, CompanyID: new(companyIDOf(company)),
		ProjectID: new(projectIDOf(ids.UUID(project.Id))), OwnerID: userIDPtr(&e.Rep1),
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	member := dealNotePerms(principal.ObjectGrant{Create: true, Read: true, Update: true})
	note := logDealNote(e.As(e.Rep1, []ids.UUID{e.Team1}, member), t, e, onDeal(ids.UUID(deal.Id)))
	if projectLinks(t, e, note, ids.UUID(project.Id)) != 1 {
		t.Fatal("the note did not inherit the project, so nothing here is tested")
	}

	_, err = e.Activities.UpdateActivity(e.As(e.Rep3, []ids.UUID{e.Team2}, member),
		ids.From[ids.ActivityKind](note), activities.UpdateActivityInput{Body: new("rewritten")})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the project owner's edit → %v, want a permission refusal", err)
	}
}

// DealsProject names no project to a role without project read.
func TestTheDealsProjectNeedsProjectRead(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	resolve := func(perms principal.Permissions) ids.UUID {
		ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
		var got ids.UUID
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			got, err = storekit.DealsProject(ctx, tx, []ids.UUID{f.erpDeal})
			return err
		}); err != nil {
			t.Fatalf("resolving the deal's project: %v", err)
		}
		return got
	}
	full := dealNotePerms(principal.ObjectGrant{Read: true})
	if got := resolve(full); got != f.erp {
		t.Fatalf("a reader of projects is told %v, want the deal's project", got)
	}
	delete(full.Objects, "project")
	if got := resolve(full); !got.IsZero() {
		t.Errorf("a role without project read is told %v, want none", got)
	}
}

// A member's note on the deal counts as the project's filed work and moves
// its clock.
func TestAMembersDealNoteCountsForTheProject(t *testing.T) {
	e := Setup(t)
	f := seedDealNoteFixture(t, e)
	when := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	logProjectNote(e.Admin(), t, e, when, onDeal(f.erpDeal))

	if got := projectClock(e.Admin(), t, e, projectIDOf(f.erp)); got == nil || !got.Equal(when) {
		t.Errorf("the project's last_activity_at = %v, want %v", got, when)
	}
	page, err := project360Service(e, clocktest.Now(t)).Assemble(e.Admin(), projectIDOf(f.erp))
	if err != nil {
		t.Fatalf("assembling the project page: %v", err)
	}
	if page.Coverage == nil || page.Coverage.Attributed != 1 || page.Coverage.UnattributedNearby != 0 {
		t.Errorf("coverage = %+v, want the note attributed", page.Coverage)
	}
}
