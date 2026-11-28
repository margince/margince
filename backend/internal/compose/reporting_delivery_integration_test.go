// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func reportingWorkerIdentity(t *testing.T, e *forecastEnv) {
	t.Helper()
	actor, ok := principal.Actor(reportingActor(e))
	if !ok {
		t.Fatal("missing fixture principal")
	}
	raw, err := json.Marshal(struct {
		Objects  map[string]principal.ObjectGrant `json:"objects"`
		RowScope principal.RowScope               `json:"row_scope"`
	}{actor.Permissions.Objects, actor.Permissions.RowScope})
	if err != nil {
		t.Fatal(err)
	}
	role := e.seedID(t, `INSERT INTO role(id,key,name,permissions) VALUES($1,'reporting-test','Reporting test',$2)`, raw)
	e.ownerExec(t, `INSERT INTO role_assignment(role_id,user_id) VALUES($1,$2)`, role, e.Rep1)
	e.ownerExec(t, `UPDATE app_user SET seat_type='full' WHERE id=$1`, e.Rep1)
}

func TestReportingWorkerFreezesReconciledChartsAndUsesCurrentAuthority(t *testing.T) {
	e := setupForecast(t)
	reportingWorkerIdentity(t, e)
	closed, writer, writerCtx := reportingWrittenClose(t, e)
	if closed.ClosedAt == nil {
		t.Fatal("no closing timestamp")
	}
	at := closed.ClosedAt.Add(time.Hour)
	service := newReportingService(e.Pool, func() time.Time { return at })
	human := reportingActor(e)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "owner_attainment"}}
	report, err := service.CreateReport(human, crmcontracts.ReportingReportInput{Name: "Weekly operating review", Audience: "private", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	request, err := service.Freeze(human, ids.UUID(report.Id), report.Revision, "manual-1")
	if err != nil {
		t.Fatal(err)
	}
	repeat, err := service.Freeze(human, ids.UUID(report.Id), report.Revision, "manual-1")
	if err != nil || repeat.Id != request.Id {
		t.Fatalf("idempotency: %+v %v", repeat, err)
	}
	poolConfig := e.Pool.Config()
	poolConfig.MaxConns, poolConfig.MinConns = 1, 0
	publicationPool, err := testdb.OwnPoolFromConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(publicationPool.Close)
	worker := &reportScheduleSweepWorker{pool: publicationPool, now: func() time.Time { return at }}
	if err := worker.Work(boundBackground(t), nil); err != nil {
		t.Fatal(err)
	}
	complete, err := service.GetExecution(human, ids.UUID(request.Id))
	if err != nil {
		t.Fatal(err)
	}
	if complete.Status != "succeeded" || complete.EditionId == nil {
		t.Fatalf("worker did not publish: %+v", complete)
	}
	edition, err := service.GetEdition(human, ids.UUID(*complete.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if *edition.Evaluation.Metrics[0].Value != 21600000 || len(edition.Evaluation.Charts) != 2 {
		t.Fatalf("wrong frozen result: %+v", edition)
	}
	assertReportingEditionTransports(human, t, e, edition)
	amount := int64(5000000)
	if _, err := writer.UpdateDeal(writerCtx, ids.From[ids.DealKind](ids.UUID(closed.Id)), deals.UpdateDealInput{AmountMinor: &amount}); err != nil {
		t.Fatal(err)
	}
	frozen, err := service.GetEdition(human, ids.UUID(edition.Id))
	if err != nil {
		t.Fatal(err)
	}
	if *frozen.Evaluation.Metrics[0].Value != 21600000 {
		t.Fatal("edition was recomputed after the source changed")
	}
	answer, err := readReportingResult(human, service, agents.ReportingRead{Mode: "edition", ID: ids.UUID(edition.Id)})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Edition == nil || *answer.Edition.Evaluation.Metrics[0].Value != 21600000 {
		t.Fatal("MCP and edition service disagree")
	}
	evidence, err := service.EditionEvidence(human, ids.UUID(edition.Id), "bookings_won", "interval", "", nil, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 || *evidence.Rows[0].Value != 21600000 {
		t.Fatalf("frozen evidence: %+v", evidence)
	}
	export, err := reportingCSV(frozen.Evaluation)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(export), "216000.00") {
		t.Fatalf("CSV lost the major-unit amount: %s", export)
	}
	doc := reportdoc.Document{Blocks: []reportdoc.Block{{Kind: reportdoc.KindStatStrip, Cells: []reportdoc.Cell{{EditionRef: &crmcontracts.ReportEditionReference{EditionId: edition.Id, Metric: "bookings_won"}}, {MetricRef: &crmcontracts.ReportMetricReference{Selection: selection, Metric: "bookings_won"}}}}}}
	live, err := service.Evaluate(human, selection)
	if err != nil {
		t.Fatal(err)
	}
	err = InstallationDB(e.Pool).TxIsolated(human, pgx.RepeatableRead, func(tx pgx.Tx) error {
		blocks, err := RenderReport(human, tx, doc, analyticsquery.DefaultFloor, service)
		if err != nil {
			return err
		}
		if len(blocks) != 1 || len(blocks[0].Values) != 2 || blocks[0].Values[0].Value != float64(21600000) || blocks[0].Values[1].Value != *live.Metrics[0].Value {
			t.Fatalf("live and frozen references disagree: %+v", blocks)
		}
		for _, value := range blocks[0].Values {
			if value.Unit == nil || *value.Unit != "EUR" || value.DefinitionVersion == nil || *value.DefinitionVersion != "2" || value.Coverage == nil || value.Context == nil {
				t.Fatalf("reference lost metric metadata: %+v", value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	rule := crmcontracts.ReportingScheduleInput{Frequency: "weekly", Day: 1, LocalTime: "09:00", Enabled: true, ReportRevision: report.Revision}
	if _, err := service.CreateSchedule(human, ids.UUID(report.Id), rule); err == nil {
		t.Fatal("enabled without retention")
	}
	e.ownerExec(t, `INSERT INTO retention_policy(object_type,retain_days,action,enabled) VALUES('report_edition',730,'erase',true)`)
	schedule, err := service.CreateSchedule(human, ids.UUID(report.Id), rule)
	if err != nil {
		t.Fatal(err)
	}
	at = schedule.NextDueAt
	if err := worker.Work(boundBackground(t), nil); err != nil {
		t.Fatal(err)
	}
	runs, err := service.ListExecutions(human, ids.UUID(report.Id), nil, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs.Data) != 2 || runs.Data[0].EditionId == nil {
		t.Fatalf("scheduled edition missing: %+v", runs)
	}
	count, err := service.PauseAll(human)
	if err != nil || count != 1 {
		t.Fatalf("pause: %d %v", count, err)
	}
	at = at.AddDate(0, 0, 14)
	if err := worker.Work(boundBackground(t), nil); err != nil {
		t.Fatal(err)
	}
	paused, err := service.ListSchedules(human, ids.UUID(report.Id))
	if err != nil {
		t.Fatal(err)
	}
	if paused.Data[0].Definition.Enabled {
		t.Fatal("paused schedule restarted")
	}
	reportingRetentionLifecycle(human, t, e, service, ids.UUID(*complete.EditionId))
	e.ownerExec(t, `UPDATE app_user SET archived_at=now() WHERE id=$1`, e.Rep1)
	request, err = service.Freeze(human, ids.UUID(report.Id), report.Revision, "revoked-user")
	if err != nil {
		t.Fatal(err)
	}
	if err := worker.Work(boundBackground(t), nil); err != nil {
		t.Fatal(err)
	}
	denied, err := service.GetExecution(human, ids.UUID(request.Id))
	if err != nil {
		t.Fatal(err)
	}
	if denied.Status != "suspended" || denied.EditionId != nil {
		t.Fatalf("lost authority still published: %+v", denied)
	}
}

func reportingRetentionLifecycle(human context.Context, t *testing.T, e *forecastEnv, service *reporting.Service, editionID ids.UUID) {
	t.Helper()
	e.ownerExec(t, `UPDATE retention_policy SET retain_days=1 WHERE object_type='report_edition'`)
	e.ownerExec(t, `UPDATE report_edition SET captured_at=now()-interval '3 days' WHERE id=$1`, editionID)
	e.ownerExec(t, `UPDATE deal SET legal_hold=true WHERE id IN(SELECT source_id FROM report_edition_contribution WHERE edition_id=$1 AND source_type='deal')`, editionID)
	retention := NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.New(slog.DiscardHandler))
	system := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), "system:privacy-retention")
	if err := retention.EvaluateInstallation(system); err != nil {
		t.Fatal(err)
	}
	held, err := service.GetEdition(human, editionID)
	if err != nil || held.Expired != nil && *held.Expired || editionImages(t, e, editionID) == 0 {
		t.Fatalf("held edition expired or lost its audit images: %+v %v", held, err)
	}
	e.ownerExec(t, `UPDATE deal SET legal_hold=false WHERE id IN(SELECT source_id FROM report_edition_contribution WHERE edition_id=$1 AND source_type='deal')`, editionID)
	if err := retention.EvaluateInstallation(system); err != nil {
		t.Fatal(err)
	}
	expired, err := service.GetEdition(human, editionID)
	if err != nil || expired.Expired == nil || !*expired.Expired || len(expired.Evaluation.Metrics) != 0 || len(expired.Evaluation.Charts) != 0 || editionImages(t, e, editionID) != 0 {
		t.Fatalf("expiry did not preserve a safe tombstone or left an audit image: %+v %v", expired, err)
	}
}

func (e *forecastEnv) ownerExec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func editionImages(t *testing.T, e *forecastEnv, editionID ids.UUID) (images int) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type = 'report_edition'
		AND entity_id = $1 AND (before IS NOT NULL OR after IS NOT NULL)`, editionID).Scan(&images); err != nil {
		t.Fatal(err)
	}
	return images
}
