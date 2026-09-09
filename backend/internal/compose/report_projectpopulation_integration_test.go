// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// projects-by-phase, project-commitments and projects-gone-quiet keep the
// caller's own/team lens: owner_id is a dimension (projects-by-phase) or
// part of defaultBy (the other two) over `project`, an identity table with
// no other narrowing — a wider population would let a rep read a named
// colleague's exact delivery figures (projects-by-phase's aggregates are
// money) or workload (the other two) by filtering or grouping on their id.
// An unowned project — one nobody has claimed — must still count toward a
// team manager's own managed-teams population, the same unowned-row fix
// every report in this cluster takes.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedUnownedProject plants one live, unowned project — the same fixture
// every case in this file measures a Team1 manager against.
func seedUnownedProject(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	companyID := ids.NewV7()
	e.WsExec(t, `INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Unowned Co', 'manual', 'human:x')`, companyID)
	projectID := ids.NewV7()
	e.WsExec(t, `INSERT INTO project (id, name, company_id, phase, source, captured_by)
		VALUES ($1, 'Unowned Delivery', $2, 'delivering', 'manual', 'human:x')`,
		projectID, companyID)
	return projectID
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
	req := httptest.NewRequest(http.MethodPost, "/v1/reports/"+report, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	reportHandlers{engine: newReportEngine(e.Pool)}.RunReport(rec, req, report)
	var result reportResultWire
	decodeWire(t, rec, http.StatusOK, &result)
	return result
}

func TestProjectsByPhaseCountsAnUnownedProjectForATeamManager(t *testing.T) {
	e := integration.Setup(t)
	seedUnownedProject(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1})
	result := runProjectReport(manager, t, e, "projects-by-phase",
		`{"group_by":["phase"],"aggregates":[{"fn":"count","as":"projects"}]}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no projects-by-phase rows, want the " +
			"unowned project to still count toward their own managed-teams population")
	}
}

func TestProjectCommitmentsCountsAnUnownedProjectForATeamManager(t *testing.T) {
	e := integration.Setup(t)
	seedUnownedProject(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1}, "activity")
	result := runProjectReport(manager, t, e, "project-commitments", `{}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no project-commitments rows, want the " +
			"unowned project to still count")
	}
}

func TestProjectsGoneQuietCountsAnUnownedProjectForATeamManager(t *testing.T) {
	e := integration.Setup(t)
	projectID := seedUnownedProject(t, e)
	// Backdated well past the default 30-day quiet threshold, with no activity
	// at all, so the report's own "measured from creation" fallback applies.
	e.WsExec(t, `UPDATE project SET created_at = now() - interval '90 days' WHERE id = $1`, projectID)

	manager := managerScopedTo(e, []ids.UUID{e.Team1})
	result := runProjectReport(manager, t, e, "projects-gone-quiet", `{}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no projects-gone-quiet rows, want the " +
			"quiet, unowned project to still count")
	}
}
