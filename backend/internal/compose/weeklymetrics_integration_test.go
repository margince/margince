// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWeeklyUsesAnalyticsDefinitionsAndKeepsFrozenFigures(t *testing.T) {
	testWeeklyReportingPrivacy(t, false)
}

func TestRetentionAnonymizationRedactsFrozenReporting(t *testing.T) {
	testWeeklyReportingPrivacy(t, true)
}

func testWeeklyReportingPrivacy(t *testing.T, anonymize bool) {
	e := setupForecast(t)
	human := reportingActor(e)
	closed, dealsStore, writer := reportingWrittenClose(t, e)
	transferred := ids.From[ids.UserKind](e.Rep3)
	if _, err := dealsStore.UpdateDeal(writer, ids.From[ids.DealKind](ids.UUID(closed.Id)), deals.UpdateDealInput{OwnerID: &transferred}); err != nil {
		t.Fatal(err)
	}
	actor, _ := principal.Actor(human)
	actor.UserID, actor.ID = e.Rep3, "human:"+e.Rep3.String()
	human = principal.WithActor(human, actor)
	contact := ids.NewV7()
	var b reportingBindings
	if _, err := e.owner.Exec(human, "INSERT INTO contact(id,first_name,last_name,full_name,source,captured_by) VALUES("+b.add(contact)+","+b.add("Jamie")+","+b.add("Buyer")+","+b.add("Jamie Buyer")+","+b.add("manual")+","+b.add("human:"+e.Rep1.String())+")", b.values...); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(human, "INSERT INTO relationship(kind,contact_id,deal_id,role,source,captured_by) VALUES('deal_stakeholder',$1,$2,'economic_buyer','manual','test')", contact, closed.Id); err != nil {
		t.Fatal(err)
	}
	observed := time.Date(2026, 9, 25, 15, 0, 0, 0, time.UTC)
	activityStore := activities.NewStore(InstallationDB(e.Pool)).WithClock(func() time.Time { return observed })
	held := "held"
	subject := "Customer review"
	scheduled := observed.Add(-time.Hour)
	input := activities.LogActivityInput{Kind: "meeting", Subject: &subject, OccurredAt: &scheduled, MeetingStatus: &held, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}}
	meeting, _, err := activityStore.LogActivity(human, input)
	if err != nil {
		t.Fatal(err)
	}
	input.MeetingStatus = nil
	if _, _, err := activityStore.LogActivity(human, input); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC)
	engine := weekly.NewEngine(e.Pool, newTeammatesSeam(e.Pool)).WithNumeric(weeklyNumericEvaluator{})
	former, _, err := engine.AssembleFor(reportingActor(e), at)
	if err != nil || former.Counts.DealsWon != 0 {
		t.Fatalf("former owner's Weekly retained the win: %+v %v", former, err)
	}
	review, created, err := engine.AssembleFor(human, at)
	if err != nil {
		t.Fatal(err)
	}
	if !created || review.Counts.DealsWon != 1 || review.Counts.MeetingsHeld != 1 || review.NumericSummary == nil {
		t.Fatalf("shared Weekly counts: %+v", review)
	}
	service := newReportingService(e.Pool, func() time.Time { return at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep3)}, Period: "last_week", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "meetings_held"}, Blocks: []crmcontracts.ReportingBlockKind{"metric_reading"}}
	evaluation, err := service.Evaluate(human, selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range evaluation.Metrics {
		if metric.Value == nil {
			t.Fatalf("missing metric: %+v", metric)
		}
		switch metric.Id {
		case "bookings_won":
			if review.NumericSummary.WonMinor == nil || float64(*review.NumericSummary.WonMinor) != *metric.Value {
				t.Fatal("Weekly and Analytics bookings diverge")
			}
		case "meetings_held":
			if float64(review.Counts.MeetingsHeld) != *metric.Value {
				t.Fatal("Weekly and Analytics confirmed meetings diverge")
			}
		}
	}
	observed = at.Add(time.Hour)
	cancelled := "canceled"
	if _, err := activityStore.UpdateActivity(human, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), activities.UpdateActivityInput{MeetingStatus: &cancelled}); err != nil {
		t.Fatal(err)
	}
	frozen, created, err := engine.AssembleFor(human, at.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if created || frozen.Counts.MeetingsHeld != 1 || frozen.NumericSummary == nil || *frozen.NumericSummary.WonMinor != 21600000 {
		t.Fatalf("frozen Weekly changed: %+v", frozen)
	}
	reportingWorkerIdentity(t, e)
	human = reportingActor(e)
	report, err := service.CreateReport(human, crmcontracts.ReportingReportInput{Name: "Confirmed customer outcomes", Audience: "private", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Freeze(human, ids.UUID(report.Id), report.Revision, "privacy-fixture")
	if err != nil {
		t.Fatal(err)
	}
	worker := &reportScheduleSweepWorker{enabled: true, pool: e.Pool, now: func() time.Time { return at }}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	completed, err := service.GetExecution(human, ids.UUID(run.Id))
	if err != nil || completed.EditionId == nil {
		t.Fatalf("freeze: %+v %v", completed, err)
	}
	pkg, err := privacy.AssembleSAR(human, InstallationDB(e.Pool), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.ReportingContributions) == 0 {
		t.Fatal("subject export omitted frozen contributions")
	}
	stale := InstallationDB(e.Pool).TxIsolated(human, pgx.RepeatableRead, func(tx pgx.Tx) error {
		var generation int64
		if err := tx.QueryRow(human, "SELECT generation FROM report_projection_fence").Scan(&generation); err != nil {
			return err
		}
		var privacyErr error
		if anonymize {
			privacyErr = anonymizeWeeklyReportingSubject(t, e, contact)
		} else {
			privacyErr = privacy.NewEraser(InstallationDB(e.Pool)).EraseContact(human, contact, "Subject request")
		}
		if err := privacyErr; err != nil {
			return err
		}
		return storekit.FenceReportProjection(human, tx)
	})
	var serialization *pgconn.PgError
	if !errors.As(stale, &serialization) || serialization.Code != "40001" {
		t.Fatalf("pre-erasure snapshot could publish: %v", stale)
	}
	redacted, err := service.GetEdition(human, ids.UUID(*completed.EditionId))
	if err != nil {
		t.Fatal(err)
	}
	if !redacted.Redacted || !redacted.Withheld {
		t.Fatal("archive did not disclose redaction")
	}
	for _, metric := range redacted.Evaluation.Metrics {
		if metric.Id == "bookings_won" && (metric.Value == nil || *metric.Value != 21600000 || metric.Coverage.Status != "partial") {
			t.Fatalf("retained business total lost after stakeholder erasure: %+v", metric)
		}
		if metric.Id == "meetings_held" && metric.Value != nil && *metric.Value != 0 {
			t.Fatal("erased meeting survives the archive")
		}
	}
	evidence, err := service.EditionEvidence(human, ids.UUID(*completed.EditionId), "bookings_won", "interval", "", nil, nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence.Rows) != 1 || evidence.Rows[0].Label != "Deal" {
		t.Fatalf("stakeholder label survived: %+v", evidence)
	}
}

func anonymizeWeeklyReportingSubject(t *testing.T, e *forecastEnv, contact ids.UUID) error {
	t.Helper()
	ctx := context.Background()
	for _, query := range []string{
		"UPDATE contact SET created_at=now()-interval '3 days',entered_at=now()-interval '3 days' WHERE id=$1",
		"UPDATE relationship SET archived_at=now() WHERE contact_id=$1 AND kind='deal_stakeholder'",
	} {
		if _, err := e.owner.Exec(ctx, query, contact); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.owner.Exec(ctx, "UPDATE retention_policy SET enabled=false"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, "INSERT INTO retention_policy(object_type,category,retain_days,action) VALUES('contact','no_consent_no_deal',1,'anonymize')"); err != nil {
		t.Fatal(err)
	}
	retention := NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.New(slog.DiscardHandler))
	system := principal.SystemActing(principal.WithWorkspaceID(ctx, e.WS), "system:privacy-retention")
	return retention.EvaluateInstallation(system)
}
