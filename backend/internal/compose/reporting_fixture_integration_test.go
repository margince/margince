// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/searchplan"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type reportingBusinessFixture struct {
	env          *forecastEnv
	at           time.Time
	service      *reporting.Service
	store        *deals.Store
	human        context.Context
	writer       context.Context
	open         []crmcontracts.Deal
	inboundLeads []ids.LeadID
	target       crmcontracts.ReportingTarget
}

func reportingBusiness(t *testing.T) *reportingBusinessFixture {
	t.Helper()
	e := setupForecast(t)
	f := &reportingBusinessFixture{env: e, at: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), human: reportingActor(e)}
	actor, _ := principal.Actor(f.human)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	f.human = principal.WithActor(f.human, actor)
	f.writer = principal.SystemActing(f.human, "system:reporting-fixture")
	f.service = newReportingService(e.Pool, func() time.Time { return f.at })
	f.store = deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).SetClock(func() time.Time { return f.at })
	_, err := f.service.PublishFramework(f.human, 0, crmcontracts.ReportingFrameworkInput{Template: "sales", Reason: "Qualification begins at proposal", Qualification: []crmcontracts.ReportingQualification{{PipelineId: openapi_types.UUID(e.pipeline), StageIds: []openapi_types.UUID{openapi_types.UUID(e.stages[60])}}}, CaptureContexts: []crmcontracts.ReportingCaptureContext{{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, PipelineId: ptrUUID(e.pipeline)}}})
	if err != nil {
		t.Fatal(err)
	}
	f.closedDeals(t)
	f.openDeals(t)
	f.outcomes(t)
	f.leads(t)
	f.at = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	f.target, err = f.service.CreateTarget(f.human, crmcontracts.ReportingTargetInput{Metric: "bookings_won", PipelineId: ptrUUID(e.pipeline), Scope: crmcontracts.ReportingScope{Kind: "workspace"}, PeriodKind: "month", PeriodStart: openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}, Value: 30000000, Reason: "Monthly commitment"})
	if err != nil {
		t.Fatal(err)
	}
	reportingWorkerIdentity(t, e)
	return f
}

func (f *reportingBusinessFixture) createDeal(t *testing.T, name string, amount int64, stage ids.UUID, owner ids.UUID) crmcontracts.Deal {
	t.Helper()
	currency := "EUR"
	user := ids.From[ids.UserKind](owner)
	closeAt := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	deal, err := f.store.CreateDeal(f.writer, deals.CreateDealInput{Name: name, AmountMinor: &amount, Currency: &currency, PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), StageID: ids.From[ids.StageKind](stage), OwnerID: &user, ExpectedClose: &closeAt, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return deal
}

func (f *reportingBusinessFixture) closedDeals(t *testing.T) {
	t.Helper()
	won := f.env.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Won',10,'won',100)`, f.env.pipeline)
	lost := f.env.seedID(t, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Lost',11,'lost',0)`, f.env.pipeline)
	for i := range 20 {
		f.at = time.Date(2026, 9, 3+i, 12, 0, 0, 0, time.UTC)
		owner := f.env.Rep1
		if i%2 == 1 {
			owner = f.env.Rep3
		}
		deal := f.createDeal(t, fmt.Sprintf("Outcome %02d", i), 1800000, f.env.stages[20], owner)
		f.at = f.at.Add(time.Hour)
		reason := "purchase_order"
		input := deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](won), WonWithoutContractReason: &reason}
		if i >= 12 {
			reason = "price"
			input = deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](lost), LostReason: &reason, LostReasonCode: new("other")}
		}
		if _, err := f.store.AdvanceDeal(f.writer, ids.From[ids.DealKind](ids.UUID(deal.Id)), input); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *reportingBusinessFixture) openDeals(t *testing.T) {
	t.Helper()
	cohorts := []struct {
		stage  int
		amount int64
		ages   []int
	}{{20, 4800000, []int{5, 7, 8, 13, 17}}, {55, 7200000, []int{14, 17, 21, 27, 32}}, {60, 5600000, []int{25, 28, 31, 44, 50}}}
	for _, cohort := range cohorts {
		for i, age := range cohort.ages {
			f.at = time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC).AddDate(0, 0, -age)
			owner := f.env.Rep1
			if i%2 == 1 {
				owner = f.env.Rep3
			}
			f.open = append(f.open, f.createDeal(t, fmt.Sprintf("Pipeline %d/%d", cohort.stage, i), cohort.amount, f.env.stages[cohort.stage], owner))
		}
	}
}

func (f *reportingBusinessFixture) outcomes(t *testing.T) {
	t.Helper()
	store := contacts.NewStore(InstallationDB(f.env.Pool)).SetClock(func() time.Time { return f.at })
	contact, err := store.CreateContact(f.human, contacts.CreateContactInput{FullName: "Customer fixture", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	contactID := ids.UUID(contact.Id)
	meetings := activities.NewStore(InstallationDB(f.env.Pool)).WithClock(func() time.Time { return f.at })
	for i := range 12 {
		f.at = time.Date(2026, 9, 5+i, 15, 0, 0, 0, time.UTC)
		held, subject := "held", fmt.Sprintf("Customer meeting %02d", i)
		scheduled := f.at.Add(-time.Hour)
		_, _, err := meetings.LogActivity(f.human, activities.LogActivityInput{Kind: "meeting", Subject: &subject, OccurredAt: &scheduled, MeetingStatus: &held, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contactID}}})
		if err != nil {
			t.Fatal(err)
		}
		if i < 6 {
			handoff, err := store.SubmitHandoff(f.human, contacts.NewSDRHandoff{ContactID: &contactID})
			if err != nil {
				t.Fatal(err)
			}
			dealID := ids.UUID(f.open[i].Id)
			if err := store.DecideHandoff(f.human, handoff, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID}); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// leads writes six leads that each wrote in by email on 10 September. Five
// were answered after 1, 2, 4, 8 and 30 hours, and one is still waiting.
func (f *reportingBusinessFixture) leads(t *testing.T) {
	t.Helper()
	store := contacts.NewStore(InstallationDB(f.env.Pool))
	mail := activities.NewStore(InstallationDB(f.env.Pool)).WithClock(func() time.Time { return f.at })
	arrived := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	owner := ids.From[ids.UserKind](f.env.Rep1)
	inbound, subject := "inbound", "Pricing question"
	for i, hours := range []int{1, 2, 4, 8, 30, 0} {
		name := fmt.Sprintf("Inbound lead %d", i)
		lead, _, err := store.CreateLead(f.writer, contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: &owner, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		f.inboundLeads = append(f.inboundLeads, ids.From[ids.LeadKind](ids.UUID(lead.Id)))
		// The lead writer stamps the database's clock, so the arrival is
		// moved into the fixture's month before anything answers it.
		if _, err := f.env.owner.Exec(context.Background(), `UPDATE lead SET created_at=$2 WHERE id=$1`, lead.Id, arrived); err != nil {
			t.Fatal(err)
		}
		f.at = arrived
		if _, _, err := mail.LogActivity(f.human, activities.LogActivityInput{Kind: "email", Subject: &subject, OccurredAt: &arrived, Direction: &inbound, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}}}); err != nil {
			t.Fatal(err)
		}
		if hours > 0 {
			if _, err := store.RecordLeadFirstResponse(f.writer, ids.From[ids.LeadKind](ids.UUID(lead.Id)), arrived.Add(time.Duration(hours)*time.Hour)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func (f *reportingBusinessFixture) selection() crmcontracts.ReportingSelection {
	return crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, PipelineId: ptrUUID(f.env.pipeline), Period: "this_month", TargetBasis: "month", CloseWindow: "fiscal_quarter", Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate", "open_pipeline", "stage_age"}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution", "owner_attainment", "stage_age"}}
}

func TestReportingBusinessFixtureReconcilesEveryChartWithItsEvidence(t *testing.T) {
	t.Parallel()
	f := reportingBusiness(t)
	selection := f.selection()
	result, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	want := map[crmcontracts.ReportingMetricID]float64{"bookings_won": 21600000, "open_pipeline": 88000000, "closed_win_rate": 60, "stage_age": 21}
	for _, metric := range result.Metrics {
		if metric.Value == nil || *metric.Value != want[metric.Id] {
			t.Fatalf("metric %s: %+v", metric.Id, metric)
		}
	}
	for _, chart := range result.Charts {
		for _, point := range chart.Points {
			if point.Value == nil || point.Evidence == nil {
				t.Fatalf("missing chart mark: %+v", point)
			}
			ref := point.Evidence
			group := ""
			if ref.GroupKey != nil {
				group = *ref.GroupKey
			}
			evidence, err := f.service.Evidence(f.human, selection, ref.Metric, ref.ContextId, group, ref.Through, nil, 100, reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision})
			if err != nil {
				t.Fatal(err)
			}
			if chart.Kind != "stage_age" {
				sum := float64(0)
				for _, row := range evidence.Rows {
					if row.Value != nil {
						sum += *row.Value
					}
				}
				if sum != *point.Value {
					t.Fatalf("%s/%s: evidence %v, mark %v", chart.Kind, point.Key, sum, *point.Value)
				}
			} else if len(evidence.Rows) != 5 {
				t.Fatalf("wrong percentile cohort: %+v", evidence)
			}
		}
	}
	selection.PipelineId = nil
	selection.Metrics = []crmcontracts.ReportingMetricID{"meetings_held", "accepted_opportunities"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"sdr_outcomes", "target_progress"}
	sdr, err := f.service.Evaluate(f.human, selection)
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range sdr.Metrics {
		expected := float64(12)
		if metric.Id == "accepted_opportunities" {
			expected = 6
		}
		if metric.Value == nil || *metric.Value != expected {
			t.Fatalf("SDR metric: %+v", metric)
		}
	}
}

func TestReportingPresetsAnswerEveryMetricTheyAskOnAPopulatedFixture(t *testing.T) {
	t.Parallel()
	f := reportingBusiness(t)
	actor, ok := principal.Actor(f.human)
	if !ok {
		t.Fatal("fixture has no human reader")
	}
	actor.Permissions.Objects["relationship"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	qualifierContext := principal.WithActor(f.human, actor)
	qualifier := contacts.NewStore(InstallationDB(f.env.Pool)).SetClock(func() time.Time { return f.at.Add(-time.Hour) }).WithDealOpener(leadDealOpener{deals: f.store})
	if _, err := qualifier.QualifyLead(qualifierContext, f.inboundLeads[0], contacts.PromoteLeadInput{Trigger: "human_qualify", Deal: &contacts.QualifyDealInput{PipelineID: &f.env.pipeline, StageID: new(f.env.stages[20])}}); err != nil {
		t.Fatal(err)
	}
	catalog, err := f.service.Catalog(f.human)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Presets) != len(reportingPresets) {
		t.Fatalf("the fixture reader is offered %d presets, want %d", len(catalog.Presets), len(reportingPresets))
	}
	for _, preset := range catalog.Presets {
		selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: preset.Metrics, Blocks: preset.Blocks}
		if preset.Id == crmcontracts.ReportingPresetIdSales {
			selection.PipelineId = ptrUUID(f.env.pipeline)
		}
		result, err := f.service.Evaluate(f.human, selection)
		if err != nil {
			t.Fatalf("%s preset: %v", preset.Id, err)
		}
		if len(result.Metrics) != len(preset.Metrics) {
			t.Fatalf("%s preset answered %d metrics, asked %d", preset.Id, len(result.Metrics), len(preset.Metrics))
		}
		for _, metric := range result.Metrics {
			if metric.Value == nil || (metric.Coverage.Status != crmcontracts.ReportingStatusOk && metric.Coverage.Status != crmcontracts.ReportingStatusPartial) {
				t.Errorf("%s preset: %s answered no figure, status %s", preset.Id, metric.Id, metric.Coverage.Status)
			}
		}
		for _, block := range preset.Blocks {
			if !slices.ContainsFunc(result.Charts, func(chart crmcontracts.ReportingChart) bool { return chart.Kind == block }) {
				t.Errorf("%s preset drew no %s chart", preset.Id, block)
			}
		}
	}
}

func TestSearchWonDealsUsesTheReportingCountAndTheMembersScope(t *testing.T) {
	t.Parallel()
	f := reportingBusiness(t)
	selection := f.selection()
	selection.PipelineId = nil
	selection.Scope = crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(f.env.Rep1)}
	selection.Metrics = []crmcontracts.ReportingMetricID{reportingBookingsWon}
	selection.Blocks = []crmcontracts.ReportingBlockKind{}
	reader := reportingRepSeat(f.env)
	arguments, err := json.Marshal(selection)
	if err != nil {
		t.Fatal(err)
	}
	plan := searchplan.Plan{Engine: "reporting", Arguments: string(arguments), Count: true}
	reply, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	lane := &assessmentLane{}
	server := Server{
		analyticsQueryHandlers:   newAnalyticsQueryHandlers(InstallationDB(f.env.Pool), 0, nil),
		analyticsContextHandlers: newAnalyticsContextHandlers(InstallationDB(f.env.Pool), func() time.Time { return f.at }),
		filterProposalHandlers:   filterProposalHandlers{pool: f.env.Pool, lane: lane, now: func() time.Time { return f.at }},
	}
	server.reportMetrics = f.service
	const question = "How many deals did I win this month?"
	vocabulary, err := server.searchDataVocabulary(reader)
	if err != nil {
		t.Fatal(err)
	}
	lane.answer = func(req model.Request) string {
		return promptref.Rewrite(string(reply), searchplan.Refs(question, vocabulary))
	}
	answer, err := server.answerSearchData(reader, question)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := f.service.Evaluate(reader, selection)
	if err != nil {
		t.Fatal(err)
	}
	if answer.Evaluation == nil || !reflect.DeepEqual(answer.Evaluation.Metrics, baseline.Metrics) || answer.Source == nil || answer.Count == nil || !*answer.Count {
		t.Fatalf("search answer %+v differs from the reporting count %+v", answer, baseline.Metrics)
	}
	own := baseline.Metrics[0].Coverage.EligibleCount
	workspace := selection
	workspace.Scope = crmcontracts.ReportingScope{Kind: "workspace"}
	all, err := f.service.Evaluate(f.human, workspace)
	if err != nil {
		t.Fatal(err)
	}
	if own == nil || all.Metrics[0].Coverage.EligibleCount == nil || *own >= *all.Metrics[0].Coverage.EligibleCount {
		t.Fatalf("own=%v, workspace=%v; fixture must contain won deals outside the selected owner lens", own, all.Metrics[0].Coverage.EligibleCount)
	}

	actor, ok := principal.Actor(reader)
	if !ok {
		t.Fatal("reader has no actor")
	}
	denied := actor
	denied.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	delete(denied.Permissions.Objects, "deal")
	noDeals, deniedErr := server.executeSearchData(principal.WithActor(reader, denied), plan)
	if deniedErr != nil && !errors.Is(deniedErr, apperrors.ErrPermissionDenied) && !errors.Is(deniedErr, apperrors.ErrInvalidArgument) {
		t.Fatal(deniedErr)
	}
	if deniedErr == nil && noDeals.Outcome != "unsupported" {
		t.Fatalf("a caller without deal read received a count: %+v", noDeals)
	}
	actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
	masked, err := server.executeSearchData(principal.WithActor(reader, actor), plan)
	if err != nil {
		t.Fatal(err)
	}
	maskedBaseline, err := f.service.Evaluate(principal.WithActor(reader, actor), selection)
	if err != nil {
		t.Fatal(err)
	}
	amountPlan := plan
	amountPlan.Count = false
	hiddenAmount, err := server.executeSearchData(principal.WithActor(reader, actor), amountPlan)
	if err != nil || hiddenAmount.Outcome != "unsupported" {
		t.Fatalf("a masked metric must fall back: %+v, %v", hiddenAmount, err)
	}
	if maskedBaseline.Metrics[0].Coverage.EligibleCount == nil {
		if masked.Outcome != "unsupported" {
			t.Fatalf("an unavailable count must fall back: %+v", masked)
		}
		return
	}
	if masked.Evaluation == nil {
		t.Fatalf("an available count lost its reporting evaluation: %+v", masked)
	}
	if !reflect.DeepEqual(masked.Evaluation.Metrics, maskedBaseline.Metrics) || (masked.Evaluation.Metrics[0].Value != nil && *masked.Evaluation.Metrics[0].Value != 0) || !masked.Evaluation.Metrics[0].Coverage.Withheld {
		t.Fatalf("search must preserve the reporting field mask: %+v", masked.Evaluation.Metrics)
	}
}

func TestANameOnlyCSVContactLandsWithAChannelWarning(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := importAdmin(e)
	runs := migration.NewRunStore(e.DB())
	run, err := runs.Create(ctx, migration.CreateRunInput{Connector: "csv", SourceRef: "channels.csv", Source: "csv:channels"})
	if err != nil {
		t.Fatal(err)
	}
	blobs := blobstore.NewMemory()
	const csv = "Name\nName Only\n"
	if err := blobs.Put(ctx, "channels.csv", strings.NewReader(csv), int64(len(csv)), "text/csv"); err != nil {
		t.Fatal(err)
	}
	source := migration.NewCSVSource(blobs, "channels.csv", migration.ObjectContact, map[string]string{"Name": "full_name"}, "Name")
	mapping := &migration.RunMapping{Object: migration.ObjectContact, Fields: map[string]string{"Name": "full_name"}, SourceKey: "Name", SourceName: "channels"}
	bundle, err := newBundleWriters(ctx, e.DB(), run.ID, mapping, source)
	if err != nil {
		t.Fatal(err)
	}
	writers := bundle.parts[migration.ObjectContact]
	preview, err := predictPages(ctx, source, writers, migration.ObjectContact)
	if err != nil {
		t.Fatal(err)
	}
	if preview.created != 1 || len(preview.warnings) != 1 || preview.warnings[0].Code != migration.ReasonContactChannelMissing {
		t.Fatalf("preview must retain and flag the contact: %+v", preview)
	}
	commit, err := newBundleWriters(ctx, e.DB(), run.ID, mapping, source)
	if err != nil {
		t.Fatal(err)
	}
	report, err := migration.NewEngine(runs, commit).Run(ctx, run.ID, source)
	if err != nil {
		t.Fatal(err)
	}
	if report.Imported != 1 || len(report.Objects) != 1 || report.Objects[0].Created != 1 || len(report.Objects[0].Warnings) != 1 || report.Objects[0].Warnings[0].Code != migration.ReasonContactChannelMissing {
		t.Fatalf("import summary must count and flag the created contact: %+v", report)
	}
}

func TestSearchRecordsPreserveTheQueryToolsHydratedResults(t *testing.T) {
	t.Parallel()
	f := reportingBusiness(t)
	server := Server{toolRegistry: f.env.toolRegistry()}
	plan, err := searchplan.Admit(searchplan.Plan{Engine: "records", Arguments: `{"plan":{"version":"v1","target":"deal","where":[{"field":"status","op":"eq","value":"open"},{"field":"currency","op":"eq","value":"EUR"},{"field":"amount_minor","op":"gt","value":50000},{"field":"expected_close_date","op":"gte","value":"2026-09-01"},{"field":"expected_close_date","op":"lt","value":"2026-10-01"}]}}`}, "Which deals close this month over EUR 50k?", `{"context":{"base_currency":"EUR"}}`)
	if err != nil {
		t.Fatal(err)
	}
	answer, err := server.executeSearchData(f.human, plan)
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]bool{}
	for _, deal := range f.open {
		if deal.AmountMinor != nil && *deal.AmountMinor > 5000000 {
			expected[deal.Id.String()] = true
		}
	}
	if len(expected) == 0 || len(expected) == len(f.open) {
		t.Fatal("fixture must distinguish the amount threshold")
	}
	if answer.Records == nil || answer.Records.Notes == nil || len(answer.Records.Rows) != len(expected) || answer.Records.Coverage != "complete_exact" {
		t.Fatalf("lost query payload or filtering: %+v", answer.Records)
	}
	for _, row := range answer.Records.Rows {
		if !expected[row.Record.Id.String()] || row.Record.RecordType != "deal" || row.Record.Fields["name"] == nil {
			t.Fatalf("unexpected or unhydrated record: %+v", row.Record)
		}
	}
	if answer.Source == nil {
		t.Fatal("record answer names no source query")
	}
}

func TestSearchAnalyticsUsesTheExistingQueryAndAnswerLabels(t *testing.T) {
	t.Parallel()
	f := reportingBusiness(t)
	handler := newAnalyticsQueryHandlers(InstallationDB(f.env.Pool), analyticsquery.DefaultFloor, nil)
	server := Server{analyticsQueryHandlers: handler}
	const arguments = `{"entity":"open-deals-per-company","measures":[{"fn":"count","as":"deals"}],"scope_kind":"workspace"}`
	answer, err := server.executeSearchData(f.human, searchplan.Plan{Engine: "analytics", Arguments: arguments})
	if err != nil {
		t.Fatal(err)
	}
	var query crmcontracts.AnalyticsQuery
	if err := json.Unmarshal([]byte(arguments), &query); err != nil {
		t.Fatal(err)
	}
	baseline, err := f.env.askAnalytics(f.human, t, queryFromWire(query))
	if err != nil {
		t.Fatal(err)
	}
	wire := handler.labelledAnswer(f.human, queryFromWire(query), baseline)
	if answer.Analytics == nil || answer.AnalyticsQuery == nil || !reflect.DeepEqual(answer.Analytics.Rows, wire.Rows) || !reflect.DeepEqual(answer.Analytics.Columns, wire.Columns) {
		t.Fatalf("search lost the analytics result or labels: %+v", answer)
	}
	raw, err := json.Marshal(answer.Analytics.Rows)
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Deals int `json:"deals"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Deals != len(f.open) {
		t.Fatalf("count=%s, want %d open deals", raw, len(f.open))
	}
}
