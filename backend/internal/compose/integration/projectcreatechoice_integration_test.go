// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What the New project form may decide at create: the key, the starting phase
// and the deal the project comes from. Each runs through the store compose builds.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func createWithKey(ctx context.Context, e *Env, company ids.UUID, name string, key *string) (string, error) {
	created, err := e.Projects.CreateProject(ctx, projects.CreateProjectInput{
		Name: name, CompanyID: companyIDOf(company), Source: "manual", Key: key,
	})
	if err != nil {
		return "", err
	}
	return *created.Key, nil
}

func keyRefusal(t *testing.T, err error) string {
	t.Helper()
	var refused *projects.ProjectKeyRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want a key refusal", err)
	}
	return refused.Code
}

func TestAChosenKeyIsStoredAndFilesMail(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Nordwind AG", nil)
	chosen := " nord "
	key, err := createWithKey(e.Admin(), e, company, "Nordwind ERP rollout", &chosen)
	if err != nil {
		t.Fatal(err)
	}
	if key != "NORD" {
		t.Fatalf("stored key = %q, want the chosen key trimmed and upper-cased", key)
	}
	var matched ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		matched, err = e.Projects.MatchProjectKey(e.Admin(), tx, []string{"nord"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if matched == ids.Nil {
		t.Fatal("a subject naming [NORD] matched no project")
	}
}

func TestAChosenKeyThatWouldMisfileMailIsRefused(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Nordwind AG", nil)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Sofia Meier' WHERE id = $1`, e.Rep2)
	taken := "NWD"
	if _, err := createWithKey(e.Admin(), e, company, "First", &taken); err != nil {
		t.Fatal(err)
	}
	for chosen, want := range map[string]string{
		"sofia":  "key_is_colleague_name",
		"Meier":  "key_is_colleague_name",
		"STATUS": "key_is_common_word",
		"fwd":    "key_is_common_word",
		"nwd":    "key_taken",
	} {
		_, err := createWithKey(e.Admin(), e, company, "Second", &chosen)
		if got := keyRefusal(t, err); got != want {
			t.Errorf("key %q refused as %q, want %q", chosen, got, want)
		}
	}
	bad := "1AB"
	var shape *projects.ProjectKeyShapeError
	if _, err := createWithKey(e.Admin(), e, company, "Third", &bad); !errors.As(err, &shape) {
		t.Errorf("a digit-led key answered %v, want the shape refusal", err)
	}
	// The mint holds itself to the same words: a project named after a
	// colleague is numbered instead of carrying the bare name.
	if key, err := createWithKey(e.Admin(), e, company, "Sofia", nil); err != nil || key != "SOFIA-1" {
		t.Errorf("a project named Sofia minted %q (%v), want SOFIA-1", key, err)
	}
}

func TestTheKeyPreviewIsTheKeyTheCreateMints(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Contoso", nil)
	for range 2 {
		preview, err := e.Projects.PreviewProjectKey(e.Admin(), "Warehouse rollout")
		if err != nil {
			t.Fatal(err)
		}
		minted, err := createWithKey(e.Admin(), e, company, "Warehouse rollout", nil)
		if err != nil {
			t.Fatal(err)
		}
		if preview != minted {
			t.Fatalf("preview %q, minted %q; the form showed a key the create did not give", preview, minted)
		}
	}
}

func TestAProjectStartsInTheChosenPhaseButNeverClosed(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Initech", nil)
	created, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Pursued", CompanyID: companyIDOf(company), Source: "manual", Phase: projects.PhasePursuing,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Phase == nil || string(*created.Phase) != projects.PhasePursuing {
		t.Fatalf("phase = %v, want pursuing", created.Phase)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM project_phase_history
		WHERE project_id = $1 AND from_phase IS NULL AND to_phase = 'pursuing'`, created.Id); n != 1 {
		t.Errorf("birth history rows in pursuing = %d, want 1", n)
	}
	_, err = e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Closed at birth", CompanyID: companyIDOf(company), Source: "manual", Phase: projects.PhaseClosed,
	})
	if _, refused := errors.AsType[*projects.ProjectStartPhaseError](err); !refused {
		t.Fatalf("a closed start answered %v, want the start-phase refusal", err)
	}
}

// projectCreatorPerms is a rep who may create projects and write the deals of
// their own team, and no others.
var projectCreatorPerms = principal.Permissions{
	Objects: map[string]principal.ObjectGrant{
		"project":       {Create: true, Read: true, Update: true},
		objDeal:         {Read: true, Update: true},
		objCompany:      {Read: true},
		objPipeline:     {Read: true},
		objRelationship: {Create: true, Read: true, Update: true},
	},
	RowScope: principal.RowScopeTeam,
}

func TestAProjectCreatedFromADealLinksItUnderTheDealsWriteCheck(t *testing.T) {
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	company := e.SeedCompany(t, "Globex", nil)
	other := e.SeedCompany(t, "Umbrella", nil)
	dealOn := func(name string, companyID ids.UUID, owner ids.UUID) ids.DealID {
		t.Helper()
		c := companyIDOf(companyID)
		d, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open, CompanyID: &c, OwnerID: userIDPtr(&owner),
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.DealKind](ids.UUID(d.Id))
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, projectCreatorPerms)
	create := func(name string, deal ids.DealID, companyID ids.UUID) (ids.UUID, error) {
		created, err := e.Projects.CreateProject(rep, projects.CreateProjectInput{
			Name: name, CompanyID: companyIDOf(companyID), Source: "manual", DealID: &deal,
		})
		return ids.UUID(created.Id), err
	}
	linkedTo := func(deal ids.DealID) string {
		return e.WsScalar(t, `SELECT coalesce(project_id::text, '') FROM deal WHERE id = $1`, deal)
	}

	teammates := dealOn("Teammate's deal", company, e.Rep2)
	project, err := create("Globex rollout", teammates, company)
	if err != nil {
		t.Fatalf("linking a deal the rep may write: %v", err)
	}
	if linkedTo(teammates) != project.String() {
		t.Errorf("deal project_id = %q, want the new project %s", linkedTo(teammates), project)
	}

	strangers := dealOn("Other team's deal", company, e.Rep3)
	if _, err := create("Globex refresh", strangers, company); err == nil {
		t.Error("a deal outside the rep's write scope was linked")
	}
	if linkedTo(strangers) != "" || e.WsCount(t, `SELECT count(*) FROM project WHERE name = 'Globex refresh'`) != 0 {
		t.Error("a refused link left the project or the pointer behind")
	}

	elsewhere := dealOn("Umbrella deal", other, e.Rep2)
	var mismatch *projects.DealCompanyMismatchError
	if _, err := create("Wrong company", elsewhere, company); !errors.As(err, &mismatch) {
		t.Errorf("a deal on another company answered %v, want the company mismatch", err)
	}

	// A create never moves a deal off the project it already names.
	var notLinkable *projects.DealNotLinkableError
	if _, err := create("Second project", teammates, company); !errors.As(err, &notLinkable) || notLinkable.Code != "deal_has_project" {
		t.Errorf("relinking a deal answered %v, want deal_has_project", err)
	}
	if linkedTo(teammates) != project.String() {
		t.Error("a refused create moved the deal off its project")
	}
	closed := dealOn("Won deal", company, e.Rep2)
	reason := "verbal"
	if _, err := e.Deals.AdvanceDeal(e.Admin(), closed, deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: &reason}); err != nil {
		t.Fatal(err)
	}
	if _, err := create("After the win", closed, company); !errors.As(err, &notLinkable) || notLinkable.Code != "deal_not_open" {
		t.Errorf("a won deal answered %v, want deal_not_open", err)
	}
}

// The approval preview runs the create's own deal check. It never offers a
// create that would take a deal from its project.
func TestTheApprovalPreviewRefusesTheDealsTheCreateRefuses(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Globex", nil)
	deal := ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Globex deal", company))
	creator, ok := principal.Actor(e.Admin())
	if !ok {
		t.Fatal("the admin context carries no principal")
	}
	preview := func() error {
		_, err := e.Projects.PreviewProjectCreationForActor(e.Admin(), projects.CreateProjectInput{
			Name: "Globex rollout", CompanyID: companyIDOf(company), Source: "manual", DealID: &deal,
		}, creator)
		return err
	}
	if err := preview(); err != nil {
		t.Fatalf("an open, unlinked deal was refused in preview: %v", err)
	}
	if _, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "First rollout", CompanyID: companyIDOf(company), Source: "manual", DealID: &deal,
	}); err != nil {
		t.Fatal(err)
	}
	var notLinkable *projects.DealNotLinkableError
	if err := preview(); !errors.As(err, &notLinkable) || notLinkable.Code != "deal_has_project" {
		t.Errorf("preview over a linked deal answered %v, want deal_has_project", err)
	}
}
