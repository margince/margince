// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package org360

// The scan's lifecycle over a real database, branch by branch: an unchanged
// account is served from what was read, a changed one is marked stale under
// the floor and read again past it, a deployment with no worker settles on
// the floor in-request, a lane that breaks or defers leaves the reader with
// the rules and the row with the truth, and a reader the account refuses —
// or who is not a person — gets no read at all.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/orgscan"
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
func scanClocked(e *integration.Env, lane orgscan.Completer, queued *[]orgscan.Queued, now func() time.Time) *orgscan.Service {
	view := org360Service(e)
	svc := orgscan.NewService(e.Pool, view, view, lane,
		func(_ context.Context, _ pgx.Tx, scan orgscan.Queued) error {
			*queued = append(*queued, scan)
			return nil
		},
		func() string { return "routing-test" }, now, nil)
	view.RecogniseScanFindings(svc)
	return svc
}

// readOnce opens the account, plays the worker for the one read queued, and
// returns the settled scan.
func readOnce(rep context.Context, t *testing.T, svc *orgscan.Service, org ids.OrganizationID, queued *[]orgscan.Queued) crmcontracts.OrganizationScan {
	t.Helper()
	if _, err := svc.Ensure(rep, org, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(*queued) == 0 {
		t.Fatal("the open queued no read")
	}
	last := (*queued)[len(*queued)-1]
	if err := svc.Run(principal.WithCorrelationID(rep, last.ScanID), last.ScanID, org); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := svc.Get(rep, org)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return got
}

func TestAnUnchangedAccountIsServedFromWhatWasReadAndAChangedOneIsMarkedStale(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, org.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []orgscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	// Before any open, the honest answer is that nobody asked.
	before, err := svc.Get(rep, org)
	if err != nil || before.State != crmcontracts.OrganizationScanStateNever {
		t.Fatalf("get before any open = %q, %v; want never", before.State, err)
	}
	done := readOnce(rep, t, svc, org, &queued)
	if done.State != crmcontracts.OrganizationScanStateDone {
		t.Fatalf("state after the read = %q", done.State)
	}

	// Opened again with nothing changed: the stored read, and no second job.
	again, err := svc.Ensure(rep, org, false)
	if err != nil {
		t.Fatalf("ensure again: %v", err)
	}
	if again.State != crmcontracts.OrganizationScanStateDone || len(queued) != 1 || lane.calls != 1 {
		t.Errorf("an unchanged account was read again: %q, %d queued, %d calls", again.State, len(queued), lane.calls)
	}

	// The account moves — another message — under the hour's floor: the
	// stored read is served, marked stale, and nothing is queued.
	seedInboundAsk(t, e, org.UUID, "workspace")
	stale, err := svc.Ensure(rep, org, false)
	if err != nil {
		t.Fatalf("ensure after a change: %v", err)
	}
	if stale.State != crmcontracts.OrganizationScanStateDone || stale.Stale == nil || !*stale.Stale || len(queued) != 1 {
		t.Errorf("a changed account under the floor: %q stale=%v, %d queued; want the stored read marked stale", stale.State, stale.Stale, len(queued))
	}
	if got, err := svc.Get(rep, org); err != nil || got.Stale == nil || !*got.Stale {
		t.Errorf("get after a change = stale %v, %v; want stale too", got.Stale, err)
	}

	// Forced: the floor and the fingerprint are both overridden.
	forced, err := svc.Ensure(rep, org, true)
	if err != nil {
		t.Fatalf("forced ensure: %v", err)
	}
	if forced.State != crmcontracts.OrganizationScanStateQueued || len(queued) != 2 {
		t.Errorf("forced = %q with %d queued; want a second read", forced.State, len(queued))
	}
	// The re-armed row is a later attempt of the same occurrence, so the
	// rail — which takes only a later attempt's transitions — draws it.
	var attempt int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT attempt FROM org_scan WHERE id = $1`, queued[1].ScanID).Scan(&attempt); err != nil {
		t.Fatalf("read the attempt: %v", err)
	}
	if attempt != 2 {
		t.Errorf("attempt after a forced re-read = %d, want 2", attempt)
	}
}

func TestAChangedAccountPastTheFloorIsReadAgainOnItsOwn(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, org.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []orgscan.Queued
	later := func() time.Time { return time.Now().Add(orgscan.RescanFloor + time.Minute) }
	svc := scanClocked(e, lane, &queued, later)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	readOnce(rep, t, svc, org, &queued)
	seedInboundAsk(t, e, org.UUID, "workspace")
	got, err := svc.Ensure(rep, org, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.State != crmcontracts.OrganizationScanStateQueued || len(queued) != 2 {
		t.Errorf("past the floor = %q with %d queued; want a fresh read", got.State, len(queued))
	}
}

func TestWithNoWorkerTheOpenSettlesOnTheRulesFloorAtOnce(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	view := org360Service(e)
	svc := orgscan.NewService(e.Pool, view, view, nil, nil, nil, time.Now, nil)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	got, err := svc.Ensure(rep, org, false)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.State != crmcontracts.OrganizationScanStateDegraded || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "No worker") {
		t.Errorf("state %q reason %v; want degraded, saying no worker runs scans here", got.State, got.DegradeReason)
	}
	if got.GeneratedBy == nil || *got.GeneratedBy != crmcontracts.Deterministic {
		t.Errorf("generated by %v, want the deterministic floor", got.GeneratedBy)
	}
}

func TestALaneThatBreaksLeavesTheRulesAdviceStanding(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, org.UUID, "workspace")
	lane := &failingLane{}
	var queued []orgscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	got := readOnce(rep, t, svc, org, &queued)
	if got.State != crmcontracts.OrganizationScanStateDegraded || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "did not answer") {
		t.Errorf("state %q reason %v; want degraded because the model did not answer usably", got.State, got.DegradeReason)
	}
	if lane.calls == 0 {
		t.Error("the lane was never asked")
	}
}

func TestABudgetDeferralPutsTheReadOffRatherThanFailingIt(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, org.UUID, "workspace")
	resumes := time.Now().Add(45 * time.Minute).UTC().Truncate(time.Second)
	lane := &failingLane{err: &ai.BudgetDeferralError{Task: ai.TaskAccountScan, NextAttemptAt: resumes}}
	var queued []orgscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	if _, err := svc.Ensure(rep, org, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	err := svc.Run(principal.WithCorrelationID(rep, queued[0].ScanID), queued[0].ScanID, org)
	var deferral *ai.BudgetDeferralError
	if !errors.As(err, &deferral) {
		t.Fatalf("run: %v, want the deferral for the carrier to snooze on", err)
	}
	got, err := svc.Get(rep, org)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != crmcontracts.OrganizationScanStateQueued || got.ResumesAt == nil || !got.ResumesAt.Equal(resumes) {
		t.Errorf("state %q resumes %v; want queued again until %v", got.State, got.ResumesAt, resumes)
	}
	// A read put off is still one read in flight: opening the page again
	// queues nothing more.
	if again, _ := svc.Ensure(rep, org, true); again.State != crmcontracts.OrganizationScanStateQueued || len(queued) != 1 {
		t.Errorf("a deferred read was queued again: %q, %d", again.State, len(queued))
	}
}

func TestAReaderTheAccountRefusesGetsNoRead(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, org.UUID, "workspace")
	theirsRaw := e.SeedOrg(t, "Other Rep's Private Account", &e.Rep3)
	e.MakeCapturePrivate(t, "organization", theirsRaw, e.Rep3)
	theirs := ids.From[ids.OrganizationKind](theirsRaw)
	lane := &quotingLane{}
	var queued []orgscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	// An open on another rep's private account refuses before any row
	// exists, as not-found: its existence stays hidden.
	if _, err := svc.Ensure(owner, theirs, false); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("an open on a hidden account: %v, want not found", err)
	}
	// An agent is refused as a matter of permission: the scan is a person's.
	agent := e.AgentFor(t, e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	if _, err := svc.Ensure(agent, org, false); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent's open: %v, want permission denied", err)
	}
	if _, err := svc.Get(agent, org); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent's read: %v, want permission denied", err)
	}

	// A read the owner queued, run under a principal whose grants no longer
	// open accounts at all, settles failed with a reason and asks the model
	// nothing.
	if _, err := svc.Ensure(owner, org, false); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if len(queued) != 1 {
		t.Fatalf("%d reads queued, want only the owner's: a refused open must queue nothing", len(queued))
	}
	ungranted := e.As(e.Rep1, []ids.UUID{e.Team1}, permsWithout("organization"))
	if err := svc.Run(principal.WithCorrelationID(ungranted, queued[0].ScanID), queued[0].ScanID, org); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := svc.Get(owner, org)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.State != crmcontracts.OrganizationScanStateFailed || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "could not be opened") || lane.calls != 0 {
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
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	seedInboundAsk(t, e, org.UUID, "workspace")
	lane := &quotingLane{}
	var queued []orgscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, permsWithout("activity"))

	got := readOnce(rep, t, svc, org, &queued)
	if got.State != crmcontracts.OrganizationScanStateDegraded || got.DegradeReason == nil || !strings.Contains(*got.DegradeReason, "no exchanges") || lane.calls != 0 {
		t.Errorf("state %q reason %v after %d calls; want degraded with nothing to read and no call", got.State, got.DegradeReason, lane.calls)
	}
	if got.Read == nil || got.Read.Exchanges != 0 {
		t.Errorf("read = %+v, want no exchange counted", got.Read)
	}
}

func TestPuttingOffAFindingNobodyRaisedStoresNothing(t *testing.T) {
	e := integration.Setup(t)
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, org.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []orgscan.Queued
	svc, view := scanFor(e, lane, &queued)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	readOnce(rep, t, svc, org, &queued)
	// Well-formed, so it passes the shape check and is looked for among the
	// rules' rows and then the scan's: a fingerprint neither raised. The
	// dismissal succeeds, saying nothing, and records nothing.
	unraised := strings.Repeat("0", 64)
	if err := view.DismissSuggestion(rep, org, unraised); err != nil {
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
	org := ids.From[ids.OrganizationKind](e.SeedOrg(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, org.UUID, "workspace")
	lane := &quotingLane{messageID: message.String(), quote: "wants to see a sample of the driver reports"}
	var queued []orgscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	everyRow := integration.AccountRepPerms
	everyRow.RowScope = principal.RowScopeAll
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, everyRow)

	got := readOnce(rep, t, svc, org, &queued)
	if got.State != crmcontracts.OrganizationScanStateDone || got.Read == nil || got.Read.Exchanges != 1 {
		t.Errorf("state %q read %+v; want the exchange read and the account done", got.State, got.Read)
	}
}
