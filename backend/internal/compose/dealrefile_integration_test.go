// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type dealRefileFixture struct {
	pipeline                                   ids.PipelineID
	stage                                      ids.StageID
	closed                                     ids.StageID
	o                                          *owedEnv
	contact, company, deal, mail, meeting, old ids.UUID
}

func newDealRefileFixture(t *testing.T, extra int) dealRefileFixture {
	t.Helper()
	o := setupOwed(t)
	o.e.GrantRole(t, o.owner, "rep")
	o.e.GrantRole(t, o.e.AdminUser, "admin")
	o.now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	mail := o.customerWrites(t, "pat@refile.example", "Please arrange a demo", o.now.Add(-time.Hour))
	old := o.customerWrites(t, "pat@refile.example", "An older discussion", o.now.Add(-31*24*time.Hour))
	meeting := captureRefileMeeting(t, o, "refile-demo")
	for range extra {
		captureRefileMeeting(t, o, ids.NewV7().String())
	}
	contact := o.contact(t, "Pat Refile", "pat@refile.example")
	company := o.e.SeedCompany(t, "Refile customer", nil)
	member, employer := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	if _, err := o.e.Contacts.CreateRelationship(o.e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &member, CompanyID: &employer, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	pipeline, open, closed := integration.DealFixture(t, o.e)
	deal, err := o.e.Deals.CreateDeal(o.e.Admin(), deals.CreateDealInput{Name: "Refile rollout", PipelineID: pipeline, StageID: open, CompanyID: &employer, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return dealRefileFixture{pipeline: pipeline, stage: open, closed: closed, o: o, contact: contact, company: company, deal: ids.UUID(deal.Id), mail: mail, meeting: meeting, old: old}
}

func (f dealRefileFixture) enqueue(t *testing.T, eventType string, entity ids.UUID) CapturedDealRefileArgs {
	t.Helper()
	e := f.o.e
	var envelope events.Envelope
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(t.Context(), `SELECT envelope FROM event_outbox WHERE envelope->>'type'=$1 AND envelope->'entity'->>'id'=$2 ORDER BY seq DESC LIMIT 1`, eventType, entity.String()).Scan(&envelope)
	}); err != nil {
		t.Fatal(err)
	}
	return f.enqueueEvent(t, envelope)
}

func (f dealRefileFixture) enqueueEvent(t *testing.T, envelope events.Envelope) CapturedDealRefileArgs {
	t.Helper()
	e := f.o.e
	runner, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	gen := NewDealRefileGen(e.Pool, runner, func() time.Time { return f.o.now })
	for range 2 {
		if err := gen.HandleEvent(context.Background(), envelope); err != nil {
			t.Fatal(err)
		}
	}
	var args CapturedDealRefileArgs
	var raw []byte
	if err := e.Pool.QueryRow(t.Context(), `SELECT args FROM river_job WHERE kind='captured_deal_refile' AND args->>'event_id'=$1`, envelope.EventID.String()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='captured_deal_refile' AND args->>'event_id'=$1`, envelope.EventID.String()); got != 1 {
		t.Fatalf("replay queued %d jobs", got)
	}
	return args
}

func (f dealRefileFixture) run(t *testing.T, args CapturedDealRefileArgs) int {
	t.Helper()
	spec, ok := jobs.SpecFor(args.Kind())
	if !ok {
		t.Fatal("missing job declaration")
	}
	worker := rivertest.NewWorker(t, riverpgxv5.New(f.o.e.Pool), &river.Config{}, jobs.Govern(newCapturedDealRefileWorker(f.o.e.Pool), spec))
	var row *rivertype.JobRow
	for pass := range 10 {
		tx, err := f.o.e.Pool.Begin(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		var result *rivertest.WorkResult
		if row == nil {
			result, err = worker.Work(t.Context(), t, tx, args, nil)
		} else {
			result, err = worker.WorkJob(t.Context(), t, tx, row)
		}
		if err != nil {
			if rollback := tx.Rollback(t.Context()); rollback != nil {
				t.Error(rollback)
			}
			t.Fatal(err)
		}
		if err := tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		if result.EventKind == river.EventKindJobCompleted {
			return pass + 1
		}
		if result.EventKind != river.EventKindJobSnoozed {
			t.Fatalf("worker ended as %s", result.EventKind)
		}
		row = result.Job
	}
	t.Fatal("refile did not drain its finite pass")
	return 0
}

func TestAContactEventReconsidersCapturedMailAndMeetingsAfterCohortResolution(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	integration.ApplyRiverSchema(t)
	args := f.enqueue(t, "contact.created", f.contact)
	f.run(t, args)
	e := f.o.e
	for _, activity := range []ids.UUID{f.mail, f.meeting} {
		if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, activity, f.deal); got != 1 {
			t.Fatalf("activity %s has %d deal links", activity, got)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='deal'`, f.old); got != 0 {
		t.Fatal("old activity was refiled beyond the waiting horizon")
	}
	var last *time.Time
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT last_activity_at FROM deal WHERE id=$1`, f.deal).Scan(&last)
	}); err != nil {
		t.Fatal(err)
	}
	if last == nil || !last.Equal(f.o.now.Add(-time.Hour)) {
		t.Fatalf("last activity = %v; future meeting must not move it", last)
	}
	before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	f.run(t, args)
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log`); got != before {
		t.Fatal("replaying the pass added audit rows")
	}
}

func TestDealReconsiderationContinuesBeyondItsFirstPage(t *testing.T) {
	f := newDealRefileFixture(t, 201)
	integration.ApplyRiverSchema(t)
	passes := f.run(t, f.enqueue(t, "contact.created", f.contact))
	if passes < 3 {
		t.Fatalf("only %d passes for cohort resolution and more than one page", passes)
	}
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE deal_id=$1`, f.deal); got != 203 {
		t.Fatalf("filed %d activities, want 203", got)
	}
}

func TestAmbiguousDealReconsiderationWritesNoLinkOrQuestion(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	integration.ApplyRiverSchema(t)
	company := ids.From[ids.CompanyKind](f.company)
	if _, err := f.o.e.Deals.CreateDeal(f.o.e.Admin(), deals.CreateDealInput{Name: "Another rollout", PipelineID: f.pipeline, StageID: f.stage, CompanyID: &company, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	before := f.o.e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`)
	f.run(t, f.enqueue(t, "contact.created", f.contact))
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=ANY($1) AND entity_type='deal'`, []ids.UUID{f.mail, f.meeting}); got != 0 {
		t.Fatalf("ambiguous activities have %d deal links", got)
	}
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != before {
		t.Fatal("reconsideration raised a question")
	}
}

func captureRefileMeeting(t *testing.T, o *owedEnv, event string) ids.UUID {
	t.Helper()
	owner := seatAddress(t, o.e, o.e.AdminUser)
	ref, err := newCaptureSink(o.e.Pool, CaptureConfig{}).Upsert(o.e.ConnectorOwnerCtx(o.e.AdminUser, "gcal"), connector.NormalizedRecord{
		EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: "gcal", SourceID: event},
		Fields:       capture.ActivityFields{Kind: "meeting", Subject: "Customer demo", OccurredAt: time.Date(2100, 1, 4, 9, 0, 0, 0, time.UTC)},
		Participants: []connector.MessageParticipant{{Email: "pat@refile.example", Role: "attendee"}, {Email: owner, Role: "organizer"}},
		Addresses:    []string{owner, "pat@refile.example"}, Source: "gcal:" + event, CapturedBy: "connector:gcal",
	}.WithProviderAttestedParticipants(true))
	if err != nil {
		t.Fatal(err)
	}
	return ref.ID
}

func TestDealReconsiderationKeepsAHumanFiling(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	integration.ApplyRiverSchema(t)
	company := ids.From[ids.CompanyKind](f.o.e.SeedCompany(t, "Other customer", nil))
	other, err := f.o.e.Deals.CreateDeal(f.o.e.Admin(), deals.CreateDealInput{Name: "Chosen by a colleague", PipelineID: f.pipeline, StageID: f.stage, CompanyID: &company, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	otherID := ids.UUID(other.Id)
	if _, err := f.o.e.Activities.RelinkActivity(f.o.e.Admin(), ids.From[ids.ActivityKind](f.mail), activities.RelinkActivityInput{EntityType: "deal", EntityID: otherID, ReplaceExistingOfType: true}); err != nil {
		t.Fatal(err)
	}
	f.run(t, f.enqueue(t, "contact.created", f.contact))
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, f.mail, otherID); got != 1 {
		t.Fatal("human deal link was moved")
	}
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, f.mail, f.deal); got != 0 {
		t.Fatal("automatic filing added another deal")
	}
}

func TestTheRecoverySweepQueuesTheSameFiniteReconsideration(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	integration.ApplyRiverSchema(t)
	var envelope events.Envelope
	if err := f.o.e.DB().Tx(f.o.e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(f.o.e.Admin(), `SELECT envelope FROM event_outbox WHERE envelope->>'type'='contact.created' AND envelope->'entity'->>'id'=$1 ORDER BY seq DESC LIMIT 1`, f.contact.String()).Scan(&envelope)
	}); err != nil {
		t.Fatal(err)
	}
	cohorts := NewCohortPromoteGen(f.o.e.Pool, contacts.NewStore(f.o.e.DB()), slog.New(slog.DiscardHandler))
	if err := cohorts.HandleEvent(context.Background(), envelope); err != nil {
		t.Fatal(err)
	}
	spec, ok := jobs.SpecFor(CapturedDealRefileSweepArgs{}.Kind())
	if !ok {
		t.Fatal("missing recovery declaration")
	}
	worker := rivertest.NewWorker(t, riverpgxv5.New(f.o.e.Pool), &river.Config{}, jobs.Govern(&capturedDealRefileSweepWorker{pool: f.o.e.Pool, now: func() time.Time { return f.o.now }}, spec))
	tx, err := f.o.e.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, err := worker.Work(t.Context(), t, tx, CapturedDealRefileSweepArgs{}, nil)
	if err != nil {
		if rollback := tx.Rollback(t.Context()); rollback != nil {
			t.Error(rollback)
		}
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if result.EventKind != river.EventKindJobCompleted {
		t.Fatalf("recovery ended as %s", result.EventKind)
	}
	var raw []byte
	if err := f.o.e.Pool.QueryRow(t.Context(), `SELECT args FROM river_job WHERE kind='captured_deal_refile'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var args CapturedDealRefileArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatal(err)
	}
	if !args.Scope.All || args.Correlation.IsZero() || !args.AsOf.Equal(f.o.now) {
		t.Fatalf("recovery args=%+v", args)
	}
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='captured_deal_refile'`); got != 1 {
		t.Fatalf("recovery duplicated %d jobs", got)
	}
	f.run(t, args)
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=ANY($1) AND deal_id=$2`, []ids.UUID{f.mail, f.meeting}, f.deal); got != 2 {
		t.Fatalf("recovery filed %d activities", got)
	}
}

func TestLeadPromotionReconsidersMailAndMeetingsCapturedBeforeTheDeal(t *testing.T) {
	o := setupOwed(t)
	o.e.GrantRole(t, o.owner, "rep")
	o.e.GrantRole(t, o.e.AdminUser, "admin")
	o.now = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	e := o.e
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Pat Refile"), Email: new("pat@refile.example"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	mail := o.customerWrites(t, "pat@refile.example", "Please arrange a demo", o.now.Add(-time.Hour))
	meeting := captureRefileMeeting(t, o, "lead-refile-demo")
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: "Refile customer", Source: "manual", Domains: []contacts.CompanyDomainInput{{Domain: "refile.example", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	companyID := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Lead rollout", PipelineID: pipeline, StageID: stage, CompanyID: &companyID, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	contact, _, err := e.Contacts.PromoteLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(lead.Id)), contacts.PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatal(err)
	}
	integration.ApplyRiverSchema(t)
	f := dealRefileFixture{o: o, contact: ids.UUID(contact.Id), company: companyID.UUID, deal: ids.UUID(deal.Id), mail: mail, meeting: meeting}
	f.run(t, f.enqueue(t, "lead.promoted", ids.UUID(lead.Id)))
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=ANY($1) AND deal_id=$2`, []ids.UUID{mail, meeting}, f.deal); got != 2 {
		t.Fatalf("promotion filed %d activities", got)
	}
}

func TestADealCreatedEventFilesPreviouslyCapturedResolvedActivities(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	integration.ApplyRiverSchema(t)
	worker := newCapturedDealRefileWorker(f.o.e.Pool)
	if _, err := worker.repairCohorts(principal.SystemActing(f.o.e.Admin(), dealRefileActor), []ids.UUID{f.contact}); err != nil {
		t.Fatal(err)
	}
	if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='deal'`, f.mail); got != 0 {
		t.Fatal("fixture already filed the mail")
	}
	args := f.enqueue(t, "deal.created", f.deal)
	if !slices.Contains(args.Scope.Companies, f.company) {
		t.Fatal("deal creation lost its company")
	}
	f.run(t, args)
	for _, activity := range []ids.UUID{f.mail, f.meeting} {
		if got := f.o.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, activity, f.deal); got != 1 {
			t.Fatalf("activity %s has %d deal links", activity, got)
		}
	}
}

func TestALateCompanyChangeRetainsBothAuditedCompaniesAndTheCurrentOne(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	integration.ApplyRiverSchema(t)
	worker := newCapturedDealRefileWorker(e.Pool)
	if _, err := worker.repairCohorts(principal.SystemActing(e.Admin(), dealRefileActor), []ids.UUID{f.contact}); err != nil {
		t.Fatal(err)
	}
	b := ids.From[ids.CompanyKind](e.SeedCompany(t, "Second employer", nil))
	c := ids.From[ids.CompanyKind](e.SeedCompany(t, "Third employer", nil))
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](f.deal), deals.UpdateDealInput{CompanyID: &b}); err != nil {
		t.Fatal(err)
	}
	var first events.Envelope
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT envelope FROM event_outbox WHERE envelope->>'type'='deal.updated' AND envelope->'entity'->>'id'=$1 ORDER BY seq DESC LIMIT 1`, f.deal.String()).Scan(&first)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](f.deal), deals.UpdateDealInput{CompanyID: &c}); err != nil {
		t.Fatal(err)
	}
	a := ids.From[ids.CompanyKind](f.company)
	available, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Available original account deal", PipelineID: f.pipeline, StageID: f.stage, CompanyID: &a, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	args := f.enqueueEvent(t, first)
	for _, company := range []ids.UUID{a.UUID, b.UUID, c.UUID} {
		if !slices.Contains(args.Scope.Companies, company) {
			t.Fatalf("late edit lost company %s: %+v", company, args.Scope)
		}
	}
	f.run(t, args)
	for _, activity := range []ids.UUID{f.mail, f.meeting} {
		if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, activity, available.Id); got != 1 {
			t.Fatalf("activity %s has %d current eligible deal links", activity, got)
		}
	}
}

func TestReopeningADealReconsidersActivitiesAClosedDealCouldNotTake(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	integration.ApplyRiverSchema(t)
	store := e.Deals.WithClock(func() time.Time { return f.o.now })
	deal := ids.From[ids.DealKind](f.deal)
	if _, err := store.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{ToStageID: f.closed, WonWithoutContractReason: new("verbal")}); err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		var closed events.Envelope
		if err := tx.QueryRow(e.Admin(), `SELECT envelope FROM event_outbox WHERE envelope->>'type'=$1 AND envelope->'entity'->>'id'=$2 ORDER BY seq DESC LIMIT 1`, deals.EventDealStageChanged, f.deal.String()).Scan(&closed); err != nil {
			return err
		}
		scope, err := dealRefileScope(e.Admin(), tx, e.Pool, closed)
		if !scope.Empty() {
			t.Fatalf("closing a deal scheduled %+v", scope)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.run(t, f.enqueue(t, "contact.created", f.contact))
	for _, activity := range []ids.UUID{f.mail, f.meeting} {
		if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='deal'`, activity); got != 0 {
			t.Fatalf("closed deal took activity %s", activity)
		}
	}
	if _, err := store.AdvanceDeal(e.Admin(), deal, deals.AdvanceDealInput{ToStageID: f.stage}); err != nil {
		t.Fatal(err)
	}
	f.run(t, f.enqueue(t, deals.EventDealStageChanged, f.deal))
	for _, activity := range []ids.UUID{f.mail, f.meeting} {
		if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, activity, f.deal); got != 1 {
			t.Fatalf("reopened deal has %d links for activity %s", got, activity)
		}
	}
}

func TestDealReconsiderationAdvancesPastAFullPageOfAmbiguousActivities(t *testing.T) {
	f := newDealRefileFixture(t, 201)
	e := f.o.e
	company := ids.From[ids.CompanyKind](f.company)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Another rollout", PipelineID: f.pipeline, StageID: f.stage, CompanyID: &company, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	last := f.o.customerWrites(t, "last@later.example", "A later resolvable discussion", f.o.now.Add(-time.Hour))
	contact := f.o.contact(t, "Later buyer", "last@later.example")
	employer := ids.From[ids.CompanyKind](e.SeedCompany(t, "Later customer", nil))
	member := ids.From[ids.ContactKind](contact)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &member, CompanyID: &employer, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Later rollout", PipelineID: f.pipeline, StageID: f.stage, CompanyID: &employer, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	integration.ApplyRiverSchema(t)
	worker := newCapturedDealRefileWorker(e.Pool)
	for _, id := range []ids.UUID{f.contact, contact} {
		if _, err := worker.repairCohorts(principal.SystemActing(e.Admin(), dealRefileActor), []ids.UUID{id}); err != nil {
			t.Fatal(err)
		}
	}
	args := f.enqueue(t, "deal.created", f.deal)
	args.Scope = capture.DealRefileScope{All: true}
	if passes := f.run(t, args); passes < 2 {
		t.Fatalf("drained in %d passes", passes)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, last, deal.Id) != 1 {
		t.Fatal("ambiguous earlier activities starved a resolvable activity")
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE deal_id=$1`, f.deal) != 0 {
		t.Fatal("ambiguous activities were filed")
	}
}

func TestDealReconsiderationRecordsARowFaultAndContinues(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	integration.ApplyRiverSchema(t)
	worker := newCapturedDealRefileWorker(e.Pool)
	ctx := principal.SystemActing(e.Admin(), dealRefileActor)
	if _, err := worker.repairCohorts(ctx, []ids.UUID{f.contact}); err != nil {
		t.Fatal(err)
	}
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN is required by the integration lane")
	}
	owner, err := testdb.Pool(t.Context(), ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	constraint := pgx.Identifier{"refile_fault_" + strings.ReplaceAll(f.mail.String(), "-", "")}.Sanitize()
	if _, err := owner.Exec(t.Context(), `ALTER TABLE activity_link ADD CONSTRAINT `+constraint+` CHECK (activity_id<>'`+f.mail.String()+`'::uuid OR deal_id IS NULL)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE activity_link DROP CONSTRAINT IF EXISTS `+constraint); err != nil {
			t.Error(err)
		}
	})
	args := f.enqueue(t, "deal.created", f.deal)
	f.run(t, args)
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, f.meeting, f.deal) != 1 {
		t.Fatal("one failing row blocked later activities")
	}
	if e.WsCount(t, `SELECT count(*) FROM system_log WHERE action='capture_deal_refile_fault' AND detail->>'activity_id'=$1`, f.mail.String()) != 1 {
		t.Fatal("row fault was not recorded")
	}
	if _, err := owner.Exec(t.Context(), `ALTER TABLE activity_link DROP CONSTRAINT `+constraint); err != nil {
		t.Fatal(err)
	}
	f.run(t, args)
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, f.mail, f.deal) != 1 {
		t.Fatal("a later complete pass did not retry the failed row")
	}
}

func TestAFutureDatedEmailAgesOutWithoutDroppingAnUpcomingMeeting(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	future := f.o.customerWrites(t, "pat@refile.example", "A sender-stated future date", time.Date(2100, 1, 4, 9, 0, 0, 0, time.UTC))
	ctx := principal.SystemActing(e.Admin(), dealRefileActor)
	var receipt time.Time
	if err := InstallationDB(e.Pool).Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT created_at FROM activity WHERE id=@activity`, pgx.NamedArgs{"activity": future}).Scan(&receipt)
	}); err != nil {
		t.Fatal(err)
	}
	worker := newCapturedDealRefileWorker(e.Pool)
	if _, err := worker.repairCohorts(ctx, []ids.UUID{f.contact}); err != nil {
		t.Fatal(err)
	}
	if err := InstallationDB(e.Pool).Tx(ctx, func(tx pgx.Tx) error {
		asOf := receipt.Add(31 * 24 * time.Hour)
		candidates, err := worker.repair.RefileCandidatesTx(ctx, tx, capture.DealRefileScope{All: true}, asOf, ids.Nil, future)
		if err != nil {
			return err
		}
		if slices.Contains(candidates, future) {
			t.Error("future sender timestamp retained an old receipt")
		}
		if !slices.Contains(candidates, f.meeting) {
			t.Error("receipt age removed an upcoming calendar meeting")
		}
		outcome, err := worker.repair.RefileTx(ctx, tx, ids.From[ids.ActivityKind](future), asOf)
		if err == nil && outcome != capture.DealNone {
			t.Errorf("direct refile retained old receipt: %s", outcome)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}
