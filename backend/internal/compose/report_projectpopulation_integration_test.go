// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// projects-by-phase, project-commitments and projects-gone-quiet measure
// every project the reader may see: a delivery lead asking how many projects
// are in delivery is asking about the installation. An unowned project and a
// project owned outside the caller's teams both count. Naming one owner goes
// through the owner gate (reportownergate.go).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedOutsideProjects plants two live, delivering projects the Team1 manager
// in every case here does not own: one nobody owns, one owned by Team2's Rep3.
func seedOutsideProjects(t *testing.T, e *integration.Env) []ids.UUID {
	t.Helper()
	companyID := ids.NewV7()
	e.WsExec(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Outside Co', 'manual', 'human:x')`, companyID)
	unowned, theirs := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO project (id, name, company_id, phase, source, captured_by)
		VALUES ($1, 'Unowned Delivery', $2, 'delivering', 'manual', 'human:x')`,
		unowned, companyID)
	e.WsExec(t, `INSERT INTO project (id, name, company_id, owner_id, phase, source, captured_by)
		VALUES ($1, 'Team2 Delivery', $2, $3, 'delivering', 'manual', 'human:x')`,
		theirs, companyID, e.Rep3)
	return []ids.UUID{unowned, theirs}
}

// managerScopedTo binds a RowScopeTeam context for a synthetic manager on the
// given teams, granted exactly the "project" (and, for the commitments key,
// "activity") reads the three project reports need.
func managerScopedTo(e *integration.Env, teams []ids.UUID, extra ...string) context.Context {
	objects := map[string]principal.ObjectGrant{
		"project":               {Read: true},
		"installation_settings": {Read: true},
	}
	for _, obj := range extra {
		objects[obj] = principal.ObjectGrant{Read: true}
	}
	return e.As(ids.NewV7(), teams, principal.Permissions{Objects: objects, RowScope: principal.RowScopeTeam})
}

func runProjectReport(ctx context.Context, t *testing.T, e *integration.Env, report, body string) reportResultWire {
	t.Helper()
	rec := postProjectReport(ctx, e, report, body)
	var result reportResultWire
	decodeWire(t, rec, http.StatusOK, &result)
	return result
}

func postProjectReport(ctx context.Context, e *integration.Env, report, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+report, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	reportHandlers{engine: newReportEngine(e.Pool)}.RunReport(rec, req, report)
	return rec
}

func TestProjectsByPhaseCountsEveryReadableProjectForATeamManager(t *testing.T) {
	e := integration.Setup(t)
	seedOutsideProjects(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1})
	result := runProjectReport(manager, t, e, "projects-by-phase",
		`{"group_by":["phase"],"aggregates":[{"fn":"count","as":"projects"}]}`)
	if len(result.Rows) != 1 || wireInt(t, result.Rows[0], "projects") != 2 {
		t.Fatalf("a Team1 manager read %+v, want one delivering row counting 2 — the "+
			"unowned project and Rep3's", result.Rows)
	}
}

// The default plan groups by owner_id beside the project's own id, so each row
// is one project the manager may open, owner and all: a listing, not narrowed.
func TestProjectCommitmentsListsEveryReadableProjectForATeamManager(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedOutsideProjects(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1}, "activity")
	result := runProjectReport(manager, t, e, "project-commitments", `{}`)
	assertListsProjects(t, result, seeded)
}

// A listing's row handles are openable by the caller it was served to. Each
// pins its project's own id, so opening one reads one project the manager may
// open, whoever owns it.
func TestAListingsRowHandlesOpenForTheCallerTheyWereServedTo(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedOutsideProjects(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1}, "activity")
	result := runProjectReport(manager, t, e, "project-commitments", `{}`)
	opened := 0
	for _, row := range result.Rows {
		handle := fmt.Sprint(row["derivation_url"])
		req := httptest.NewRequest(http.MethodGet, handle, nil).WithContext(manager)
		rec := httptest.NewRecorder()
		reportHandlers{engine: newReportEngine(e.Pool)}.ExplainReport(
			rec, req, "project-commitments", crmcontracts.ExplainReportParams{})
		var detail derivationWire
		decodeWire(t, rec, http.StatusOK, &detail)
		if detail.TotalRows != 1 || len(detail.Rows) != 1 || fmt.Sprint(detail.Rows[0]["id"]) != fmt.Sprint(row["project_id"]) {
			t.Errorf("the handle for project %v opened %+v, want that one project", row["project_id"], detail.Rows)
		}
		opened++
	}
	if opened != len(seeded) {
		t.Errorf("opened %d row handles, want one per seeded project (%d)", opened, len(seeded))
	}
}

func TestProjectsGoneQuietListsEveryReadableProjectForATeamManager(t *testing.T) {
	e := integration.Setup(t)
	seeded := seedOutsideProjects(t, e)
	// Backdated well past the default 30-day quiet threshold, with no activity
	// at all, so the report's own "measured from creation" fallback applies.
	e.WsExec(t, `UPDATE project SET created_at = now() - interval '90 days' WHERE id = ANY($1)`, seeded)

	manager := managerScopedTo(e, []ids.UUID{e.Team1})
	result := runProjectReport(manager, t, e, "projects-gone-quiet", `{}`)
	assertListsProjects(t, result, seeded)
}

// The issues' own reproduction named the outside owner in a filter. On an
// install-wide report that is a refusal for a manager of another team, not
// an empty list.
func TestAProjectReportFilteredToAnotherTeamsOwnerIsRefused(t *testing.T) {
	e := integration.Setup(t)
	seedOutsideProjects(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1}, "activity")
	for _, report := range []string{"projects-by-phase", "project-commitments", "projects-gone-quiet"} {
		rec := postProjectReport(manager, e, report, fmt.Sprintf(`{"filters":{"owner_id":%q}}`, e.Rep3))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s filtered to Team2's Rep3 answered %d %s for a Team1 manager, want 403",
				report, rec.Code, rec.Body.String())
		}
	}
}

func assertListsProjects(t *testing.T, result reportResultWire, want []ids.UUID) {
	t.Helper()
	listed := map[string]bool{}
	for _, row := range result.Rows {
		listed[fmt.Sprint(row["project_id"])] = true
	}
	for _, id := range want {
		if !listed[id.String()] {
			t.Errorf("project %s is missing from %+v", id, result.Rows)
		}
	}
	if result.PopulationNarrowed != nil {
		t.Errorf("population_narrowed = %q on a listing, which names each project rather than "+
			"breaking the answer down by owner", *result.PopulationNarrowed)
	}
}
