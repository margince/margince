// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The approval fan-out against a real roster.
//
// Every claim here is a database fact — which seats the roster admits, what
// each seat's STORED role resolves to, and whether the approval row is still
// pending when the envelope is consumed — and none of them can be asserted
// against hand-built principals. A suite that bound its own permissions onto a
// context would be checking its own fixture, which is how the partial copy of
// this predicate in webhooks drifted from the inbox without anything failing.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealDeciderGrants is what deciding a close-date correction costs: the deal's
// own update verb, plus the read the target probe applies above it. Spelled as
// the role document an installation would write, because that is what
// EffectiveAuthority reads — a permission set bound onto a context never
// reaches this consumer.
const dealDeciderGrants = `{"objects":{"deal":{"read":true,"update":true}},"row_scope":"all"}`

// stagedSummary is the sentence the card carries, and the subject the notice
// has to arrive with. A literal rather than a derivation: the assertion is that
// the fan-out repeats what was staged, and a computed expectation could agree
// with a bug in both places.
const stagedSummary = "The close date on Fleet renewal has passed"

// approvalNotifyEnv is the seeded world one fan-out case runs in.
type approvalNotifyEnv struct {
	e      *integration.Env
	db     *database.DB
	svc    *approvals.Service
	notify *ApprovalNotify
	queue  *recordingNoticeMailQueue
	deal   ids.UUID
}

func newApprovalNotifyEnv(t *testing.T) approvalNotifyEnv {
	t.Helper()
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	queue := &recordingNoticeMailQueue{}
	a := approvalNotifyEnv{
		e: e, db: db, svc: approvals.NewService(db), queue: queue,
		notify: NewApprovalNotify(e.Pool, db, queue),
	}
	a.deal = a.seedDeal(t)
	return a
}

// stagedMailJob is one enqueue as the lane made it.
//
// BOTH HALVES, and the options are the half that has to be here: this kind is
// opts_owner: caller, so the call site is the only place its queue and its
// attempt ladder are written down. A double that took the options and dropped
// them would leave that number asserted by nothing at all.
type stagedMailJob struct {
	args SendNotificationEmailArgs
	opts *river.InsertOpts
}

// recordingNoticeMailQueue stands in for River's insert. The durable queue is a
// true boundary, which is what makes a fake the right shape here: what this
// suite asserts is WHICH notices get a mail job staged and under what options,
// and River's own insert is proven by the kinds the census walks.
type recordingNoticeMailQueue struct {
	mu   sync.Mutex
	jobs []stagedMailJob
}

func (q *recordingNoticeMailQueue) EnqueueTx(
	_ context.Context, _ pgx.Tx, args river.JobArgs, opts *river.InsertOpts,
) error {
	staged, mine := args.(SendNotificationEmailArgs)
	if !mine {
		return fmt.Errorf("the notify lane staged a %s, which is not its to stage", args.Kind())
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.jobs = append(q.jobs, stagedMailJob{args: staged, opts: opts})
	return nil
}

func (q *recordingNoticeMailQueue) staged() []stagedMailJob {
	q.mu.Lock()
	defer q.mu.Unlock()
	return slices.Clone(q.jobs)
}

// seedDeal builds the record a staged correction points at, through the deal
// store's own doors: the target probe asks that store's read rule, so a row
// inserted around it would be proving the probe against a deal production never
// writes.
func (a approvalNotifyEnv) seedDeal(t *testing.T) ids.UUID {
	t.Helper()
	ctx := a.e.Admin()
	pipeline, err := a.e.Deals.CreatePipeline(ctx, deals.CreatePipelineInput{Name: "Sales"})
	if err != nil {
		t.Fatalf("seeding the pipeline: %v", err)
	}
	pipelineID := ids.From[ids.PipelineKind](ids.UUID(pipeline.Id))
	stage, err := a.e.Deals.CreateStage(ctx, deals.CreateStageInput{
		PipelineID: pipelineID, Name: "Qualified", Position: 1, Semantic: "open"})
	if err != nil {
		t.Fatalf("seeding the stage: %v", err)
	}
	owner := ids.From[ids.UserKind](a.e.AdminUser)
	deal, err := a.e.Deals.CreateDeal(ctx, deals.CreateDealInput{
		Name: "Fleet renewal", PipelineID: pipelineID,
		StageID: ids.From[ids.StageKind](ids.UUID(stage.Id)), OwnerID: &owner, Source: "manual"})
	if err != nil {
		t.Fatalf("seeding the deal: %v", err)
	}
	return ids.UUID(deal.Id)
}

// grantRole gives one seat REAL stored grants. The consumer resolves every
// seat's authority from role_assignment through EffectiveAuthority, so a seat
// with no role resolves to no grants — and a suite that forgot this would watch
// the admit case fail in exactly the shape a working refusal has.
func (a approvalNotifyEnv) grantRole(t *testing.T, user ids.UUID, permissions string) {
	t.Helper()
	// The whole id, not a prefix: these seats are minted as UUIDv7 in one
	// breath, so any prefix short enough to read is the same for all of them.
	key := "notifydecider-" + user.String()
	a.e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Notify decider', $2::jsonb)`,
		key, permissions)
	a.e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		SELECT r.id, $1 FROM role r WHERE r.key = $2`, user, key)
}

// proposerCtx is the principal a server-side sweep stages under. onBehalfOf is
// the seat the row is staged FOR, which is what the self-only narrowing reads;
// the zero value stages a proposal that belongs to the shared inbox.
func (a approvalNotifyEnv) proposerCtx(onBehalfOf ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), a.e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:test-proposer", OnBehalfOf: onBehalfOf,
	})
}

// stageCorrection stages one close-date correction against the seeded deal —
// the shared-inbox kind — through the real staging path, and hands back the
// envelope the outbox recorded for it.
func (a approvalNotifyEnv) stageCorrection(t *testing.T) (ids.ApprovalID, kevents.Envelope) {
	t.Helper()
	// Marshalled from the module's own type, never hand-written JSON: a literal
	// that drifted from it would stage a payload the product never produces.
	proposal, err := json.Marshal(deals.CloseDateCorrection{
		DealID:            ids.From[ids.DealKind](a.deal),
		ExpectedCloseDate: "2026-12-01",
		Basis:             "the date has passed and the deal is still open",
	})
	if err != nil {
		t.Fatalf("marshalling the proposal: %v", err)
	}
	return a.stage(t, a.proposerCtx(ids.Nil), approvals.StageInput{
		Kind:           deals.CloseDateCorrectionKind,
		ProposedChange: proposal,
		DiffHash:       "h-" + a.deal.String(),
		TargetType:     "deal",
		TargetID:       a.deal,
		Summary:        stagedSummary,
	})
}

func (a approvalNotifyEnv) stage(t *testing.T, ctx context.Context, in approvals.StageInput) (ids.ApprovalID, kevents.Envelope) {
	t.Helper()
	id, err := a.svc.Stage(ctx, in)
	if err != nil {
		t.Fatalf("staging a %s: %v", in.Kind, err)
	}
	var env kevents.Envelope
	a.read(t, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'approval.requested'
			   AND envelope->'entity'->>'id' = $1`, id.String()).Scan(&env)
	})
	return id, env
}

// deliver hands one envelope to the consumer the way the subscriber would.
func (a approvalNotifyEnv) deliver(t *testing.T, env kevents.Envelope) {
	t.Helper()
	if err := a.notify.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("the fan-out refused the envelope: %v", err)
	}
}

// read runs one workspace-bound read over the fixture's own tables.
func (a approvalNotifyEnv) read(t *testing.T, fn func(context.Context, pgx.Tx) error) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), a.e.WS)
	if err := database.WithWorkspaceTx(ctx, a.e.Pool, func(tx pgx.Tx) error {
		return fn(ctx, tx)
	}); err != nil {
		t.Fatalf("reading the fixture's tables: %v", err)
	}
}

// deliveredNotice is one recorded line, read straight from the table rather
// than through the recipient's own inbox: the claim is what was WRITTEN, and a
// read filtered by the reader could hide a notice addressed to the wrong seat.
type deliveredNotice struct {
	id         ids.UUID
	recipient  ids.UUID
	kind       string
	subject    string
	dedupeKey  string
	targetType *string
	targetID   *ids.UUID
}

func (a approvalNotifyEnv) delivered(t *testing.T) []deliveredNotice {
	t.Helper()
	var out []deliveredNotice
	a.read(t, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id, recipient_user_id, kind, subject,
			coalesce(dedupe_key, ''), target_type, target_id
			 FROM notice ORDER BY recipient_user_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n deliveredNotice
			if err := rows.Scan(&n.id, &n.recipient, &n.kind, &n.subject,
				&n.dedupeKey, &n.targetType, &n.targetID); err != nil {
				return err
			}
			out = append(out, n)
		}
		return rows.Err()
	})
	return out
}

func TestApprovalNotifyReachesTheSeatsThatCouldDecideAndNobodyElse(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser, dealDeciderGrants)
	// The deactivated manager holds the SAME grants as the admin, so the only
	// thing keeping them off the fan-out is the roster's liveness filter. A
	// fixture that left them ungranted would pass with no filter at all.
	a.grantRole(t, a.e.Rep3, dealDeciderGrants)
	a.e.WsExec(t, `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, a.e.Rep3)

	approvalID, env := a.stageCorrection(t)
	a.deliver(t, env)

	rows := a.delivered(t)
	if len(rows) != 1 {
		t.Fatalf("one seat could decide this; %d notice(s) were written: %+v", len(rows), rows)
	}
	got := rows[0]
	if got.recipient != a.e.AdminUser {
		t.Fatalf("the notice went to %s, not to the one seat holding the grant", got.recipient)
	}
	if got.kind != notices.KindApprovalPending {
		t.Fatalf("notice kind %q, want %q", got.kind, notices.KindApprovalPending)
	}
	if want := "approval_pending:" + approvalID.String(); got.dedupeKey != want {
		t.Fatalf("dedupe key %q, want %q", got.dedupeKey, want)
	}
	if got.subject != stagedSummary {
		t.Fatalf("subject %q does not carry the staged summary %q", got.subject, stagedSummary)
	}
	if got.targetType == nil || *got.targetType != "deal" || got.targetID == nil || *got.targetID != a.deal {
		t.Fatalf("the notice does not point at the staged deal: %+v", got)
	}

	// Redelivery is the bus's normal behaviour, and the dedupe key is what makes
	// it free: the same line, not a second one.
	a.deliver(t, env)
	replayed := a.delivered(t)
	if len(replayed) != 1 || replayed[0].id != got.id {
		t.Fatalf("a replayed envelope wrote a second notice: %+v", replayed)
	}
}

func TestApprovalNotifySaysNothingAboutAnApprovalThatStoppedBeingPending(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser, dealDeciderGrants)
	approvalID, env := a.stageCorrection(t)

	// Withdrawal writes the terminal status and NO event, so the envelope in
	// hand still says pending. Re-reading the row at consume time is the only
	// thing between that stale envelope and a notice about a card nobody can
	// answer.
	ctx := a.proposerCtx(ids.Nil)
	if err := a.db.Tx(ctx, func(tx pgx.Tx) error {
		retracted, err := a.svc.WithdrawInTx(ctx, tx, approvalID, "superseded by a fresher reading")
		if err != nil {
			return err
		}
		if !retracted {
			t.Fatal("the staged row was not live to withdraw; the fixture proves nothing")
		}
		return nil
	}); err != nil {
		t.Fatalf("withdrawing the proposal: %v", err)
	}

	a.deliver(t, env)
	if rows := a.delivered(t); len(rows) != 0 {
		t.Fatalf("a withdrawn approval was announced to %d seat(s): %+v", len(rows), rows)
	}
}

func TestApprovalNotifyWritesNothingForASeatWhoSwitchedApprovalsOff(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser, dealDeciderGrants)
	// A second decider who changed nothing, so the silence below is the
	// preference and not the fan-out failing to reach anybody at all.
	a.grantRole(t, a.e.Rep2, dealDeciderGrants)
	// Through the seat's own writer, not an INSERT: the row a hand-written
	// fixture produces is one the product may never write.
	seatCtx := a.e.As(a.e.AdminUser, nil, integration.AdminPerms)
	if _, err := notices.NewStore(a.db).SaveNotificationPreference(
		seatCtx, notices.ClassApprovalPending, notices.DeliveryOff); err != nil {
		t.Fatalf("switching the class off: %v", err)
	}

	_, env := a.stageCorrection(t)
	a.deliver(t, env)
	rows := a.delivered(t)
	if len(rows) != 1 || rows[0].recipient != a.e.Rep2 {
		t.Fatalf("want the one line for the seat who left the class on; got %+v", rows)
	}
}

func TestApprovalNotifyOnASelfOnlyKindReachesOnlyTheSeatItWasStagedFor(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	// A step-up needs no object grant at all — the lender is the whole of its
	// authority — so the admin here is the widest seat in the workspace and
	// still must not hear about it.
	a.grantRole(t, a.e.AdminUser, dealDeciderGrants)

	_, env := a.stage(t, a.proposerCtx(a.e.Rep1), approvals.StageInput{
		Kind:           approvals.KindVolumeRelease,
		ProposedChange: json.RawMessage(`{"passport_label":"Nightly research"}`),
		DiffHash:       "h-stepup-" + a.e.Rep1.String(),
		Summary:        "Nightly research is asking to keep reading",
	})
	a.deliver(t, env)

	rows := a.delivered(t)
	if len(rows) != 1 {
		t.Fatalf("a step-up is one seat's to answer; %d notice(s) were written: %+v", len(rows), rows)
	}
	if rows[0].recipient != a.e.Rep1 {
		t.Fatalf("the step-up reached %s rather than the seat that lent the passport", rows[0].recipient)
	}
	if rows[0].targetType != nil || rows[0].targetID != nil {
		t.Fatalf("a staging about no record was given a target: %+v", rows[0])
	}
}
