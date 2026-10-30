// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A change the input check reads makes its last reading out of date, and only a
// later pass that read the change makes it current again. Every change here is
// made through the real domain writer, and the event the marker consumes is the
// one that writer put in the outbox.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type freshnessEnv struct {
	*assuranceJobEnv
	marker  *AssuranceFreshness
	store   *assurance.Store
	faulted ids.UUID
	healthy ids.UUID
}

func setupFreshness(t *testing.T) *freshnessEnv {
	t.Helper()
	e := &freshnessEnv{assuranceJobEnv: setupAssuranceJob(t)}
	e.marker = NewAssuranceFreshness(e.Pool, identity.NewService(e.Pool))
	e.store = assurance.NewStore(InstallationDB(e.Pool))
	e.faulted = e.dealNamed(t, "Assurance Faulted")
	e.healthy = e.dealNamed(t, "Assurance Fixture")
	e.mailboxReadOnTime(t)
	if err := e.run(t); err != nil {
		t.Fatalf("the first pass: %v", err)
	}
	if f := e.freshness(t); !f.Known {
		t.Fatal("the first pass read every source and still left freshness unknown")
	}
	return e
}

// mailboxReadOnTime is a mail connector that synced three hours ago, so a pass
// reads every source it needs and may record what it read.
func (e *freshnessEnv) mailboxReadOnTime(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	conn := integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'gmail', $2, 'connected')`, e.Rep1)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_sync_state (connection_id, last_success_at)
		VALUES ($1, now() - interval '3 hours')`, conn); err != nil {
		t.Fatalf("seeding the mailbox: %v", err)
	}
}

func (e *freshnessEnv) dealNamed(t *testing.T, name string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.Pool.QueryRow(e.Admin(), `SELECT id FROM deal WHERE name = $1`, name).Scan(&id); err != nil {
		t.Fatalf("finding the deal %q: %v", name, err)
	}
	return id
}

// editDeal changes the deal's amount through the deals store and returns the
// deal.updated envelope that write staged.
func (e *freshnessEnv) editDeal(t *testing.T, deal ids.UUID, amount int64) kevents.Envelope {
	t.Helper()
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		AmountMinor: &amount, Currency: new("EUR"),
	}); err != nil {
		t.Fatalf("editing the deal: %v", err)
	}
	return e.lastEnvelope(t, "deal.updated", deal)
}

func (e *freshnessEnv) lastEnvelope(t *testing.T, eventType string, entity ids.UUID) kevents.Envelope {
	t.Helper()
	var env kevents.Envelope
	if err := e.Pool.QueryRow(e.Admin(), `
		SELECT envelope FROM event_outbox
		 WHERE envelope->>'type' = $1 AND envelope->'entity'->>'id' = $2
		 ORDER BY seq DESC LIMIT 1`, eventType, entity.String()).Scan(&env); err != nil {
		t.Fatalf("reading the %s the writer staged: %v", eventType, err)
	}
	return env
}

func (e *freshnessEnv) deliver(t *testing.T, envs ...kevents.Envelope) {
	t.Helper()
	if err := e.marker.HandleBatch(context.Background(), envs); err != nil {
		t.Fatalf("the marker refused the envelopes: %v", err)
	}
}

func (e *freshnessEnv) freshness(t *testing.T) assurance.Freshness {
	t.Helper()
	run, err := e.store.LatestRun(e.Admin())
	if err != nil {
		t.Fatalf("reading the latest run: %v", err)
	}
	return run.Freshness
}

// findings is the list the endpoint serves, read through the function the
// server wires to it.
func (e *freshnessEnv) findings(t *testing.T) []assurance.Exception {
	t.Helper()
	var found []assurance.Exception
	if err := e.store.InTx(e.Admin(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		found, err = AssuranceExceptionsIn(ctx, tx, assurance.Population{})
		return err
	}); err != nil {
		t.Fatalf("reading the findings: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("the pass raised no finding, so nothing here could be out of date")
	}
	return found
}

// outOfDate is which deals' findings the list marks out of date. A finding
// with no answer either way fails: after a complete pass it is always known.
func (e *freshnessEnv) outOfDate(t *testing.T) map[ids.UUID]bool {
	t.Helper()
	out := map[ids.UUID]bool{}
	for _, f := range e.findings(t) {
		if f.InputsChanged == nil {
			t.Fatalf("finding %s reached the list with no freshness answer", f.ID)
		}
		out[f.SubjectID] = out[f.SubjectID] || *f.InputsChanged
	}
	return out
}

func (e *freshnessEnv) generation(t *testing.T, deal ids.UUID) int64 {
	t.Helper()
	var generation int64
	if err := e.Pool.QueryRow(e.Admin(), `
		SELECT generation FROM assurance_input_change
		 WHERE subject_kind = 'deal' AND subject_id = $1`, deal).Scan(&generation); err != nil {
		t.Fatalf("reading the deal's generation: %v", err)
	}
	return generation
}

func TestEditingADealMarksItsFindingsOutOfDateUntilTheNextPass(t *testing.T) {
	e := setupFreshness(t)
	if f := e.freshness(t); f.OutOfDate() {
		t.Fatalf("a pass that just ran reads out of date: %+v", f)
	}

	e.deliver(t, e.editDeal(t, e.faulted, 3200000))

	f := e.freshness(t)
	if !f.OutOfDate() || f.ChangedDeals != 1 || f.WorkspaceChanged || f.FirstChangedAt == nil {
		t.Fatalf("after one deal edit the reading is %+v, want one changed deal", f)
	}
	if marked := e.outOfDate(t); !marked[e.faulted] {
		t.Fatalf("the edited deal's findings still read as current: %v", marked)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the next pass: %v", err)
	}
	if f := e.freshness(t); f.OutOfDate() {
		t.Fatalf("the pass that read the edit left the reading out of date: %+v", f)
	}
	if marked := e.outOfDate(t); marked[e.faulted] {
		t.Fatal("the pass that read the edit left its findings out of date")
	}
}

// A stage every deal is judged against reaches every finding, not one deal's.
func TestEditingAStageMarksEveryFindingOutOfDate(t *testing.T) {
	e := setupFreshness(t)
	var stage ids.UUID
	if err := e.Pool.QueryRow(e.Admin(), `SELECT stage_id FROM deal WHERE id = $1`, e.faulted).Scan(&stage); err != nil {
		t.Fatalf("reading the deal's stage: %v", err)
	}
	if _, err := e.Deals.UpdateStage(e.Admin(), ids.From[ids.StageKind](stage), deals.UpdateStageInput{
		Name: new("Qualified again"),
	}); err != nil {
		t.Fatalf("renaming the stage: %v", err)
	}
	e.deliver(t, e.lastEnvelope(t, "stage.updated", stage))

	if f := e.freshness(t); !f.WorkspaceChanged || f.ChangedDeals != 0 {
		t.Fatalf("after a stage edit the reading is %+v, want a workspace change", f)
	}
	for deal, marked := range e.outOfDate(t) {
		if !marked {
			t.Errorf("deal %s's findings read as current after its stage changed", deal)
		}
	}
}

// An event delivered twice, or after a later one, only moves the generation
// up: the reading can become out of date, never current.
func TestADuplicateOrLateDeliveryNeverMakesAReadingCurrent(t *testing.T) {
	e := setupFreshness(t)
	first := e.editDeal(t, e.faulted, 3300000)
	second := e.editDeal(t, e.faulted, 3400000)

	e.deliver(t, second)
	afterSecond := e.generation(t, e.faulted)
	e.deliver(t, first)
	if got := e.generation(t, e.faulted); got <= afterSecond {
		t.Fatalf("a late delivery left the generation at %d, not above %d", got, afterSecond)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the next pass: %v", err)
	}
	e.deliver(t, first)
	if f := e.freshness(t); !f.OutOfDate() {
		t.Fatal("a redelivered event after the pass read as a current reading; it must stay on the safe side")
	}
}

// A burst of edits to one deal, read as one batch, is one row and one step.
func TestABurstOfEditsIsOneMarkerWrite(t *testing.T) {
	e := setupFreshness(t)
	burst := make([]kevents.Envelope, 0, 20)
	for i := range 20 {
		burst = append(burst, e.editDeal(t, e.faulted, int64(3500000+i)))
	}
	e.deliver(t, burst[:1]...)
	before := e.generation(t, e.faulted)

	e.deliver(t, burst...)

	if got := e.generation(t, e.faulted); got != before+1 {
		t.Fatalf("twenty edits in one batch moved the generation from %d to %d, want one step", before, got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM assurance_input_change WHERE subject_id = $1`, e.faulted); n != 1 {
		t.Fatalf("the burst left %d marker rows for one deal, want 1", n)
	}
}

// A change committed while a pass is reading is not the pass's to clear.
func TestAChangeDuringAPassKeepsItsResultOutOfDate(t *testing.T) {
	e := setupFreshness(t)
	duringRead := func(ctx context.Context, tx pgx.Tx, asOf time.Time, coverage []assurance.SourceCoverage) ([]assurance.Subject, error) {
		subjects, err := AssuranceSubjects(ctx, tx, asOf, coverage)
		e.deliver(t, e.editDeal(t, e.faulted, 3600000))
		return subjects, err
	}
	if _, err := e.scanner(duringRead).Scan(e.passCtx(), e.at, nil); err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if f := e.freshness(t); f.ChangedDeals != 1 {
		t.Fatalf("an edit made during the pass reads as checked: %+v", f)
	}
}

// A pass that read the deals but not the mailbox did not read every input,
// so it clears nothing either.
func TestAPassThatMissedASourceKeepsTheReadingOutOfDate(t *testing.T) {
	e := setupFreshness(t)
	e.deliver(t, e.editDeal(t, e.faulted, 3650000))
	mailUnread := func(context.Context, pgx.Tx, time.Time) []assurance.SourceCoverage {
		return []assurance.SourceCoverage{
			{Source: sourceMail, State: assurance.CoverageUnavailable},
			offerCoverage(e.at),
		}
	}
	scanner := assurance.NewScanner(e.store, AssuranceSubjects, mailUnread, AssuranceConfig())
	result, err := scanner.Scan(e.passCtx(), e.at.Add(time.Minute), nil)
	if err != nil || result.Status != assurance.StatusIncomplete || result.EligibleDeals == 0 {
		t.Fatalf("the pass = %+v, %v; want an incomplete run that read the deals", result, err)
	}
	if f := e.freshness(t); f.ChangedDeals != 1 {
		t.Fatalf("a pass that missed the mailbox cleared the edit: %+v", f)
	}
}

// A pass that could not read the deals clears nothing.
func TestAFailedPassKeepsTheReadingOutOfDate(t *testing.T) {
	e := setupFreshness(t)
	e.deliver(t, e.editDeal(t, e.faulted, 3700000))
	unreadable := func(context.Context, pgx.Tx, time.Time, []assurance.SourceCoverage) ([]assurance.Subject, error) {
		return nil, errors.New("the deals could not be read")
	}
	result, err := e.scanner(unreadable).Scan(e.passCtx(), e.at.Add(time.Minute), nil)
	if err != nil || result.Status != assurance.StatusIncomplete {
		t.Fatalf("the failed pass = %+v, %v; want an incomplete run", result, err)
	}
	if f := e.freshness(t); f.ChangedDeals != 1 {
		t.Fatalf("a pass that read no deal cleared the edit: %+v", f)
	}
}

// Reading the panel never runs a pass.
func TestReadingFreshnessRunsNoPass(t *testing.T) {
	e := setupFreshness(t)
	runs := e.WsCount(t, `SELECT count(*) FROM assurance_run`)
	e.deliver(t, e.editDeal(t, e.faulted, 3800000))
	e.freshness(t)
	e.outOfDate(t)
	if got := e.WsCount(t, `SELECT count(*) FROM assurance_run`); got != runs {
		t.Fatalf("reading the panel moved the run count from %d to %d", runs, got)
	}
}

func (e *freshnessEnv) scanner(subjects assurance.SubjectsFunc) *assurance.Scanner {
	return assurance.NewScanner(e.store, subjects, AssuranceCoverage, AssuranceConfig())
}

func (e *freshnessEnv) passCtx() context.Context {
	return principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), assuranceActor)
}

// An installation upgraded from before the tracking has runs that recorded
// nothing. Until a pass records what it read, nothing is claimed either way.
func TestAReadingFromBeforeTheTrackingIsUnknownUntilTheNextPass(t *testing.T) {
	e := setupFreshness(t)
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(context.Background(), `UPDATE assurance_run SET read_through_generation = NULL`); err != nil {
		t.Fatalf("forgetting what the runs read: %v", err)
	}

	f := e.freshness(t)
	if got := assurance.FreshnessToWire(f).State; got != crmcontracts.AssuranceFreshnessStateUnknown {
		t.Fatalf("a reading no run recorded the inputs of is %q, want unknown", got)
	}
	for _, finding := range e.findings(t) {
		if finding.InputsChanged != nil {
			t.Fatalf("finding %s claims inputs_changed=%v, which nothing can know", finding.ID, *finding.InputsChanged)
		}
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the next pass: %v", err)
	}
	if got := assurance.FreshnessToWire(e.freshness(t)).State; got != crmcontracts.AssuranceFreshnessStateCurrent {
		t.Fatalf("after a complete pass the reading is %q, want current", got)
	}
}

// list_input_checks reads through the same function as the endpoint, so a
// model is told a finding is out of date exactly when a screen is.
func TestTheToolMarksAnEditedDealsFindingsOutOfDate(t *testing.T) {
	e := setupFreshness(t)
	e.deliver(t, e.editDeal(t, e.faulted, 3900000))

	raw, err := inputChecksToolReader(e.Pool)(e.Admin(), agents.InputChecksArgs{})
	if err != nil {
		t.Fatalf("list_input_checks: %v", err)
	}
	var answer agents.InputChecksResult
	if err := json.Unmarshal(raw, &answer); err != nil {
		t.Fatalf("decoding list_input_checks: %v", err)
	}
	marked := map[string]bool{}
	for _, check := range answer.Data {
		if check.InputsChanged == nil {
			t.Fatalf("list_input_checks sent finding %s with no freshness answer", check.ID)
		}
		marked[check.SubjectID] = marked[check.SubjectID] || *check.InputsChanged
	}
	if !marked[e.faulted.String()] {
		t.Fatalf("list_input_checks reads the edited deal's findings as current: %v", marked)
	}
}

// A note on the deal moves what its viability read, so the deal is marked; the
// same activity seen again after a pass read it marks nothing.
func TestAnActivityOnADealMarksItWhenTheCheckReadsIt(t *testing.T) {
	e := setupFreshness(t)
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Customer asked for the revised quote"), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.faulted}},
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}
	captured := e.lastEnvelope(t, "activity.captured", ids.UUID(logged.Id))

	e.deliver(t, captured)
	if marked := e.outOfDate(t); !marked[e.faulted] || marked[e.healthy] {
		t.Fatalf("after a note on one deal the marked deals are %v, want that deal alone", marked)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the next pass: %v", err)
	}
	e.deliver(t, captured)
	if f := e.freshness(t); f.OutOfDate() {
		t.Fatalf("a note the pass already read marked the reading again: %+v", f)
	}
}
