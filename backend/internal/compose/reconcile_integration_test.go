// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Overnight follow-up reconciliation over real migrated Postgres
// (features/07 §8a, B-E06.2a): the nightly pass turns a captured
// interaction with no next step into a STAGED follow-up proposal —
// never a silent write. After a run, the deal is untouched and the
// follow-up sits in the morning approval inbox; a human confirm creates
// it exactly once, a reject creates nothing, and a rep who cannot see
// the deal cannot see — or decide — its proposal.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose/integration"
	"github.com/gradionhq/margince/backend/internal/modules/approvals"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
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
	e.svc.WithEffect(deals.FollowUpReconcileKind, followUpConfirmEffect(e.svc, e.Activities))
	e.svc.WithPrecheck(deals.FollowUpReconcileKind, followUpPrecheck())
	// The stager production builds, drafter included. A harness that omitted
	// the drafting seam would exercise a stager that can only ever propose a
	// task, and would report the drafted-reply branch as working while nothing
	// in production reached it.
	stager := followUpStager{svc: e.svc, draft: newCommsAdapter(e.Pool, nil, SendPath{})}
	e.reconciler = deals.NewFollowUpReconciler(e.DB(), stager, quiet)
	return e
}

// reconcile runs the reconciler over this env's workspace under exactly the
// scope the follow_up_workspace worker binds.
func (e *reconcileEnv) reconcile() error {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalSystem, ID: "agent:overnight"})
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
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

func (e *reconcileEnv) pendingFollowUps(t *testing.T, dealID ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM approval WHERE kind = 'deal_follow_up' AND target_entity_id = $1 AND status = 'pending'`,
		dealID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *reconcileEnv) followUpApproval(t *testing.T, dealID ids.UUID) (ids.ApprovalID, deals.FollowUpProposal) {
	t.Helper()
	var id ids.ApprovalID
	var raw []byte
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id, proposed_change FROM approval WHERE kind = 'deal_follow_up' AND target_entity_id = $1 AND status = 'pending'`,
		dealID).Scan(&id, &raw); err != nil {
		t.Fatalf("no staged follow-up to decide: %v", err)
	}
	proposal, err := deals.UnmarshalFollowUpProposal(raw)
	if err != nil {
		t.Fatalf("staged proposal does not round-trip: %v", err)
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

// --- staging: a real touch with no next step becomes a proposal, not a write ---

func TestFollowUpReconcileStagesProposalAndCommitsNothing(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Touched, no next step", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Discovery call", 1)
	before := e.dealVersion(t, deal)

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}

	if got := e.pendingFollowUps(t, deal); got != 1 {
		t.Fatalf("pending follow-up proposals = %d, want 1", got)
	}
	// None committed: the deal is untouched and no task exists yet.
	if after := e.dealVersion(t, deal); after != before {
		t.Errorf("deal version moved %d → %d; the pass must stage, not write", before, after)
	}
	if got, _, _, _ := e.dealTasks(t, deal); got != 0 {
		t.Errorf("tasks created pre-approval = %d, want 0 (staged, not committed)", got)
	}

	// The proposal is grounded in the real interaction and dated ahead.
	_, proposal := e.followUpApproval(t, deal)
	if proposal.EvidenceActivityID.UUID != call {
		t.Errorf("evidence activity = %s, want the seeded call %s", proposal.EvidenceActivityID, call)
	}
	if proposal.EvidenceKind != "call" {
		t.Errorf("evidence kind = %q, want call", proposal.EvidenceKind)
	}
	wantDue := today().AddDate(0, 0, 3).Format(time.DateOnly)
	if proposal.DueDate != wantDue {
		t.Errorf("proposed due date = %q, want %q (today + follow-up lead)", proposal.DueDate, wantDue)
	}
}

// --- suppression: an existing next step, no real touch, and dedupe ---

func TestFollowUpReconcileSuppressesWhenNoDiscrepancy(t *testing.T) {
	e := setupReconcile(t)

	// A recent call but an OPEN task already queued: the rep has a next
	// step — do not nag.
	planned := e.SeedDeal(t, "Has next step", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, planned, "meeting", "Kickoff", 2)
	e.seedTask(t, planned, false)

	// A deal with only a note (not a call/mail/meeting): no real touch.
	noteOnly := e.SeedDeal(t, "Note only", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, noteOnly, "email", "Old thread", 24*10) // outside the 48h window

	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, planned); got != 0 {
		t.Errorf("deal with an open next step staged %d proposals, want 0", got)
	}
	if got := e.pendingFollowUps(t, noteOnly); got != 0 {
		t.Errorf("deal with no recent interaction staged %d proposals, want 0", got)
	}
}

func TestFollowUpReconcileDoesNotStackAcrossPasses(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Reconciled twice", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Call", 1)

	for pass := 0; pass < 2; pass++ {
		if err := e.reconcile(); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}
	if got := e.pendingFollowUps(t, deal); got != 1 {
		t.Errorf("after two passes, pending proposals = %d, want still 1 (no duplicate)", got)
	}
}

// --- confirm / reject: the human decision is the only write ---

func TestFollowUpConfirmCreatesTheTaskExactlyOnce(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Confirm me", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Discovery", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, proposal := e.followUpApproval(t, deal)

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
	e.seedInteraction(t, deal, "call", "Discovery", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.followUpApproval(t, deal)

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
	e.seedInteraction(t, deal, "meeting", "Sync", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.followUpApproval(t, deal)

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

// A rejection STICKS. The nightly pass runs again tomorrow over the same deal,
// and a rep who said no must not be asked the same thing a second time.
//
// The pass only ever asked whether a proposal was still PENDING, so a rejected
// one left no trace it could see: the next run staged a fresh proposal, and the
// rep's no became a daily question. That is the failure StageUnlessDeclined
// exists to prevent, and it needs a stable logical identity to recognise the
// proposal as the same one.
func TestARejectedFollowUpIsNotAskedAgainTomorrow(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Asked once", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "meeting", "Sync", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.followUpApproval(t, deal)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// Tomorrow's pass, over a deal whose situation has not changed.
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, deal); got != 0 {
		t.Errorf("after a rejection the next pass staged %d proposals, want 0 — "+
			"the rep is being asked again", got)
	}
}

// A rejection on ONE deal says nothing about another. The identity is the deal,
// so declining a follow-up must not quiet the whole pipeline — which is the way
// a too-broad identity fails, and it fails silently: proposals simply stop
// appearing and nobody can point at the moment they stopped.
func TestARejectionOnOneDealDoesNotSilenceAnother(t *testing.T) {
	e := setupReconcile(t)
	declined := e.SeedDeal(t, "Said no here", e.pipeline, e.open, &e.Rep1)
	other := e.SeedDeal(t, "Never asked", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, declined, "meeting", "Sync", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.followUpApproval(t, declined)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// The other deal now earns a proposal of its own.
	e.seedInteraction(t, other, "call", "First contact", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, other); got != 1 {
		t.Errorf("the untouched deal has %d proposals, want 1 — a rejection "+
			"elsewhere silenced it", got)
	}
	if got := e.pendingFollowUps(t, declined); got != 0 {
		t.Errorf("the declined deal has %d proposals, want 0", got)
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
	e.seedInteraction(t, deal, "meeting", "Sync", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, proposal := e.followUpApproval(t, deal)

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
	var invalid *approvals.InvalidEditError
	if !errors.As(err, &invalid) {
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
	e.seedInteraction(t, deal, "meeting", "Sync", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, proposal := e.followUpApproval(t, deal)

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

// The rejection survives a payload that has MOVED. The proposal's due date is
// computed from "today", so tomorrow's proposal is a different document — and
// supersession matches by containment of the identity, not by equality of the
// payload. If that were the other way round, the memory would last exactly one
// night and this fix would be decorative.
func TestARejectionSurvivesTheProposalChangingUnderIt(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Payload moves", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "meeting", "Sync", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, first := e.followUpApproval(t, deal)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// Move the deal's clock so the next pass computes a different due date over
	// the SAME interaction — a different document, the same question.
	e.WsExec(t, `UPDATE deal SET last_activity_at = now() - interval '2 days' WHERE id = $1`, deal)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, deal); got != 0 {
		t.Errorf("a moved payload staged %d proposals past the rejection, want 0 — "+
			"the memory is keyed on the payload rather than on the situation", got)
	}
	if first.DealID.UUID != deal {
		t.Errorf("the first proposal named deal %s, want %s", first.DealID.UUID, deal)
	}
}

// A rejection covers the conversation it answered, not the deal forever.
//
// The admit case beside the three refusals above, and the one that decides
// whether the memory is a memory or a mute button. A decline is remembered with
// no expiry, so keying it on the deal alone would let one "no" bury every later
// follow-up: the rep says no after a discovery call, has a real conversation
// weeks later that again ends with no next step, and is never asked. The
// evidence activity is what tells those apart.
func TestANewConversationIsProposedAgainAfterAnEarlierRejection(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Moved on", e.pipeline, e.open, &e.Rep1)
	// Both interactions sit inside the pass's 48h lookback; the declined one is
	// simply the older, so the later call becomes the evidence on the reruns.
	e.seedInteraction(t, deal, "meeting", "Discovery", 40)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, declined := e.followUpApproval(t, deal)
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("reject: %v", err)
	}

	// A genuinely new interaction, later than the one that was declined.
	e.WsExec(t, `UPDATE deal SET last_activity_at = now() - interval '2 days' WHERE id = $1`, deal)
	fresh := e.seedInteraction(t, deal, "call", "They called back", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	if got := e.pendingFollowUps(t, deal); got != 1 {
		t.Fatalf("a new conversation staged %d proposals, want 1 — an old rejection "+
			"is silencing a deal it was never asked about", got)
	}
	_, asked := e.followUpApproval(t, deal)
	if asked.EvidenceActivityID.UUID != fresh {
		t.Errorf("the new proposal cites interaction %s, want the fresh one %s",
			asked.EvidenceActivityID.UUID, fresh)
	}
	if asked.EvidenceActivityID == declined.EvidenceActivityID {
		t.Error("the new proposal cites the interaction that was already declined")
	}
}

// --- row scope: a proposal never leaks a deal the decider cannot see ---

func TestFollowUpProposalRespectsRowScope(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Rep1's deal", e.pipeline, e.open, &e.Rep1)
	e.seedInteraction(t, deal, "call", "Private call", 1)
	if err := e.reconcile(); err != nil {
		t.Fatal(err)
	}
	approvalID, _ := e.followUpApproval(t, deal)

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
