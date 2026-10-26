// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
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
	if want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).ghostedThread, ghostedThresholdDays); summary != want {
		t.Errorf("summary = %q, want %q", summary, want)
	}
	if days := signalDaysSilent(t, e, company, kindGhostedThread); days != 15 {
		t.Errorf("the scan recorded %d days silent, want 15", days)
	}
}

// A day mail counts from its date. In Auckland its stored noon UTC is already
// the next local day, so counting the stored instant would say 14.
func TestAGhostedSignalCountsADayMailFromItsDate(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Pacific/Auckland"'::jsonb WHERE key = 'installation.timezone'`)
	zone, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	dated := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 16, 9, 0, 0, 0, zone)

	company := e.SeedCompany(t, "Day Ghost Co", nil)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employedAt(t, e, company, "Dana Ghost")
	logMail(t, e, contact, "inbound", dated.AddDate(0, 0, -5))
	subject, outbound := "Proposal", "outbound"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &outbound, OccurredAt: &dated,
		OccurredPrecision: activities.PrecisionDay, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatalf("logging the day mail: %v", err)
	}

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
	if want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).ghostedThread, ghostedThresholdDays); summary != want {
		t.Errorf("summary = %q, want %q", summary, want)
	}
	if days := signalDaysSilent(t, e, company, kindGhostedThread); days != 15 {
		t.Errorf("the scan recorded %d days silent, want 15", days)
	}
}

// The digest and the signal are two readers of one quiet project. A project
// untouched since 22:00 local, judged at 01:00 local 45 calendar days later,
// is 44 days and three hours quiet by the clock. Both count 45; the signal
// records it on its audit row and says the rule's threshold.
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
	if want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).projectQuiet, "ERP rollout", projects.DefaultProjectQuietDays); summary != want {
		t.Errorf("signal summary = %q, want %q", summary, want)
	}
	if days := signalDaysSilent(t, e, project.UUID, kindProjectGoneQuiet); days != 45 {
		t.Errorf("the scan recorded %d days quiet, want 45", days)
	}
}

// signalDaysSilent is the day count the scan recorded when it raised the
// signal. The stored summary carries no age, so the audit row is where the
// count the scan measured can be read.
func signalDaysSilent(t *testing.T, e *integration.Env, subject ids.UUID, kind string) int {
	t.Helper()
	var days int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT (l.after->>'days_silent')::int FROM audit_log l JOIN signal s ON s.id = l.entity_id
		 WHERE l.entity_type = 'signal' AND l.action = 'create' AND s.entity_id = $1 AND s.kind = $2`,
		subject, kind).Scan(&days); err != nil {
		t.Fatalf("reading the recorded day count: %v", err)
	}
	return days
}

// A later scan finds the same silence. It raises no second signal, and the
// summary it stored still holds, because it names the threshold, not an age.
func TestALaterScanKeepsOneGhostedSignalWhoseSummaryStillHolds(t *testing.T) {
	e := integration.Setup(t)
	wrote := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Later Ghost Co", nil)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employedAt(t, e, company, "Lee Ghost")
	logMail(t, e, contact, "inbound", wrote.AddDate(0, 0, -5))
	logMail(t, e, contact, "outbound", wrote)

	ctx := principal.SystemActing(e.Admin(), signalScanActor)
	want := fmt.Sprintf(signalSummaryCopyFor(textlang.English).ghostedThread, ghostedThresholdDays)
	for _, now := range []time.Time{wrote.AddDate(0, 0, 15), wrote.AddDate(0, 0, 25)} {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			_, err := WriteGhostedSignals(ctx, tx, now)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE entity_id = $1 AND kind = $2`, company, kindGhostedThread); n != 1 {
		t.Fatalf("two scans raised %d signals, want 1", n)
	}
	if got := e.WsScalar(t, `SELECT summary FROM signal WHERE entity_id = $1 AND kind = $2`, company, kindGhostedThread); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}
