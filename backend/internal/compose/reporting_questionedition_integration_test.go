// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// questionEditions sends a report whose layout asks one question, counting
// Rep1's open deals.
type questionEditions struct {
	e       *forecastEnv
	service *reporting.Service
	human   context.Context
	report  crmcontracts.ReportingReport
	worker  *reportScheduleSweepWorker
	sends   int
}

func newQuestionEditions(t *testing.T) *questionEditions {
	t.Helper()
	e := setupForecast(t)
	reportingWorkerIdentity(t, e)
	amount := int64(100_000)
	for range 7 {
		e.seedOpenDeal(t, "Counted", 20, &e.Rep1, &amount, nil)
	}
	at := time.Date(2026, 9, 24, 13, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	human := reportingActor(e)
	question := crmcontracts.ReportingCardQuestion{
		Entity:   "open-deals-per-company",
		GroupBy:  &[]string{"company_id"},
		Measures: []crmcontracts.AnalyticsMeasure{{Fn: "count", As: new("deals")}},
	}
	layout := []crmcontracts.ReportingSection{{Cards: []crmcontracts.ReportingCard{
		{Id: "trend", Kind: "bookings_trend", Width: crmcontracts.ReportingCardWidthFull},
		{Id: "open-deals", Kind: "question", Question: &question, Width: crmcontracts.ReportingCardWidthFull},
	}}}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Layout: &layout}
	report, err := service.CreateReport(human, crmcontracts.ReportingReportInput{Name: "Open deals review", Audience: "workspace", Selection: selection})
	if err != nil {
		t.Fatal(err)
	}
	poolConfig := e.Pool.Config()
	poolConfig.MaxConns, poolConfig.MinConns = 1, 0
	publicationPool, err := testdb.OwnPoolFromConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(publicationPool.Close)
	worker := &reportScheduleSweepWorker{pool: publicationPool, now: func() time.Time { return at }}
	return &questionEditions{e: e, service: service, human: human, report: report, worker: worker}
}

// send publishes one edition and reads it back as its sender.
func (q *questionEditions) send(t *testing.T) crmcontracts.ReportingEdition {
	t.Helper()
	q.sends++
	request, err := q.service.Freeze(q.human, ids.UUID(q.report.Id), q.report.Revision, fmt.Sprintf("question-%d", q.sends))
	if err != nil {
		t.Fatal(err)
	}
	if err := q.worker.Work(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	done, err := q.service.GetExecution(q.human, ids.UUID(request.Id))
	if err != nil || done.EditionId == nil {
		t.Fatalf("the edition was not published: %+v %v", done, err)
	}
	return q.read(q.human, t, ids.UUID(*done.EditionId))
}

func (q *questionEditions) read(reader context.Context, t *testing.T, edition ids.UUID) crmcontracts.ReportingEdition {
	t.Helper()
	out, err := q.service.GetEdition(reader, edition)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (q *questionEditions) keepForAYear(t *testing.T) {
	t.Helper()
	if _, err := q.e.owner.Exec(context.Background(), `INSERT INTO retention_policy(object_type,retain_days,action,enabled) VALUES('report_edition',365,'erase',true)`); err != nil {
		t.Fatal(err)
	}
}

func onlyQuestion(t *testing.T, edition crmcontracts.ReportingEdition) crmcontracts.ReportingEditionQuestion {
	t.Helper()
	if edition.Questions == nil || len(*edition.Questions) != 1 || (*edition.Questions)[0].CardId != "open-deals" {
		t.Fatalf("the edition's question cards read %+v, want the one card", edition.Questions)
	}
	return (*edition.Questions)[0]
}

// An edition keeps an answer only under a retention policy, which is what
// later erases it. Under one, the sender reads back the answer they were
// given, resting on the records it counted.
func TestASentEditionKeepsItsQuestionAnswerUnderARetentionPolicy(t *testing.T) {
	q := newQuestionEditions(t)

	unkept := onlyQuestion(t, q.send(t))
	if unkept.Status != crmcontracts.ReportingEditionQuestionStatusNotKept || unkept.Reason == nil ||
		*unkept.Reason != crmcontracts.ReportingEditionQuestionReasonRetentionUnconfigured || unkept.Answer != nil {
		t.Fatalf("with no retention policy the card reads %+v, want not kept for that reason", unkept)
	}

	q.keepForAYear(t)
	edition := q.send(t)
	kept := onlyQuestion(t, edition)
	if kept.Status != crmcontracts.ReportingEditionQuestionStatusKept || kept.Answer == nil || len(kept.Answer.Rows) != 1 {
		t.Fatalf("under a retention policy the card reads %+v, want the kept answer", kept)
	}
	if got := kept.Answer.Rows[0]["deals"]; got != float64(7) && got != int64(7) {
		t.Fatalf("the kept answer counts %v, want Rep1's 7 open deals", got)
	}
	var sources int
	if err := q.e.owner.QueryRow(context.Background(), `SELECT count(*) FROM report_edition_question_source WHERE edition_id=$1 AND source_type='deal'`, edition.Id).Scan(&sources); err != nil {
		t.Fatal(err)
	}
	if sources != 7 {
		t.Fatalf("the kept answer rests on %d deals, want the 7 it counted", sources)
	}
}

// A reader who may measure only their own deals cannot ask about Rep1's, so
// nothing of the answer Rep1 was given reaches them.
func TestAKeptAnswerShowsNothingToAReaderWhoMayNotAskIt(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)

	reader := q.e.ownLensRepCtx(q.e.Rep3)
	actor, _ := principal.Actor(reader)
	for _, object := range []string{"report_definition", "report_edition"} {
		actor.Permissions.Objects[object] = principal.ObjectGrant{Read: true}
	}
	reader = principal.WithActor(reader, actor)

	seen := onlyQuestion(t, q.read(reader, t, ids.UUID(edition.Id)))
	if seen.Status == crmcontracts.ReportingEditionQuestionStatusKept && seen.Answer != nil {
		for _, row := range seen.Answer.Rows {
			if row["_withheld"] != true {
				t.Fatalf("a rep who may not measure Rep1's deals read the kept row %v", row)
			}
		}
		return
	}
	if seen.Status != crmcontracts.ReportingEditionQuestionStatusWithheld || seen.Answer != nil {
		t.Fatalf("the narrower reader's card reads %+v, want it withheld", seen)
	}
}

// A legal hold on a record behind a kept answer keeps the edition from
// expiring. The chart contributions are removed first, so only the answer's
// records can hold it. Once the hold lifts, expiry removes the answer.
func TestALegalHoldBehindAKeptAnswerHoldsItsEdition(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)
	ctx := context.Background()
	for _, statement := range []string{
		`DELETE FROM report_edition_contribution WHERE edition_id=$1`,
		`UPDATE report_edition SET captured_at=now()-interval '3 days' WHERE id=$1`,
		`UPDATE deal SET legal_hold=true WHERE id=(SELECT min(source_id::text)::uuid FROM report_edition_question_source WHERE edition_id=$1)`,
	} {
		if _, err := q.e.owner.Exec(ctx, statement, edition.Id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.e.owner.Exec(ctx, `UPDATE retention_policy SET retain_days=1 WHERE object_type='report_edition'`); err != nil {
		t.Fatal(err)
	}
	retention := NewRetentionServiceFor(InstallationDB(q.e.Pool), nil, slog.New(slog.DiscardHandler))
	system := principal.SystemActing(principal.WithWorkspaceID(ctx, q.e.WS), "system:privacy-retention")
	if err := retention.EvaluateInstallation(system); err != nil {
		t.Fatal(err)
	}
	held := q.read(q.human, t, ids.UUID(edition.Id))
	if held.Expired != nil && *held.Expired || onlyQuestion(t, held).Status != crmcontracts.ReportingEditionQuestionStatusKept {
		t.Fatalf("a held record behind the answer did not hold the edition: %+v", held)
	}

	if _, err := q.e.owner.Exec(ctx, `UPDATE deal SET legal_hold=false`); err != nil {
		t.Fatal(err)
	}
	if err := retention.EvaluateInstallation(system); err != nil {
		t.Fatal(err)
	}
	expired := q.read(q.human, t, ids.UUID(edition.Id))
	if expired.Expired == nil || !*expired.Expired || (expired.Questions != nil && len(*expired.Questions) != 0) {
		t.Fatalf("expiry left the kept answer: %+v", expired.Questions)
	}
}

// Subject access hands over each kept answer row a record about the subject
// was counted in, and the subject's erasure removes the edition's answers.
func TestASubjectsKeptAnswersAreHandedOverAndErased(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)
	subject := q.e.seedID(t, `INSERT INTO contact(id,full_name,source,captured_by) VALUES($1,'Erika Mustermann','manual','human:x')`)
	if _, err := q.e.owner.Exec(context.Background(), `INSERT INTO relationship(kind,contact_id,deal_id,source,captured_by)
		SELECT 'deal_stakeholder',$1,min(source_id::text)::uuid,'manual','human:x' FROM report_edition_question_source WHERE edition_id=$2`, subject, edition.Id); err != nil {
		t.Fatal(err)
	}
	controller := privacyController(q.e)

	pkg, err := privacy.AssembleSAR(controller, InstallationDB(q.e.Pool), ids.From[ids.ContactKind](subject))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.ReportingQuestionAnswers) != 1 {
		t.Fatalf("subject access handed over %d kept answer rows, want the one the subject's deal was counted in", len(pkg.ReportingQuestionAnswers))
	}

	if err := privacy.NewEraser(InstallationDB(q.e.Pool)).EraseContact(controller, subject, "subject request"); err != nil {
		t.Fatal(err)
	}
	erased := q.read(q.human, t, ids.UUID(edition.Id))
	if !erased.Redacted || (erased.Questions != nil && len(*erased.Questions) != 0) {
		t.Fatalf("erasure left the kept answer or did not mark the edition: redacted=%v %+v", erased.Redacted, erased.Questions)
	}
}

// privacyController may answer and erase for a rights-case subject.
func privacyController(e *forecastEnv) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"contact", "company", "deal", "activity", "lead", "project", "report_edition", "report_definition"} {
		grants[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:controller", UserID: e.Rep1, SeatType: principal.SeatFull, Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants}})
}

// A kept answer stays only while every record behind it is still in the
// reader's population. Here one counted deal moves to another owner and leaves
// the report's scope. Even the sender is then shown nothing of the answer.
func TestAKeptAnswerIsWithheldOnceOneOfItsRecordsLeavesTheReadersReach(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)
	if _, err := q.e.owner.Exec(context.Background(), `UPDATE deal SET owner_id=$1 WHERE id=(SELECT min(source_id::text)::uuid FROM report_edition_question_source WHERE edition_id=$2)`, q.e.Rep3, edition.Id); err != nil {
		t.Fatal(err)
	}
	seen := onlyQuestion(t, q.read(q.human, t, ids.UUID(edition.Id)))
	if seen.Status != crmcontracts.ReportingEditionQuestionStatusWithheld || seen.Answer != nil {
		t.Fatalf("the card reads %+v after one of its deals left the scope, want it withheld", seen)
	}
}

// A record that stays readable but moves to another group no longer vouches
// for its row. The kept answer is then withheld.
func TestAKeptAnswerIsWithheldOnceOneOfItsRecordsChangesGroup(t *testing.T) {
	q := newQuestionEditions(t)
	q.keepForAYear(t)
	edition := q.send(t)
	if before := onlyQuestion(t, q.read(q.human, t, ids.UUID(edition.Id))); before.Status != crmcontracts.ReportingEditionQuestionStatusKept {
		t.Fatalf("the sender's card reads %+v before anything moved, want the kept answer", before)
	}
	company := q.e.seedID(t, `INSERT INTO company(id,display_name,source,captured_by) VALUES($1,'Moved To GmbH','manual','human:x')`)
	if _, err := q.e.owner.Exec(context.Background(), `UPDATE deal SET company_id=$1 WHERE id=(SELECT min(source_id::text)::uuid FROM report_edition_question_source WHERE edition_id=$2)`, company, edition.Id); err != nil {
		t.Fatal(err)
	}
	seen := onlyQuestion(t, q.read(q.human, t, ids.UUID(edition.Id)))
	if seen.Status != crmcontracts.ReportingEditionQuestionStatusWithheld {
		t.Fatalf("the card reads %+v after a counted deal changed company, want it withheld", seen)
	}
}
