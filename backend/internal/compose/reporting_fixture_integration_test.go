// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type reportingBusinessFixture struct {
	env     *forecastEnv
	at      time.Time
	service *reporting.Service
	store   *deals.Store
	human   context.Context
	writer  context.Context
	open    []crmcontracts.Deal
	target  crmcontracts.ReportingTarget
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
	f.store = deals.NewStore(InstallationDB(e.Pool), DealsInstallation()).WithClock(func() time.Time { return f.at })
	_, err := f.service.PublishFramework(f.human, 0, crmcontracts.ReportingFrameworkInput{Template: "sales", Reason: "Qualification begins at proposal", Qualification: []crmcontracts.ReportingQualification{{PipelineId: openapi_types.UUID(e.pipeline), StageIds: []openapi_types.UUID{openapi_types.UUID(e.stages[60])}}}, CaptureContexts: []crmcontracts.ReportingCaptureContext{{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, PipelineId: ptrUUID(e.pipeline)}}})
	if err != nil {
		t.Fatal(err)
	}
	f.closedDeals(t)
	f.openDeals(t)
	f.outcomes(t)
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
			input = deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](lost), LostReason: &reason}
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
	store := contacts.NewStore(InstallationDB(f.env.Pool)).WithClock(func() time.Time { return f.at })
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

func (f *reportingBusinessFixture) selection() crmcontracts.ReportingSelection {
	return crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, PipelineId: ptrUUID(f.env.pipeline), Period: "this_month", TargetBasis: "month", CloseWindow: "fiscal_quarter", Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate", "open_pipeline", "stage_age"}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution", "owner_attainment", "stage_age"}}
}

func TestReportingBusinessFixtureReconcilesEveryChartWithItsEvidence(t *testing.T) {
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
