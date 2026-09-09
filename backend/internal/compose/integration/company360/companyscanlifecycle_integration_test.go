// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The scan's lifecycle over a real database, branch by branch: an unchanged
// account is served from what was read, a changed one is marked stale under
// the floor and read again past it, a read whose worker died is re-armed by
// the next open and reclaimed by the worker's own retry, a deployment with no
// worker settles on the floor in-request, a lane that breaks or defers leaves
// the reader with the rules and the row with the truth, and a reader the
// account refuses — or who is not a person — gets no read at all.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// failingLane answers with the error it was given, or with words no parser
// takes, so the read exercises the paths a broken lane takes.
type failingLane struct {
	err   error
	calls int
}

func (l *failingLane) Complete(context.Context, model.Request) (model.Response, error) {
	l.calls++
	if l.err != nil {
		return model.Response{}, l.err
	}
	return model.Response{Text: "I would rather not say."}, nil
}

// scanClocked is scanFor with the clock the ensure rule reads, for the
// branches that turn on how long ago the last read settled.
func scanClocked(e *integration.Env, lane companyscan.Completer, queued *[]companyscan.Queued, now func() time.Time) *companyscan.Service {
	view := company360Service(e)
	svc := companyscan.NewService(e.Pool, view, view, lane,
		func(_ context.Context, _ pgx.Tx, scan companyscan.Queued) error {
			*queued = append(*queued, scan)
			return nil
		},
		func() string { return "routing-test" }, now, nil)
	view.RecogniseScanFindings(svc)
	return svc
}

// reasonSaid is a settle's degrade reason as words. A failing assertion that
// printed the pointer would name an address and leave the reader none the
// wiser about which floor the read fell to.
func reasonSaid(reason *string) string {
	if reason == nil {
		return "none"
	}
	return *reason
}

// readOnce opens the account, plays the worker for the one read queued, and
// returns the settled scan.
func readOnce(rep context.Context, t *testing.T, svc *companyscan.Service, company ids.CompanyID, queued *[]companyscan.Queued) crmcontracts.CompanyScan {
	t.Helper()
	if _, err := svc.Ensure(rep, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(*queued) == 0 {
		t.Fatal("the open queued no read")
	}
	last := (*queued)[len(*queued)-1]
	if err := svc.Run(principal.WithCorrelationID(rep, last.ScanID), last.ScanID, company); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return got
}

func TestAnUnchangedAccountIsServedFromWhatWasReadAndAChangedOneIsMarkedStale(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	// Before any open, the honest answer is that nobody asked.
	before, err := svc.Get(rep, company)
	if err != nil || before.State != crmcontracts.CompanyScanStateNever {
		t.Fatalf("get before any open = %q, %v; want never", before.State, err)
	}
	done := readOnce(rep, t, svc, company, &queued)
	if done.State != crmcontracts.CompanyScanStateDone {
		t.Fatalf("state after the read = %q", done.State)
	}

	// Opened again with nothing changed: the stored read, and no second job.
	again, err := svc.Ensure(rep, company, false)
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if again.State != crmcontracts.CompanyScanStateDone || len(queued) != 1 || lane.calls != 1 {
		t.Errorf("an unchanged account was read again: %q, %d queued, %d calls", again.State, len(queued), lane.calls)
	}

	// The account moves — another message — under the hour's floor: the
	// stored read is served, marked stale, and nothing is queued.
	seedInboundAsk(t, e, company.UUID, "workspace")
	stale, err := svc.Ensure(rep, company, false)
	if err != nil {
		t.Fatalf("ensure after a change: %v", err)
	}
	if stale.State != crmcontracts.CompanyScanStateDone || stale.Stale == nil || !*stale.Stale || len(queued) != 1 {
		t.Errorf("a changed account under the floor: %q stale=%v, %d queued; want the stored read marked stale", stale.State, stale.Stale, len(queued))
	}
	if got, err := svc.Get(rep, company); err != nil || got.Stale == nil || !*got.Stale {
		t.Errorf("get after a change = stale %v, %v; want stale too", got.Stale, err)
	}

	// Forced: the floor and the fingerprint are both overridden.
	forced, err := svc.Ensure(rep, company, true)
	if err != nil {
		t.Fatalf("forced ensure: %v", err)
	}
	if forced.State != crmcontracts.CompanyScanStateQueued || len(queued) != 2 {
		t.Errorf("forced = %q with %d queued; want a second read", forced.State, len(queued))
	}
	// The re-armed row is a later attempt of the same occurrence, so the
	// rail — which takes only a later attempt's transitions — draws it.
	if attempt := scanAttempt(t, queued[1].ScanID); attempt != 2 {
		t.Errorf("attempt after a forced re-read = %d, want 2", attempt)
	}
}

func TestAChangedAccountPastTheFloorIsReadAgainOnItsOwn(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	later := func() time.Time { return time.Now().Add(companyscan.RescanFloor + time.Minute) }
	svc := scanClocked(e, lane, &queued, later)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	readOnce(rep, t, svc, company, &queued)
	seedInboundAsk(t, e, company.UUID, "workspace")
	got, err := svc.Ensure(rep, company, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.State != crmcontracts.CompanyScanStateQueued || len(queued) != 2 {
		t.Errorf("past the floor = %q with %d queued; want a fresh read", got.State, len(queued))
	}
}

// killWorker leaves the read as a worker that died mid-read leaves it: running,
// claimed `ago` before now, with nobody coming back for it.
func killWorker(t *testing.T, scanID ids.UUID, ago time.Duration) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `
		UPDATE company_scan SET status = 'running', started_at = now() - ($2 * interval '1 microsecond')
		 WHERE id = $1`, scanID, ago.Microseconds()); err != nil {
		t.Fatalf("leave the read running with a dead worker: %v", err)
	}
}

func scanAttempt(t *testing.T, scanID ids.UUID) int {
	t.Helper()
	var attempt int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT attempt FROM company_scan WHERE id = $1`, scanID).Scan(&attempt); err != nil {
		t.Fatalf("read the attempt: %v", err)
	}
	return attempt
}

// A worker that dies leaves the row running, and nothing but the reader
// opening the account again could ever move it: the page polls a live row,
// the rail calls it stalled past the lease, and neither starts anything.
// Inside the lease the open joins the read — a real worker may hold it —
// and past it the open re-arms the same occurrence as a later attempt.
func TestAReadWhoseWorkerDiedIsReadAgainWhenTheAccountIsOpened(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, company.UUID, "workspace")
	var queued []companyscan.Queued
	svc := scanClocked(e, &failingLane{}, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	if _, err := svc.Ensure(rep, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	killWorker(t, queued[0].ScanID, companyscan.ScanLease-time.Minute)
	held, err := svc.Ensure(rep, company, true)
	if err != nil {
		t.Fatalf("ensure inside the lease: %v", err)
	}
	if held.State != crmcontracts.CompanyScanStateRunning || len(queued) != 1 {
		t.Errorf("inside the lease: %q with %d queued; want the held read served, nothing queued", held.State, len(queued))
	}

	killWorker(t, queued[0].ScanID, companyscan.ScanLease+time.Second)
	rearmed, err := svc.Ensure(rep, company, false)
	if err != nil {
		t.Fatalf("ensure past the lease: %v", err)
	}
	if rearmed.State != crmcontracts.CompanyScanStateQueued || len(queued) != 2 {
		t.Errorf("past the lease: %q with %d queued; want the read queued again", rearmed.State, len(queued))
	}
	if attempt := scanAttempt(t, queued[0].ScanID); queued[1].ScanID != queued[0].ScanID || attempt != 2 {
		t.Errorf("re-armed as %s attempt %d; want the same occurrence at attempt 2", queued[1].ScanID, attempt)
	}
}

// The worker's own retry finds the row its earlier attempt left running. A
// holder inside its lease is left alone — reading the account twice bills it
// twice — and one past it is taken over as a new attempt and read to the end.
func TestTheWorkerReclaimsAReadItsDeadPredecessorLeftRunning(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	if _, err := svc.Ensure(rep, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	scanID := queued[0].ScanID
	worker := principal.WithCorrelationID(rep, scanID)

	killWorker(t, scanID, companyscan.ScanLease-time.Minute)
	if err := svc.Run(worker, scanID, company); err != nil {
		t.Fatalf("run against a live holder: %v", err)
	}
	held, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get while held: %v", err)
	}
	if held.State != crmcontracts.CompanyScanStateRunning || lane.calls != 0 {
		t.Errorf("a holder inside its lease: %q after %d model calls; want left running, unread", held.State, lane.calls)
	}

	killWorker(t, scanID, companyscan.ScanLease+time.Second)
	if err := svc.Run(worker, scanID, company); err != nil {
		t.Fatalf("run against a dead holder: %v", err)
	}
	got, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != crmcontracts.CompanyScanStateDone || lane.calls != 1 {
		t.Errorf("a holder past its lease: %q after %d model calls; want read to done", got.State, lane.calls)
	}
	if attempt := scanAttempt(t, scanID); attempt != 2 {
		t.Errorf("attempt after the reclaim = %d, want 2 so the rail draws the retry", attempt)
	}
}

func TestWithNoWorkerTheOpenSettlesOnTheRulesFloorAtOnce(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	view := company360Service(e)
	svc := companyscan.NewService(e.Pool, view, view, nil, nil, nil, time.Now, nil)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	got, err := svc.Ensure(rep, company, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.State != crmcontracts.CompanyScanStateDegraded || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "No worker") {
		t.Errorf("state %q reason %v; want degraded, saying no worker runs scans here", got.State, got.DegradeReason)
	}
	if got.GeneratedBy == nil || *got.GeneratedBy != crmcontracts.Deterministic {
		t.Errorf("generated by %v, want the deterministic floor", got.GeneratedBy)
	}
}

func TestALaneThatBreaksLeavesTheRulesAdviceStanding(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &failingLane{}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	got := readOnce(rep, t, svc, company, &queued)
	if got.State != crmcontracts.CompanyScanStateDegraded || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "did not answer") {
		t.Errorf("state %q reason %v; want degraded because the model did not answer usably", got.State, got.DegradeReason)
	}
	if lane.calls == 0 {
		t.Error("the lane was never asked")
	}
}

func TestABudgetDeferralPutsTheReadOffRatherThanFailingIt(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, company.UUID, "workspace")
	resumes := time.Now().Add(45 * time.Minute).UTC().Truncate(time.Second)
	lane := &failingLane{err: &ai.BudgetDeferralError{Task: ai.TaskAccountScan, NextAttemptAt: resumes}}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	if _, err := svc.Ensure(rep, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	err := svc.Run(principal.WithCorrelationID(rep, queued[0].ScanID), queued[0].ScanID, company)
	var deferral *ai.BudgetDeferralError
	if !errors.As(err, &deferral) {
		t.Fatalf("run: %v, want the deferral for the carrier to snooze on", err)
	}
	got, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != crmcontracts.CompanyScanStateQueued || got.ResumesAt == nil || !got.ResumesAt.Equal(resumes) {
		t.Errorf("state %q resumes %v; want queued again until %v", got.State, got.ResumesAt, resumes)
	}
	// A read put off is still one read in flight: opening the page again
	// queues nothing more.
	if again, _ := svc.Ensure(rep, company, true); again.State != crmcontracts.CompanyScanStateQueued || len(queued) != 1 {
		t.Errorf("a deferred read was queued again: %q, %d", again.State, len(queued))
	}
}

func TestAReaderTheAccountRefusesGetsNoRead(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, company.UUID, "workspace")
	theirsRaw := e.SeedCompany(t, "Other Rep's Private Account", &e.Rep3)
	e.MakeCapturePrivate(t, "company", theirsRaw, e.Rep3)
	theirs := ids.From[ids.CompanyKind](theirsRaw)
	lane := &quotingLane{}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	// An open on another rep's private account refuses before any row
	// exists, as not-found: its existence stays hidden.
	if _, err := svc.Ensure(owner, theirs, false); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an open on a hidden account: %v, want not found", err)
	}
	// An agent is refused as a matter of permission: the scan is a person's.
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	if _, err := svc.Ensure(agent, company, false); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent's open: %v, want permission denied", err)
	}
	if _, err := svc.Get(agent, company); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent's read: %v, want permission denied", err)
	}

	// A read the owner queued, run under a principal whose grants no longer
	// open accounts at all, settles failed with a reason and asks the model
	// nothing.
	if _, err := svc.Ensure(owner, company, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("%d reads queued, want only the owner's: a refused open must queue nothing", len(queued))
	}
	ungranted := e.As(e.Rep1, []ids.UUID{e.Team1}, permsWithout("company"))
	if err := svc.Run(principal.WithCorrelationID(ungranted, queued[0].ScanID), queued[0].ScanID, company); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := svc.Get(owner, company)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != crmcontracts.CompanyScanStateFailed || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "could not be opened") || lane.calls != 0 {
		t.Errorf("state %q reason %v after %d calls; want failed, unopened, unasked", got.State, got.DegradeReason, lane.calls)
	}
}

// permsWithout is the account rep's grants less one object.
func permsWithout(object string) principal.Permissions {
	perms := integration.AccountRepPerms
	perms.Objects = map[string]principal.ObjectGrant{}
	for granted, grant := range integration.AccountRepPerms.Objects {
		if granted != object {
			perms.Objects[granted] = grant
		}
	}
	return perms
}

func TestAReaderWithoutTheActivityGrantIsReadFromTheRecordsAlone(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, permsWithout("activity"))

	got := readOnce(rep, t, svc, company, &queued)
	if got.State != crmcontracts.CompanyScanStateDone || got.DegradeReason != nil || lane.calls != 0 {
		t.Errorf("state %q reason %q after %d calls; want done with nothing to read and no call", got.State, reasonSaid(got.DegradeReason), lane.calls)
	}
	if got.GeneratedBy == nil || *got.GeneratedBy != crmcontracts.Deterministic {
		t.Errorf("generated by %v, want the rules: nothing was read, so no model wrote this", got.GeneratedBy)
	}
	if got.Read == nil || got.Read.Exchanges != 0 {
		t.Errorf("read = %+v, want no exchange counted", got.Read)
	}
}

// The account every reader meets first: added a moment ago, nothing said to
// it yet, and a website read still in flight behind it. The scan reads what
// is there — nothing — and settles DONE. It must not raise a fault: a
// degraded settle here lands on the agent rail's faults arm, which holds the
// orb amber until somebody acknowledges it, so every new company would wear a
// warning for being new.
func TestAnAccountWithNothingSaidToItYetIsReadCleanly(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	lane := &quotingLane{}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	got := readOnce(rep, t, svc, company, &queued)
	if got.State != crmcontracts.CompanyScanStateDone || got.DegradeReason != nil || lane.calls != 0 {
		t.Errorf("state %q reason %q after %d calls; want done, unexplained and unasked", got.State, reasonSaid(got.DegradeReason), lane.calls)
	}
	if got.Read == nil || got.Read.Exchanges != 0 || got.Read.Deals != 0 {
		t.Errorf("read = %+v, want nought exchanges and nought deals", got.Read)
	}

	// What the rail heard, which is the whole of why the status word matters.
	// Both counts, because a read that announced nothing at all would satisfy
	// "no faults" on its own and the orb would be right by accident.
	var announced, faulted int
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE envelope->'payload'->>'state' IN ('degraded', 'failed'))
		  FROM event_outbox
		 WHERE envelope->>'type' = 'ai_task.state_changed'
		   AND envelope->'payload'->>'source' = $1
		   AND envelope->'payload'->>'occurrence_key' = $2`,
		companyscan.ActivitySource, queued[0].ScanID.String()).Scan(&announced, &faulted); err != nil {
		t.Fatalf("count the rail's transitions: %v", err)
	}
	if announced != 3 || faulted != 0 {
		t.Errorf("the rail heard %d transitions of which %d faults; want queued, running and done, and no fault", announced, faulted)
	}
}

func TestPuttingOffAFindingNobodyRaisedStoresNothing(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc, view := scanFor(e, lane, &queued)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	readOnce(rep, t, svc, company, &queued)
	// Well-formed, so it passes the shape check and is looked for among the
	// rules' rows and then the scan's: a fingerprint neither raised. The
	// dismissal succeeds, saying nothing, and records nothing.
	unraised := strings.Repeat("0", 64)
	if err := view.DismissSuggestion(rep, company, unraised, nil); err != nil {
		t.Fatalf("dismissing a finding nobody raised: %v, want a quiet success", err)
	}
	var stored int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM suggestion_dismissal WHERE fingerprint = $1`, unraised).Scan(&stored); err != nil {
		t.Fatalf("count dismissals: %v", err)
	}
	if stored != 0 {
		t.Errorf("%d dismissal rows stored for a finding nobody raised, want none", stored)
	}
}

// A reader whose grants reach every row reads the account's whole
// correspondence, not only their own team's: the words are scoped the way
// the timeline is for the same reader.
func TestAReaderWithEveryRowReadsTheWholeCorrespondence(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	everyRow := integration.AccountRepPerms
	everyRow.RowScope = principal.RowScopeAll
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, everyRow)

	got := readOnce(rep, t, svc, company, &queued)
	if got.State != crmcontracts.CompanyScanStateDone || got.Read == nil || got.Read.Exchanges != 1 {
		t.Errorf("state %q read %+v; want the exchange read and the account done", got.State, got.Read)
	}
}
