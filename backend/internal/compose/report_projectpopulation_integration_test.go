// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// projects-by-phase, project-commitments and projects-gone-quiet are the
// exact case reportpopulation.go's own comment names as the motivating
// example for measureEveryReadableRow: "how many projects are in delivery"
// is a question about the installation, not about the asker. A team manager
// must see a project owned by a seat on a different team, not a silently
// narrowed slice of their own team's work.

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

// seedCrossTeamProject plants one live project owned by Rep3 (Team2), the
// same fixture every case in this file measures a Team1 manager against.
func seedCrossTeamProject(t *testing.T, e *integration.Env) ids.UUID {
	t.Helper()
	orgID := ids.NewV7()
	e.WsExec(t, `INSERT INTO organization (id, display_name, source, captured_by)
		VALUES ($1, 'Cross-team Co', 'manual', 'human:x')`, orgID)
	projectID := ids.NewV7()
	e.WsExec(t, `INSERT INTO project (id, name, organization_id, owner_id, phase, source, captured_by)
		VALUES ($1, 'Cross-team Delivery', $2, $3, 'delivering', 'manual', 'human:x')`,
		projectID, orgID, e.Rep3)
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

func TestProjectsByPhaseIsNotNarrowedToATeamManagersOwnTeams(t *testing.T) {
	e := integration.Setup(t)
	seedCrossTeamProject(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1})
	result := runProjectReport(manager, t, e, "projects-by-phase",
		`{"group_by":["phase"],"aggregates":[{"fn":"count","as":"projects"}]}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no projects-by-phase rows, want Rep3's " +
			"Team2 project to still count — this is the installation's own delivery question")
	}
}

func TestProjectCommitmentsIsNotNarrowedToATeamManagersOwnTeams(t *testing.T) {
	e := integration.Setup(t)
	seedCrossTeamProject(t, e)

	manager := managerScopedTo(e, []ids.UUID{e.Team1}, "activity")
	result := runProjectReport(manager, t, e, "project-commitments", `{}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no project-commitments rows, want Rep3's " +
			"Team2 project to still count")
	}
}

func TestProjectsGoneQuietIsNotNarrowedToATeamManagersOwnTeams(t *testing.T) {
	e := integration.Setup(t)
	projectID := seedCrossTeamProject(t, e)
	// Backdated well past the default 30-day quiet threshold, with no activity
	// at all, so the report's own "measured from creation" fallback applies.
	e.WsExec(t, `UPDATE project SET created_at = now() - interval '90 days' WHERE id = $1`, projectID)

	manager := managerScopedTo(e, []ids.UUID{e.Team1})
	result := runProjectReport(manager, t, e, "projects-gone-quiet", `{}`)
	if len(result.Rows) == 0 {
		t.Fatal("a Team1 manager read no projects-gone-quiet rows, want Rep3's " +
			"quiet Team2 project to still count")
	}
}
