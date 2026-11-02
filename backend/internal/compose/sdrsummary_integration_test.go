// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sdrFixture is an SDR (Rep1) handing prospects on through the real handoff
// writer, under a clock the test moves.
type sdrFixture struct {
	e *forecastEnv
	// ae is the account executive the SDR hands prospects to.
	ae       ids.UUID
	at       time.Time
	ctx      context.Context
	handoffs *contacts.Store
}

func newSdrFixture(t *testing.T, template crmcontracts.ReportingFrameworkInputTemplate) *sdrFixture {
	t.Helper()
	f := &sdrFixture{e: setupForecast(t), at: time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)}
	clock := func() time.Time { return f.at }
	ctx := reportingActor(f.e)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["lead"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	f.ctx = principal.WithActor(ctx, actor)
	f.handoffs = contacts.NewStore(InstallationDB(f.e.Pool)).WithClock(clock)
	f.ae = f.e.seedID(t, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'ae@example.test', 'Account Executive')`)
	framework := crmcontracts.ReportingFrameworkInput{Template: template, Reason: "SDR team setup", Qualification: []crmcontracts.ReportingQualification{}, CaptureContexts: []crmcontracts.ReportingCaptureContext{}}
	if _, err := newReportingService(f.e.Pool, clock).PublishFramework(f.ctx, 0, framework); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *sdrFixture) submit(t *testing.T, name string, to *ids.UUID) ids.UUID {
	t.Helper()
	contact := f.e.seedID(t, `INSERT INTO contact(id,full_name,source,captured_by) VALUES($1,$2,'manual','system:fixture')`, name)
	id, err := f.handoffs.SubmitHandoff(f.ctx, contacts.NewSDRHandoff{ContactID: &contact, AssignedTo: to})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (f *sdrFixture) summary(ctx context.Context, t *testing.T, owner ids.UUID) crmcontracts.SdrSummary {
	t.Helper()
	kind := crmcontracts.GetSdrSummaryParamsScopeKindOwner
	id := openapi_types.UUID(owner)
	var out crmcontracts.SdrSummary
	body := sdrServe(ctx, t, f.e, func(h sdrSummaryHandlers, w http.ResponseWriter, r *http.Request) {
		h.GetSdrSummary(w, r, crmcontracts.GetSdrSummaryParams{ScopeKind: &kind, ScopeId: &id})
	}, f.at)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (f *sdrFixture) page(ctx context.Context, t *testing.T, owner ids.UUID, cursor *string, limit int) crmcontracts.SdrWaitingHandoffPage {
	t.Helper()
	kind := crmcontracts.ListSdrWaitingHandoffsParamsScopeKindOwner
	id := openapi_types.UUID(owner)
	var out crmcontracts.SdrWaitingHandoffPage
	body := sdrServe(ctx, t, f.e, func(h sdrSummaryHandlers, w http.ResponseWriter, r *http.Request) {
		h.ListSdrWaitingHandoffs(w, r, crmcontracts.ListSdrWaitingHandoffsParams{ScopeKind: &kind, ScopeId: &id, Cursor: cursor, Limit: &limit})
	}, f.at)
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func sdrServe(ctx context.Context, t *testing.T, e *forecastEnv, call func(sdrSummaryHandlers, http.ResponseWriter, *http.Request), at time.Time) []byte {
	t.Helper()
	clock := func() time.Time { return at }
	h := newSdrSummaryHandlers(InstallationDB(e.Pool), clock).withTargets(newReportingService(e.Pool, clock))
	w := httptest.NewRecorder()
	call(h, w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

func TestSdrSummaryAgesTheCurrentSubmissionAndDisclosesABlockedRecipient(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr)
	e := f.e
	returned := f.submit(t, "Returned prospect", &f.ae)
	f.at = f.at.Add(time.Hour)
	blocked := f.submit(t, "Blocked prospect", &e.Rep3)
	f.at = f.at.Add(time.Hour)
	queued := f.submit(t, "Queued prospect", nil)
	var reason ids.UUID
	if err := e.Pool.QueryRow(context.Background(), `SELECT id FROM sdr_handoff_reason WHERE applies_to='recycled' AND active ORDER BY sort_order LIMIT 1`).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	if err := f.handoffs.DecideHandoff(f.ctx, returned, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: &reason}); err != nil {
		t.Fatal(err)
	}
	if got := f.summary(f.ctx, t, e.Rep1).Handoffs; got == nil || got.Waiting != 2 || got.Returned != 1 {
		t.Fatalf("a recycled handoff waits on its submitter, not on a recipient: %+v", got)
	}
	f.at = f.at.Add(2 * time.Hour)
	resubmittedAt := f.at
	if err := f.handoffs.ResubmitHandoff(f.ctx, returned, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Pool.Exec(context.Background(), `UPDATE app_user SET status='suspended' WHERE id=$1`, e.Rep3); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)

	got := f.summary(f.ctx, t, e.Rep1)
	totals := got.Handoffs
	if got.Workflow != crmcontracts.SdrSummaryWorkflowHandoff || totals == nil {
		t.Fatalf("an SDR framework reads handoffs: %+v", got)
	}
	if totals.Waiting != 3 || totals.Unassigned != 1 || totals.Blocked != 1 || totals.Returned != 0 {
		t.Fatalf("totals = %+v, want 3 waiting, 1 unassigned, 1 blocked, none returned", totals)
	}
	if totals.OldestWaitingSince == nil || !totals.OldestWaitingSince.Equal(time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("the oldest wait is the blocked handoff's, since the returned one started again: %v", totals.OldestWaitingSince)
	}
	if other := f.summary(f.ctx, t, f.ae).Handoffs; other == nil || other.Waiting != 0 {
		t.Fatalf("a handoff is credited to its submitter, not its recipient: %+v", other)
	}

	first := f.page(f.ctx, t, e.Rep1, nil, 2)
	if len(first.Rows) != 2 || first.NextCursor == nil || ids.UUID(first.Rows[0].Id) != blocked || ids.UUID(first.Rows[1].Id) != queued {
		t.Fatalf("page one is the two oldest waits, with a cursor: %+v", first)
	}
	if r := first.Rows[0].Recipient; r == nil || r.Active || ids.UUID(r.Id) != e.Rep3 {
		t.Fatalf("a suspended recipient is disclosed as inactive: %+v", r)
	}
	if first.Rows[1].Recipient != nil || first.Rows[1].Subject == nil || first.Rows[1].Subject.Label != "Queued prospect" {
		t.Fatalf("a queued handoff names its prospect and no recipient: %+v", first.Rows[1])
	}
	second := f.page(f.ctx, t, e.Rep1, first.NextCursor, 2)
	if len(second.Rows) != 1 || second.NextCursor != nil || ids.UUID(second.Rows[0].Id) != returned {
		t.Fatalf("page two holds the resubmitted handoff alone: %+v", second)
	}
	if row := second.Rows[0]; !row.Resubmitted || !row.WaitingSince.Equal(resubmittedAt) {
		t.Fatalf("a resubmitted handoff waits from its resubmission: %+v", row)
	}
}

func TestSdrSummaryNeverAgesAHandoffPastItsAsOf(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr)
	e := f.e
	f.submit(t, "Earlier prospect", &f.ae)
	f.at = f.at.Add(2 * time.Hour)
	f.submit(t, "Later prospect", &f.ae)
	// The answer is stamped between the two submissions, as a read that
	// sampled its clock before the second one committed would be.
	f.at = f.at.Add(-time.Hour)
	if totals := f.summary(f.ctx, t, e.Rep1).Handoffs; totals == nil || totals.Waiting != 1 {
		t.Fatalf("a handoff submitted after as_of is not yet waiting: %+v", totals)
	}
	page := f.page(f.ctx, t, e.Rep1, nil, 10)
	if len(page.Rows) != 1 {
		t.Fatalf("the page holds only the handoff waiting at as_of: %+v", page.Rows)
	}
	for _, row := range page.Rows {
		if row.WaitingSince.After(page.AsOf) {
			t.Fatalf("a wait starts after the answer's as_of, so its age is negative: %+v at %v", row, page.AsOf)
		}
	}
}

func TestSdrSummaryAcceptanceAfterResubmissionCountsOnce(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr)
	e := f.e
	handoff := f.submit(t, "Accepted prospect", &f.ae)
	var reason ids.UUID
	if err := e.Pool.QueryRow(context.Background(), `SELECT id FROM sdr_handoff_reason WHERE applies_to='recycled' AND active ORDER BY sort_order LIMIT 1`).Scan(&reason); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	if err := f.handoffs.DecideHandoff(f.ctx, handoff, contacts.HandoffDecision{Status: contacts.HandoffRecycled, ReasonID: &reason}); err != nil {
		t.Fatal(err)
	}
	f.at = f.at.Add(time.Hour)
	if err := f.handoffs.ResubmitHandoff(f.ctx, handoff, nil); err != nil {
		t.Fatal(err)
	}
	deal, _, _ := reportingWrittenClose(t, e)
	dealID := ids.UUID(deal.Id)
	f.at = f.at.Add(time.Hour)
	if err := f.handoffs.DecideHandoff(f.ctx, handoff, contacts.HandoffDecision{Status: contacts.HandoffAccepted, DealID: &dealID}); err != nil {
		t.Fatal(err)
	}
	if totals := f.summary(f.ctx, t, e.Rep1).Handoffs; totals == nil || totals.Waiting != 0 || totals.Returned != 0 {
		t.Fatalf("an accepted handoff waits on nobody: %+v", totals)
	}
	f.at = f.at.Add(time.Minute)
	service := newReportingService(e.Pool, func() time.Time { return f.at })
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"accepted_opportunities"}, Blocks: []crmcontracts.ReportingBlockKind{"metric_reading"}}
	result, err := service.Evaluate(f.ctx, selection)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Metrics) != 1 || result.Metrics[0].Value == nil || *result.Metrics[0].Value != 1 {
		t.Fatalf("one acceptance after a resubmission is one accepted handoff: %+v", result.Metrics)
	}
}

func TestSdrWaitingHandoffWithheldProspectIsRestricted(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr)
	e := f.e
	f.submit(t, "Private prospect", &f.ae)
	if _, err := e.Pool.Exec(context.Background(), `UPDATE contact SET visibility='owner', owner_id=$1 WHERE full_name='Private prospect'`, e.Rep3); err != nil {
		t.Fatal(err)
	}
	own := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeOwn)
	page := f.page(own, t, e.Rep1, nil, 10)
	if len(page.Rows) != 1 {
		t.Fatalf("the seat's own handoff is listed: %+v", page)
	}
	if row := page.Rows[0]; !row.Restricted || row.Subject != nil {
		t.Fatalf("a prospect outside the seat's row scope is neither named nor linked: %+v", row)
	}
	if totals := f.summary(own, t, e.Rep1).Handoffs; totals == nil || totals.Waiting != 1 {
		t.Fatalf("the restricted handoff still counts as the seat's own work: %+v", totals)
	}
	kind := crmcontracts.GetSdrSummaryParamsScopeKindOwner
	colleague := openapi_types.UUID(e.Rep3)
	w := httptest.NewRecorder()
	newSdrSummaryHandlers(InstallationDB(e.Pool), func() time.Time { return f.at }).GetSdrSummary(w, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(own), crmcontracts.GetSdrSummaryParams{ScopeKind: &kind, ScopeId: &colleague})
	if w.Code != http.StatusNotFound {
		t.Fatalf("a rep asking for a colleague's handoffs gets %d, want the 404 every analytics read gives", w.Code)
	}
}

func TestSdrWaitingHandoffWithAMaskedLeadCompanyIsRestricted(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSdr)
	e := f.e
	lead := e.seedID(t, `INSERT INTO lead(id,full_name,company_name,source,captured_by) VALUES($1,'','Masked Company Ltd','inbound','system:fixture')`)
	if _, err := f.handoffs.SubmitHandoff(f.ctx, contacts.NewSDRHandoff{LeadID: &lead, AssignedTo: &f.ae}); err != nil {
		t.Fatal(err)
	}
	// A mask binds a bounded seat; an all-rows seat is not masked at all.
	reader := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeOwn)
	actor, _ := principal.Actor(reader)
	actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "lead", Field: "company_name", Condition: principal.MaskAlways}}
	reader = principal.WithActor(reader, actor)
	page := f.page(reader, t, e.Rep1, nil, 10)
	if len(page.Rows) != 1 {
		t.Fatalf("the seat's own handoff is listed: %+v", page)
	}
	if row := page.Rows[0]; !row.Restricted || row.Subject != nil {
		t.Fatalf("a lead labelled by a masked company name must not show it: %+v", row.Subject)
	}
}

func TestSdrSummaryWithoutAHandoffWorkflowReadsNoHandoffs(t *testing.T) {
	f := newSdrFixture(t, crmcontracts.ReportingFrameworkInputTemplateSales)
	e := f.e
	f.submit(t, "Waiting anyway", &f.ae)
	contact := e.seedID(t, `INSERT INTO contact(id,full_name,source,captured_by) VALUES($1,'Booked buyer','manual','system:fixture')`)
	for _, meeting := range []struct {
		status string
		offset time.Duration
	}{{"booked", 48 * time.Hour}, {"booked", -48 * time.Hour}, {"canceled", 72 * time.Hour}} {
		activity := e.seedID(t, `INSERT INTO activity(id,kind,subject,meeting_status,occurred_at,host_user_id,source,captured_by) VALUES($1,'meeting','Discovery',$2,$3,$4,'manual',$5)`,
			meeting.status, f.at.Add(meeting.offset), e.Rep1, "human:"+e.Rep1.String())
		if _, err := e.Pool.Exec(context.Background(), `INSERT INTO activity_link(activity_id,entity_type,contact_id) VALUES($1,'contact',$2)`, activity, contact); err != nil {
			t.Fatal(err)
		}
	}
	got := f.summary(f.ctx, t, e.Rep1)
	if got.Workflow != crmcontracts.SdrSummaryWorkflowQualification || got.Handoffs != nil {
		t.Fatalf("a sales framework has no handoff workflow, whatever rows exist: %+v", got)
	}
	if got.UpcomingMeetings == nil || *got.UpcomingMeetings != 1 {
		t.Fatalf("one booked meeting is still ahead: %v", got.UpcomingMeetings)
	}
	masked := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeOwn)
	actor, _ := principal.Actor(masked)
	actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "activity", Field: "meeting_status", Condition: principal.MaskAlways}}
	if hidden := f.summary(principal.WithActor(masked, actor), t, e.Rep1).UpcomingMeetings; hidden == nil || *hidden != 0 {
		t.Fatalf("a seat with the meeting status masked counts no meeting by it: %v", hidden)
	}
}

func TestLeadsAwaitingResponseCountsTheUnansweredArrivalCohort(t *testing.T) {
	e := setupForecast(t)
	now := clocktest.Now(t)
	arrived, replied := now.AddDate(0, 0, -2), now.AddDate(0, 0, -1)
	inbound := func(lead ids.UUID) {
		mail := e.seedID(t, `INSERT INTO activity(id,kind,subject,direction,occurred_at,source,captured_by) VALUES($1,'email','Question','inbound',$2,'manual','system:fixture')`, arrived)
		if _, err := e.Pool.Exec(context.Background(), `INSERT INTO activity_link(activity_id,entity_type,lead_id) VALUES($1,'lead',$2)`, mail, lead); err != nil {
			t.Fatal(err)
		}
	}
	waiting := e.seedID(t, `INSERT INTO lead(id,full_name,source,captured_by,created_at) VALUES($1,'Still waiting','inbound','human:x',$2)`, arrived)
	inbound(waiting)
	answered := e.seedID(t, `INSERT INTO lead(id,full_name,source,captured_by,created_at,first_response_at) VALUES($1,'Answered','inbound','human:x',$2,$3)`, arrived, replied)
	inbound(answered)
	imported := e.seedID(t, `INSERT INTO lead(id,full_name,source,captured_by,source_system,created_at) VALUES($1,'Imported','inbound','human:x','mirror:hubspot',$2)`, arrived)
	inbound(imported)
	e.seedID(t, `INSERT INTO lead(id,full_name,source,captured_by,created_at) VALUES($1,'Never wrote','inbound','human:x',$2)`, arrived)

	service := newReportingService(e.Pool, func() time.Time { return now })
	admin := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll)
	interval := crmcontracts.ReportingWindow{StartAt: now.AddDate(0, 0, -3), EndAt: now.AddDate(0, 0, 1)}
	metrics := []crmcontracts.ReportingMetricID{"lead_first_response", "leads_awaiting_response"}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", Metrics: metrics, Blocks: []crmcontracts.ReportingBlockKind{"metric_reading"}}
	result, err := service.Evaluate(admin, selection)
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionsValue(t, result, "leads_awaiting_response"); got != 1 {
		t.Fatalf("one lead wrote in and has had no reply, read %v", got)
	}
	answeredCohort := sectionsReading(t, result, "lead_first_response")
	if answeredCohort.Coverage.EligibleCount == nil || *answeredCohort.Coverage.EligibleCount != 1 {
		t.Fatalf("the response time counts the one answered lead, never the waiting one: %+v", answeredCohort.Coverage)
	}
	rows := sectionsEvidence(admin, t, service, result, "leads_awaiting_response", "interval", "")
	if len(rows) != 1 || rows[0].SourceId == nil || ids.UUID(*rows[0].SourceId) != waiting {
		t.Fatalf("the evidence is the waiting lead: %+v", rows)
	}
}

func TestLeadEvidenceLabelHidesAMaskedCompanyName(t *testing.T) {
	e := setupForecast(t)
	now := clocktest.Now(t)
	e.seedID(t, `INSERT INTO lead(id,full_name,company_name,source,captured_by,owner_id,created_at) VALUES($1,'','Masked Company Ltd','inbound','human:x',$2,$3)`, e.Rep1, now.AddDate(0, 0, -1))
	reader := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeOwn)
	actor, _ := principal.Actor(reader)
	actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "lead", Field: "company_name", Condition: principal.MaskAlways}}
	reader = principal.WithActor(reader, actor)
	service := newReportingService(e.Pool, func() time.Time { return now })
	result := sectionsEvaluation(reader, t, service, now.AddDate(0, 0, -2), crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}, "leads_created")
	rows := sectionsEvidence(reader, t, service, result, "leads_created", "interval", "")
	if len(rows) != 1 || rows[0].Label == "Masked Company Ltd" {
		t.Fatalf("a lead labelled by a masked company name must not show it: %+v", rows)
	}
}
