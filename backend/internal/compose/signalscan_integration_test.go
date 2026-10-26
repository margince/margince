// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A mail written at 22:00 local and judged at 01:00 local, fifteen calendar
// days later, is fourteen days and three hours by the clock. The summary counts
// the calendar days of the installation.
func TestAGhostedSignalCountsCalendarDaysInTheInstallationZone(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Asia/Ho_Chi_Minh"'::jsonb WHERE key = 'installation.timezone'`)
	zone, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	wrote := time.Date(2026, 9, 1, 22, 0, 0, 0, zone)
	now := time.Date(2026, 9, 16, 1, 0, 0, 0, zone)

	company := e.SeedCompany(t, "Zone Ghost Co", nil)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employedAt(t, e, company, "Zoe Ghost")
	logMail(t, e, contact, "inbound", wrote.AddDate(0, 0, -5))
	logMail(t, e, contact, "outbound", wrote)

	ctx := principal.SystemActing(e.Admin(), signalScanActor)
	var summary string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := WriteGhostedSignals(ctx, tx, now); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT summary FROM signal WHERE entity_id = $1 AND kind = $2`,
			company, kindGhostedThread).Scan(&summary)
	}); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).ghostedThread, 15); summary != want {
		t.Errorf("summary = %q, want %q", summary, want)
	}
}

// The digest and the signal are two readers of one quiet project. A project
// untouched since 22:00 local, judged at 01:00 local 45 calendar days later,
// is 44 days and three hours quiet by the clock. Both say 45.
func TestTheDigestAndTheSignalCountAQuietProjectInTheSameCalendarDays(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Asia/Ho_Chi_Minh"'::jsonb WHERE key = 'installation.timezone'`)
	zone, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	company := e.SeedCompany(t, "Quiet Zone Co", nil)
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "ERP rollout", CompanyID: ids.From[ids.CompanyKind](company), OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	project := ids.From[ids.ProjectKind](ids.UUID(created.Id))
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), project, projects.AdvanceProjectPhaseInput{ToPhase: "pursuing"}); err != nil {
		t.Fatalf("advancing the project: %v", err)
	}
	quietSince := time.Date(2026, 9, 1, 22, 0, 0, 0, zone)
	now := time.Date(2026, 10, 16, 1, 0, 0, 0, zone)
	e.WsExec(t, `UPDATE project SET created_at = $2 WHERE id = $1`, project, quietSince)

	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"project": {Read: true}},
		RowScope: principal.RowScopeTeam,
	})
	var digestDays int
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		quiet, err := digestQuietProjects(reader, tx, now)
		if err != nil {
			return err
		}
		if len(quiet) != 1 {
			return fmt.Errorf("digest lists %d quiet projects, want 1", len(quiet))
		}
		digestDays = quiet[0].DaysQuiet
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx := principal.SystemActing(e.Admin(), signalScanActor)
	var summary string
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := WriteProjectQuietSignals(ctx, tx, now); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT summary FROM signal WHERE entity_id = $1 AND kind = $2`,
			project, kindProjectGoneQuiet).Scan(&summary)
	}); err != nil {
		t.Fatal(err)
	}
	if digestDays != 45 {
		t.Errorf("digest days_quiet = %d, want 45", digestDays)
	}
	if want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).projectQuiet, "ERP rollout", 45); summary != want {
		t.Errorf("signal summary = %q, want %q", summary, want)
	}
}
