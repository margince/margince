// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Overnight follow-up reconciliation over real migrated Postgres. The nightly
// pass turns an interaction with no next step into the deal owner's task, once
// per interaction. A proposal staged before that still decides as it did. A
// confirm creates the task once, and a reject creates nothing. A rep who cannot
// see the deal cannot decide it.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reconcileEnv wraps integration.Env with the default pipeline, the follow-up
// reconciler, and the approvals service carrying its confirm effect.
type reconcileEnv struct {
	*integration.Env
	owner      *pgx.Conn
	pipeline   ids.PipelineID
	open       ids.StageID
	reconciler *deals.FollowUpReconciler
	svc        *approvals.Service
}

// reconcilePerms is a team-scoped rep who may create activities and read
// deals — exactly what confirming a follow-up needs, and no more, so the
// row-scope test bites.
var reconcilePerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"activity":              {Create: true, Read: true},
		"contact":               {Read: true},
		"deal":                  {Read: true, Update: true},
		"pipeline":              {Read: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

func setupReconcile(t *testing.T) *reconcileEnv {
	t.Helper()
	e := &reconcileEnv{Env: integration.Setup(t), owner: integration.OwnerConn(t)}
	e.pipeline, e.open, _ = integration.DealFixture(t, e.Env)
	quiet := slog.New(slog.NewTextHandler(os.Stderr, nil))
	e.svc = approvals.NewService(e.DB())
	e.svc.WithEffect(deals.FollowUpReconcileKind, followUpConfirmEffect(e.svc, e.Activities, func() time.Time { return clocktest.Now(t) }))
	e.svc.WithPrecheck(deals.FollowUpReconcileKind, followUpPrecheck(e.DB(), func() time.Time { return clocktest.Now(t) }))
	// The stager production builds, drafter included. A harness that omitted
	// the drafting seam would exercise a stager that can only ever propose a
	// task, and would report the drafted-reply branch as working while nothing
	// in production reached it.
	stager := followUpStager{
		svc:   e.svc,
		tasks: e.Activities,
		draft: newCommsAdapter(e.Pool, nil, SendPath{}),
		owner: dealOwnerAuthority{db: e.DB(), users: identity.NewServiceFor(e.DB())},
		pool:  e.Pool,
	}
	e.reconciler = deals.NewFollowUpReconciler(e.DB(), stager, quiet)
	// The deal owner needs real grants, not the harness's in-memory ones. The
	// drafter composes under the owner's authority as the database holds it.
	// Without them every deal falls back to the task, and the draft tests
	// would prove nothing.
	e.grantOwner(t, e.Rep1, reconcileOwnerPolicy)
	return e
}

// setupReconcileWithOwnerPolicy is setupReconcile with the deal owner's stored
// grants named by the caller, for the suites that prove WHICH grant the drafting
// path depends on. Everything else is identical, so a test using it isolates the
// one grant it varies.
//
// The default role is REPLACED rather than added to. Grants accumulate across
// role assignments, so layering a narrow document on the broad one would leave
// the owner holding everything and the test would pass while proving nothing.
func setupReconcileWithOwnerPolicy(t *testing.T, document string) *reconcileEnv {
	t.Helper()
	e := setupReconcile(t)
	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM role_assignment WHERE user_id = $1`, e.Rep1); err != nil {
		t.Fatalf("clearing the default owner role: %v", err)
	}
	e.grantOwner(t, e.Rep1, document)
	return e
}

// reconcileOwnerPolicy mirrors reconcilePerms as a stored role document: an
// activity-and-deal rep WITH contact read, since composing a reply resolves the
// counterparty's address. The suite that proves the gate bites uses the
// narrower policy below instead.
const reconcileOwnerPolicy = `{"objects":{"activity":{"create":true,"read":true,"update":true},
	  "deal":{"read":true,"update":true},"contact":{"read":true},
	  "company":{"read":true},"pipeline":{"read":true}},"row_scope":"all"}`

// grantOwner gives a member an actual role row, which is what
// EffectiveAuthority reads. The harness's As(...) permissions never reach this
// path, deliberately: the drafter resolves authority from the database so a
// card is only ever composed under grants the owner really holds.
func (e *reconcileEnv) grantOwner(t *testing.T, userID ids.UUID, document string) {
	t.Helper()
	ctx := context.Background()
	roleID := ids.NewV7()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO role (id, key, name, permissions) VALUES ($1, $2, 'Reconcile rep', $3)`,
		roleID, "reconcile_rep_"+roleID.String(), document); err != nil {
		t.Fatalf("seeding the owner's role: %v", err)
	}
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO role_assignment (id, user_id, role_id) VALUES ($1, $2, $3)`,
		ids.NewV7(), userID, roleID); err != nil {
		t.Fatalf("assigning the owner's role: %v", err)
	}
}

// reconcile runs the reconciler over this env's workspace under exactly the
// scope the follow_up_workspace worker binds.
func (e *reconcileEnv) reconcile() error {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.SystemActing(ctx, "agent:overnight")
	return e.reconciler.ReconcileWorkspace(ctx)
}

// seedInteraction plants a captured call/mail/meeting on the deal,
// occurredHoursAgo before now — the "real touch" side of the discrepancy.
func (e *reconcileEnv) seedInteraction(t *testing.T, dealID ids.UUID, kind, subject string, occurredHoursAgo int) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx,
		`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		 VALUES ($1, $2, $3, now() - make_interval(hours => $4), 'manual', 'human:x')`,
		id, kind, subject, occurredHoursAgo); err != nil {
		t.Fatalf("seed %s activity: %v", kind, err)
	}
	e.linkActivity(t, id, dealID)
	return id
}

// seedMeeting plants a meeting at an offset from now, so a test can say
// "tomorrow" or "an hour ago" directly. A NEGATIVE hours value is the future.
//
// status is written as given, and "" writes NULL — which is what a captured
// calendar entry nobody has marked actually carries, and the case a NOT IN
// predicate silently drops.
//
// The meeting is a colleague's (Rep2 hosts it, the deals are Rep1's). A
// meeting anyone here hosts is the account's next step.
func (e *reconcileEnv) seedMeeting(
	t *testing.T, dealID ids.UUID, subject string, occurredHoursAgo, durationSeconds int, status string,
) {
	t.Helper()
	e.seedMeetingHostedBy(t, dealID, subject, occurredHoursAgo, durationSeconds, status, &e.Rep2)
}

// seedMeetingHostedBy is seedMeeting with the host named; nil is an imported
// row no seat here hosts or attends.
func (e *reconcileEnv) seedMeetingHostedBy(
	t *testing.T, dealID ids.UUID, subject string, occurredHoursAgo, durationSeconds int, status string, host *ids.UUID,
) {
	t.Helper()
	id := ids.NewV7()
	var meetingStatus *string
	if status != "" {
		meetingStatus = &status
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, duration_seconds, meeting_status, source, captured_by, host_user_id)
		 VALUES ($1, 'meeting', $2, now() - make_interval(hours => $3), $4, $5, 'manual', 'human:x', $6)`,
		id, subject, occurredHoursAgo, durationSeconds, meetingStatus, host); err != nil {
		t.Fatalf("seed meeting: %v", err)
	}
	e.linkActivity(t, id, dealID)
}

// seedTask plants a task on the deal — the "next step already planned"
// side that suppresses the proposal when it is still open.
func (e *reconcileEnv) seedTask(t *testing.T, dealID ids.UUID, done bool) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity (id, kind, subject, occurred_at, due_at, is_done, done_at, source, captured_by)
		 VALUES ($1, 'task', 'Existing next step', now(), now() + interval '2 days', $2,
		         CASE WHEN $2 THEN now() ELSE NULL END, 'manual', 'human:x')`,
		id, done); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	e.linkActivity(t, id, dealID)
	return id
}

func (e *reconcileEnv) linkActivity(t *testing.T, activityID, dealID ids.UUID) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, activityID, dealID); err != nil {
		t.Fatalf("link activity to deal: %v", err)
	}
}

// followUps counts the overnight follow-up tasks on the deal.
func (e *reconcileEnv) followUps(t *testing.T, dealID ids.UUID) int {
	t.Helper()
	count, _, _, _ := e.dealTasks(t, dealID)
	return count
}

// followUpEvidence reads the interaction the deal's latest follow-up task
// answers, and that interaction's kind and time.
func (e *reconcileEnv) followUpEvidence(t *testing.T, dealID ids.UUID) (ids.UUID, string, time.Time) {
	t.Helper()
	var evidence ids.UUID
	var kind string
	var occurred time.Time
	if err := e.owner.QueryRow(context.Background(), `
		SELECT ev.id, ev.kind, ev.occurred_at FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		JOIN activity ev ON a.source_id = $1::text || '/' || ev.id::text
		WHERE a.kind = 'task' AND a.source_system = 'overnight-reconcile'
		ORDER BY a.created_at DESC LIMIT 1`, dealID).Scan(&evidence, &kind, &occurred); err != nil {
		t.Fatalf("no follow-up task names its evidence: %v", err)
	}
	return evidence, kind, occurred
}

// stageLegacyFollowUp stages the proposal the pass staged before it filed
// tasks directly, for the rows still waiting on a decision.
func (e *reconcileEnv) stageLegacyFollowUp(t *testing.T, dealID, evidence ids.UUID) (ids.ApprovalID, deals.FollowUpProposal) {
	t.Helper()
	proposal := deals.FollowUpProposal{
		DealID:             ids.From[ids.DealKind](dealID),
		DueDate:            today().AddDate(0, 0, 3).Format(time.DateOnly),
		Subject:            "Follow up on the deal",
		Body:               "Follow up on the call.",
		EvidenceActivityID: ids.From[ids.ActivityKind](evidence),
		EvidenceKind:       "call",
		EvidenceOccurredAt: today(),
	}
	raw, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = onBehalfOf(principal.SystemActing(ctx, overnightAgent), e.Rep1)
	id, err := e.svc.Stage(ctx, approvals.StageInput{
		Kind: deals.FollowUpReconcileKind, ProposedChange: canonical, DiffHash: hash,
		TargetType: approvalTargetDeal, TargetID: dealID, Summary: "Draft a follow-up",
	})
	if err != nil {
		t.Fatalf("staging a follow-up proposal: %v", err)
	}
	return id, proposal
}

// dealTasks reports the follow-up tasks the confirm effect created on the
// deal, and the provenance of the first — used to prove exactly-once and
// the agent:overnight attribution.
func (e *reconcileEnv) dealTasks(t *testing.T, dealID ids.UUID) (count int, capturedBy, sourceSystem string, due *time.Time) {
	t.Helper()
	ctx := context.Background()
	if err := e.owner.QueryRow(ctx, `
		SELECT count(*) FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.kind = 'task' AND a.source = 'overnight-reconcile'`, dealID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		return 0, "", "", nil
	}
	if err := e.owner.QueryRow(ctx, `
		SELECT a.captured_by, a.source_system, a.due_at FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.kind = 'task' AND a.source = 'overnight-reconcile'
		ORDER BY a.occurred_at DESC LIMIT 1`, dealID).Scan(&capturedBy, &sourceSystem, &due); err != nil {
		t.Fatal(err)
	}
	return count, capturedBy, sourceSystem, due
}

func (e *reconcileEnv) dealVersion(t *testing.T, dealID ids.UUID) int64 {
	t.Helper()
	var v int64
	if err := e.owner.QueryRow(context.Background(), `SELECT version FROM deal WHERE id = $1`, dealID).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

// --- filing: a real touch with no next step becomes the owner's task ---

func TestFollowUpReconcileFilesTheTaskOnTheOwnersList(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Touched, no next step", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Discovery call", 1)
	before := e.dealVersion(t, deal)

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}

	count, capturedBy, sourceSystem, due := e.dealTasks(t, deal)
	if count != 1 {
		t.Fatalf("follow-up tasks = %d, want 1", count)
	}
	if capturedBy != "agent:overnight" || sourceSystem != "overnight-reconcile" {
		t.Errorf("task captured by %q from %q, want agent:overnight from overnight-reconcile", capturedBy, sourceSystem)
	}
	wantDue := today().AddDate(0, 0, 3).Format(time.DateOnly)
	if due == nil || due.UTC().Format(time.DateOnly) != wantDue {
		t.Errorf("task due = %v, want %s (today + follow-up lead)", due, wantDue)
	}
	if evidence, kind, _ := e.followUpEvidence(t, deal); evidence != call || kind != "call" {
		t.Errorf("task answers %s %s, want the seeded call %s", kind, evidence, call)
	}
	// Nothing waits on a decision, and the deal's own fields are untouched.
	var staged int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE target_entity_id = $1`, deal).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if staged != 0 {
		t.Errorf("the pass staged %d approvals, want none: the task is filed directly", staged)
	}
	if after := e.dealVersion(t, deal); after != before {
		t.Errorf("deal version moved %d → %d; the pass writes a task, not the deal", before, after)
	}
}

// --- suppression: an existing next step, no real touch, and dedupe ---

func TestFollowUpReconcileSuppressesWhenNoDiscrepancy(t *testing.T) {
	e := setupReconcile(t)

	// A recent call but an OPEN task already queued: the rep has a next
	// step — do not nag.
	planned := e.SeedDeal(t, "Has next step", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, planned, "call", "Kickoff", 2)
	e.seedTask(t, planned, false)

	// A deal with only a note (not a call/mail/meeting): no real touch.
	noteOnly := e.SeedDeal(t, "Note only", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, noteOnly, "email", "Old thread", 24*10) // outside the 48h window

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, planned); got != 0 {
		t.Errorf("deal with an open next step filed %d follow-up tasks, want 0", got)
	}
	if got := e.followUps(t, noteOnly); got != 0 {
		t.Errorf("deal with no recent interaction filed %d follow-up tasks, want 0", got)
	}
}

// A meeting the rep has BOOKED is not a touch that already happened, and the
// sweep used to read it as one: the evidence window had only a lower bound, so
// tomorrow's meeting satisfied "landed in the window" and — being the newest
// row — outranked the real interaction the follow-up should have named. The rep
// was told a meeting "left no next step planned" the day before it happened.
// The deal carries a real past call AND tomorrow's meeting, which is the shape
// that reproduced it: an open task is deliberately absent, so the only thing
// standing between this deal and a proposal is the meeting. Seeding the future
// meeting ALONE would prove nothing — it falls outside the lookback's lower
// bound too, so the sweep would stay silent even with the bug present.
func TestAMeetingBookedForTomorrowIsNotEvidenceOfAMissedNextStep(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Meeting booked ahead", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	e.seedMeeting(t, deal, "Architecture review", -24, 3600, "")

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 0 {
		t.Errorf("a meeting booked for tomorrow filed %d follow-up tasks, want 0 — "+
			"it has not happened, so it is neither a touch to reconcile nor a "+
			"conversation that ended without a next step", got)
	}

	if t.Failed() {
		_, kind, at := e.followUpEvidence(t, deal)
		t.Logf("the task named evidence of kind %q at %s", kind, at)
	}
}

// An imported meeting nobody here hosts or attends is history, not a plan: it
// does not stand in for the account's next step.
func TestAnImportedMeetingNobodyTakesPartInIsNotANextStep(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Call then an imported meeting", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	e.seedMeetingHostedBy(t, deal, "Imported session", -48, 1800, "booked", nil)

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 1 {
		t.Errorf("a deal whose only meeting nobody here takes part in filed %d follow-up tasks, want 1", got)
	}
}

// The other half of the same fact: a meeting on the calendar IS the next step.
// A past call with nothing queued would normally earn a proposal; a booked
// meeting answers it, because the rep already has the thing the proposal would
// ask them to arrange.
func TestABookedMeetingCountsAsThePlannedNextStep(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Call then a booked meeting", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	e.seedMeeting(t, deal, "Follow-up session", -48, 1800, "booked")

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 0 {
		t.Errorf("a deal with a meeting on the calendar filed %d follow-up tasks, want 0", got)
	}

	// The guard on this test's own premise: the same call WITHOUT the booked
	// meeting must still earn a proposal. Without this, the assertion above
	// would pass just as well if the sweep had stopped working entirely.
	bare := e.SeedDeal(t, "Call and nothing else", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, bare, "call", "Discovery call", 3)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, bare); got != 1 {
		t.Errorf("a call with no next step filed %d follow-up tasks, want 1 — if this is "+
			"0 the sweep is silent for some other reason and the case above proves "+
			"nothing", got)
	}
}

// A meeting that will never happen is not a next step. Cancelled and no-show
// both mean the calendar is empty again, so the deal is back to owing one.
func TestACancelledFutureMeetingIsNotANextStep(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Meeting called off", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	e.seedMeeting(t, deal, "Session nobody will attend", -24, 3600, "canceled")

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 1 {
		t.Errorf("a cancelled meeting suppressed the follow-up (%d filed, want 1) — "+
			"a meeting that will never happen leaves the deal owing a next step", got)
		return
	}

	// And the task must name the CALL, not the cancelled meeting in the
	// future. This is the assertion that isolates the evidence window: a
	// cancelled meeting is not a next step, so nothing else stops the sweep
	// reaching for it as the newest row — only the upper time bound does.
	_, kind, at := e.followUpEvidence(t, deal)
	if kind != "call" {
		t.Errorf("the task named a %q from %s as its evidence, want the call — "+
			"a meeting that has not happened cannot be the interaction that left "+
			"no next step", kind, at)
	}
	if at.After(time.Now()) {
		t.Errorf("the task's evidence is dated %s, which is in the future", at)
	}
}

// A meeting still in progress is not yet evidence either: it started, but "over"
// means ended, so the conversation cannot have left anything behind yet.
func TestAMeetingStillRunningIsNotYetEvidence(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Meeting under way", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Earlier call", 3)
	// Started an hour ago and scheduled for two hours, so it is neither over
	// nor still in the future. A running meeting has to count as the PLANNED
	// next step for this to hold: otherwise the earlier call is evidence, the
	// meeting is not yet a plan, and the sweep tells a rep they have no next
	// step while they are sitting in one.
	e.seedMeeting(t, deal, "Long workshop", 1, 2*3600, "")

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 0 {
		t.Errorf("a meeting still running filed %d follow-up tasks, want 0 — a rep in "+
			"the meeting is not a rep who forgot to plan one", got)
	}
}

// A future row already marked `held` is not a plan. Everywhere else in the tree
// that status means the meeting happened, so a deal whose only forward-dated
// meeting carries it still owes a next step.
func TestAFutureMeetingAlreadyMarkedHeldIsNotAPlan(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Settled ahead of time", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)
	e.seedMeeting(t, deal, "Already recorded as done", -24, 3600, "held")

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 1 {
		t.Errorf("a future meeting marked held suppressed the follow-up (%d filed, "+
			"want 1) — `held` says the meeting happened, so it is not a plan", got)
	}
}

// The task lands on the deal owner's list, driven through the REAL reconciler.
func TestTheFollowUpTaskIsAssignedToTheDealOwner(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Owned by a rep", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery call", 3)

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	var assignee *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT a.assignee_id FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.source_system = 'overnight-reconcile'`, deal).Scan(&assignee); err != nil {
		t.Fatalf("reading the follow-up task: %v", err)
	}
	if assignee == nil || *assignee != e.Rep1 {
		t.Errorf("the task is assigned to %v, want the deal owner %s", assignee, e.Rep1)
	}
}

// A deal nobody owns gets no follow-up: nobody is there to do it.
func TestAnUnownedDealGetsNoFollowUpTask(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Nobody owns this", e.pipeline, e.open, nil)
	// CreateDeal falls back to the ACTING principal when handed no owner
	// (storekit.OwnerOrActor), so a nil here seeds a deal owned by the admin.
	// Clearing the column is the only way to reach the state this test is about
	// — the one ON DELETE SET NULL leaves behind when a member is removed.
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE deal SET owner_id = NULL WHERE id = $1`, deal); err != nil {
		t.Fatalf("clearing the deal owner: %v", err)
	}
	e.seedInteraction(t, deal, "call", "Discovery call", 3)

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 0 {
		t.Errorf("an unowned deal got %d follow-up task(s), want none", got)
	}
}

// The slipping lane reads the SAME "has a next step" question through
// dealsWithNoOpenNextStep, and it is a different reader with its own statement.
// Both were task-only before, so a fix to one alone would have left the two
// disagreeing: the at-risk lane reporting "no next step" about a deal the
// overnight sweep had just called planned.
func TestTheSlippingLaneReadsABookedMeetingAsANextStep(t *testing.T) {
	e := setupReconcile(t)
	booked := e.SeedDeal(t, "Has a meeting booked", e.pipeline, e.open, &e.Rep1)
	e.seedMeeting(t, booked, "Next Thursday", -72, 3600, "booked")

	bare := e.SeedDeal(t, "Has nothing planned", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, bare, "call", "Discovery call", 3)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx,
		principal.Principal{Type: principal.PrincipalSystem, ID: "agent:overnight"})
	stepless, err := dealsWithNoOpenNextStep(
		ctx, e.Pool, []ids.UUID{booked, bare}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if stepless[booked] {
		t.Errorf("the lane reports no next step for a deal with a meeting on the " +
			"calendar; the overnight sweep reads the same deal as planned")
	}
	// The premise guard: the reader must still find the deal that genuinely has
	// nothing, or the assertion above passes for a reader that answers "planned"
	// to everything.
	if !stepless[bare] {
		t.Errorf("the lane found a next step on a deal that has only a past call")
	}
}

func TestFollowUpReconcileDoesNotStackAcrossPasses(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Reconciled twice", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Call", 1)

	for pass := range 2 {
		if err := e.reconcile(); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}
	if got := e.followUps(t, deal); got != 1 {
		t.Errorf("after two passes, follow-up tasks = %d, want still 1 (no duplicate)", got)
	}
}

// --- a proposal staged before tasks were filed directly ---

func TestFollowUpConfirmCreatesTheTaskExactlyOnce(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Confirm me", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Discovery", 1)
	approvalID, proposal := e.stageLegacyFollowUp(t, deal, call)

	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, true, nil); err != nil {
		t.Fatalf("approve + effect: %v", err)
	}

	count, capturedBy, sourceSystem, due := e.dealTasks(t, deal)
	if count != 1 {
		t.Fatalf("follow-up tasks created = %d, want exactly 1", count)
	}
	if capturedBy != "agent:overnight" {
		t.Errorf("captured_by = %q, want agent:overnight (the agent's suggestion, on behalf of the human)", capturedBy)
	}
	if sourceSystem != "overnight-reconcile" {
		t.Errorf("source_system = %q, want overnight-reconcile", sourceSystem)
	}
	if due == nil || due.Format(time.DateOnly) != proposal.DueDate {
		t.Errorf("task due = %v, want the proposed %s", due, proposal.DueDate)
	}

	// Exactly-once: the proposal is no longer pending, so a re-driven
	// decision is refused and no second task is created.
	if _, err := e.svc.Decide(human, approvalID, true, nil); err == nil {
		t.Error("a second decision on an approved proposal succeeded; want it refused")
	}
	if again, _, _, _ := e.dealTasks(t, deal); again != 1 {
		t.Errorf("tasks after a replayed decision = %d, want still 1", again)
	}
}

// An overnight proposal waits until somebody works their morning inbox, and
// that is exactly the window a rep moves the stage, edits the amount or
// corrects the close date in. None of those can make "this deal was touched and
// has no next step" false, and the effect creates an ACTIVITY — it reads no
// field of the deal at all.
//
// The stager has always said it carries no pin. That stopped being true when
// the pin moved server-side, and nothing failed until a human approved after an
// edit: the decision commits first, so the refusal arrives as an approved
// proposal whose task was never created.
func TestADealEditDoesNotCancelAWaitingFollowUp(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Edited while the question waited", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Discovery", 1)
	approvalID, _ := e.stageLegacyFollowUp(t, deal, call)

	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	before := e.dealVersion(t, deal)
	// Through the real writer, so the row's version moves the way any edit in
	// the product moves it.
	renamed := "Renamed while it waited"
	if _, err := deals.NewStore(e.DB(), DealsInstallation()).UpdateDeal(human, ids.From[ids.DealKind](deal),
		deals.UpdateDealInput{Name: &renamed}); err != nil {
		t.Fatalf("editing the deal: %v", err)
	}
	if after := e.dealVersion(t, deal); after == before {
		t.Fatalf("the deal's version did not move (still %d), so this test would pass whether or "+
			"not the pin is declined", after)
	}

	if _, err := e.svc.Decide(human, approvalID, true, nil); err != nil {
		t.Fatalf("approve after the edit: %v — a deal edit must not cancel a waiting follow-up", err)
	}
	if count, _, _, _ := e.dealTasks(t, deal); count != 1 {
		t.Errorf("follow-up tasks created = %d, want 1 — the approval was released but its effect "+
			"did not run", count)
	}
}

func TestFollowUpRejectWritesNothing(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Reject me", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Sync", 1)
	approvalID, _ := e.stageLegacyFollowUp(t, deal, call)

	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if got, _, _, _ := e.dealTasks(t, deal); got != 0 {
		t.Errorf("a rejected follow-up created %d tasks, want 0", got)
	}
	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM approval WHERE id = $1`, approvalID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" {
		t.Errorf("approval status = %q, want rejected", status)
	}
}

// A rep's answer to a proposal staged before tasks were filed directly still
// stands: the next pass over the same interaction writes no task.
func TestALegacyFollowUpTheRepAnsweredIsNotFiledAgain(t *testing.T) {
	for name, approve := range map[string]bool{"declined": false, "approved and done": true} {
		t.Run(name, func(t *testing.T) {
			e := setupReconcile(t)
			deal := e.SeedDeal(t, "Answered once", e.pipeline, e.open, &e.Rep1)
			call := e.seedInteraction(t, deal, "call", "Sync", 1)
			approvalID, _ := e.stageLegacyFollowUp(t, deal, call)
			human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
			if _, err := e.svc.Decide(human, approvalID, approve, nil); err != nil {
				t.Fatalf("decide: %v", err)
			}
			e.WsExec(t, `UPDATE activity SET is_done = true, done_at = now() WHERE source_system = 'overnight-reconcile'`)
			before, _, _, _ := e.dealTasks(t, deal)

			if err := e.reconcile(); err != nil {
				t.Fatal(err)
			}
			if after, _, _, _ := e.dealTasks(t, deal); after != before {
				t.Errorf("tasks after the next pass = %d, want still %d", after, before)
			}
		})
	}
}

// An edit the effect cannot use REFUSES the decision, rather than recording a
// yes that produces nothing.
//
// The approval commits before its effect runs and a failed effect never
// un-decides it, so without a preflight the rep sees a decision go through, no
// task appears, and there is no surface to decide the row again — the work
// simply did not happen and nothing says so. A due date is the reachable case:
// the card lets a human edit the payload, and the effect parses that date.
func TestAnEditTheEffectCannotUseRefusesTheDecision(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Edited badly", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Sync", 1)
	approvalID, proposal := e.stageLegacyFollowUp(t, deal, call)

	proposal.DueDate = "next Tuesday"
	edited, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	_, err = e.svc.DecideEdited(human, approvalID, edited)
	if err == nil {
		t.Fatal("an unusable due date was accepted, want the decision refused")
	}
	// The rep has to be able to ACT on the refusal. An untyped error here reads
	// as 500 internal at the handler, which says the server broke rather than
	// that the date needs fixing — and a rep told that has no reason to retry.
	if _, ok := errors.AsType[*approvals.InvalidEditError](err); !ok {
		t.Fatalf("refusal is %T, want *approvals.InvalidEditError so the rep "+
			"gets a 422 naming the field rather than an opaque 500: %v", err, err)
	}

	// Refused means UNDECIDED: the rep fixes the date and approves the same row.
	var status string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status FROM approval WHERE id = $1`, approvalID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "pending" {
		t.Errorf("approval status = %q, want pending — a refused decision must "+
			"leave the row answerable", status)
	}
	if got, _, _, _ := e.dealTasks(t, deal); got != 0 {
		t.Errorf("a refused decision created %d tasks, want 0", got)
	}
}

// A VALID edit still goes through, and the effect uses the edited value.
//
// The admit case beside the refusal above: a preflight that refused everything
// would pass that test just as well, and would have quietly broken the one
// thing the card exists for — a rep changing the date before saying yes.
func TestAValidEditIsApprovedAndTheEffectUsesIt(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Edited well", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Sync", 1)
	approvalID, proposal := e.stageLegacyFollowUp(t, deal, call)

	moved := time.Now().UTC().AddDate(0, 0, 9).Format(time.DateOnly)
	proposal.DueDate = moved
	edited, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.DecideEdited(human, approvalID, edited); err != nil {
		t.Fatalf("a valid edit was refused: %v", err)
	}
	count, _, _, due := e.dealTasks(t, deal)
	if count != 1 {
		t.Fatalf("the approved follow-up created %d tasks, want 1", count)
	}
	if due == nil || due.UTC().Format(time.DateOnly) != moved {
		t.Errorf("the task is due %v, want the edited date %s — the effect used "+
			"the staged proposal rather than the human's edit", due, moved)
	}
}

// A task the rep cleared is not written again: the next night's pass over
// the same interaction gets the first task back instead.
func TestAFollowUpTheRepClearedIsNotWrittenAgain(t *testing.T) {
	for _, clear := range []string{"is_done = true, done_at = now()", "archived_at = now()"} {
		t.Run(clear, func(t *testing.T) {
			e := setupReconcile(t)
			deal := e.SeedDeal(t, "Cleared once", e.pipeline, e.open, &e.Rep1)
			e.seedInteraction(t, deal, "call", "Sync", 1)
			if err := e.reconcile(); err != nil {
				t.Fatal(err)
			}
			e.WsExec(t, `UPDATE activity SET `+clear+` WHERE source_system = 'overnight-reconcile'`)

			if err := e.reconcile(); err != nil {
				t.Fatal(err)
			}
			var written int
			if err := e.owner.QueryRow(context.Background(),
				`SELECT count(*) FROM activity WHERE source_system = 'overnight-reconcile'`).Scan(&written); err != nil {
				t.Fatal(err)
			}
			if written != 1 {
				t.Errorf("follow-up tasks after the rep cleared the first = %d, want still 1", written)
			}
		})
	}
}

// A cleared task covers the conversation it answered, not the deal forever. A
// later conversation that ends with no next step gets its own task.
func TestANewConversationGetsANewFollowUpAfterTheLastWasDone(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Moved on", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery", 40)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE activity SET is_done = true, done_at = now() WHERE source_system = 'overnight-reconcile'`)

	fresh := e.seedInteraction(t, deal, "call", "They called back", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 2 {
		t.Fatalf("follow-up tasks = %d, want 2: the new conversation has none", got)
	}
	if evidence, _, _ := e.followUpEvidence(t, deal); evidence != fresh {
		t.Errorf("the new task answers %s, want the fresh call %s", evidence, fresh)
	}
}

// --- row scope: a proposal never leaks a deal the decider cannot see ---

func TestFollowUpProposalRespectsRowScope(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Rep1's deal", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Private call", 1)
	approvalID, _ := e.stageLegacyFollowUp(t, deal, call)

	// rep3 sits in team2; rep1's deal is invisible to them, so the staged
	// proposal reads as absent — no decide oracle for a leaked UUID.
	outsider := e.As(e.Rep3, []ids.UUID{e.Team2}, reconcilePerms)
	if _, err := e.svc.Decide(outsider, approvalID, true, nil); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("outsider decide → %v, want ErrNotFound (row-scope existence hiding)", err)
	}
	if got, _, _, _ := e.dealTasks(t, deal); got != 0 {
		t.Errorf("an undecidable proposal still created %d tasks, want 0", got)
	}

	// The owner can see and confirm it.
	owner := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(owner, approvalID, true, nil); err != nil {
		t.Fatalf("owner decide: %v", err)
	}
	if got, _, _, _ := e.dealTasks(t, deal); got != 1 {
		t.Errorf("owner confirm created %d tasks, want 1", got)
	}
}

// A call that reached nobody is no conversation to follow up on: a deal whose
// only call left a voicemail gets no proposal, and once a reached call lands,
// the proposal is grounded on that one.
func TestFollowUpReconcileGroundsOnlyOnACallThatReachedSomebody(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Rang, nobody there", e.pipeline, e.open, &e.Rep1)
	voicemail := e.seedInteraction(t, deal, "call", "Rang the buyer", 2)
	if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET call_outcome = 'voicemail' WHERE id = $1`, voicemail); err != nil {
		t.Fatal(err)
	}
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.followUps(t, deal); got != 0 {
		t.Fatalf("a voicemail grounded %d follow-up tasks, want none", got)
	}

	reached := e.seedInteraction(t, deal, "call", "Spoke to the buyer", 1)
	if _, err := e.owner.Exec(context.Background(), `UPDATE activity SET call_outcome = 'reached' WHERE id = $1`, reached); err != nil {
		t.Fatal(err)
	}
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if evidence, _, _ := e.followUpEvidence(t, deal); evidence != reached {
		t.Errorf("the task is grounded on %s, want the reached call %s", evidence, reached)
	}
}
