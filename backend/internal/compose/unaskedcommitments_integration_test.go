// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A commitment request that reached a colleague who did not make the promise
// is taken back. The reconcile sweep retracts the notice, a queued email is
// dropped when sent, and the withdrawal data task closes the request.

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// commitmentDeciderGrants is what deciding a commitment proposal takes.
const commitmentDeciderGrants = `{"objects":{"activity":{"read":true,"create":true},"contact":{"read":true,"update":true}},"row_scope":"all"}`

func (a approvalNotifyEnv) grantCommitmentRole(t *testing.T, user ids.UUID) {
	t.Helper()
	key := "commitmentdecider-" + user.String()
	a.e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Commitment decider', $2::jsonb)`,
		key, commitmentDeciderGrants)
	a.e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		SELECT r.id, $1 FROM role r WHERE r.key = $2`, user, key)
}

// stageCommitmentFor stages a promise its colleague made through the real
// staging path, and tells everybody the notifier tells.
func (a approvalNotifyEnv) stageCommitmentFor(t *testing.T, seat, source ids.UUID, locator string) ids.ApprovalID {
	t.Helper()
	body, err := json.Marshal(CommitmentTaskProposal{
		SourceActivityID: source, Summary: "Send the pricing", Quote: "We will send the pricing.", SeatID: &seat, PromiserSeatID: &seat, Locator: locator,
	})
	if err != nil {
		t.Fatal(err)
	}
	id, env := a.stage(a.proposerCtx(seat), t, approvals.StageInput{
		Kind: CommitmentTaskKind, ProposedChange: body, DiffHash: locator,
		TargetType: string(recordTypeActivity), TargetID: source, Summary: "Send the pricing",
	})
	a.deliver(t, env)
	// The dispatcher records the decision a proposal rests on beside it.
	decision := crmcontracts.CommitmentAdmission{
		Locator: locator, Summary: "Send the pricing", SourceQuote: "We will send the pricing.",
		Outcome: string(CommitmentProposed), CurrentWork: true, RuleVersion: activities.CommitmentAdmissionVersion,
		EvidenceIds: []crmcontracts.Id{crmcontracts.Id(source)}, EvaluatedAt: clocktest.Now(t),
		Meaning: &crmcontracts.CommitmentMeaning{State: "open", DeadlinePrecision: "none"},
	}
	if err := a.e.DB().Tx(a.e.Admin(), func(tx pgx.Tx) error {
		return activities.RecordCommitmentAdmissionTx(a.e.Admin(), tx, ids.From[ids.ActivityKind](source), decision)
	}); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestARequestSentToTheWrongColleagueIsTakenBack(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantCommitmentRole(t, a.e.Rep1)
	a.grantCommitmentRole(t, a.e.Rep2)
	note, _, err := a.e.Activities.LogActivity(a.e.Admin(), activities.LogActivityInput{
		Kind: "note", Body: new("We will send the pricing."), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(note.Id)
	kept := a.stageCommitmentFor(t, a.e.Rep1, source, "kept")
	wrong := a.stageCommitmentFor(t, a.e.Rep1, source, "wrong")
	if got := a.unreadFor(t, a.e.Rep1); got != 2 {
		t.Fatalf("the promiser has %d notices, want one per request", got)
	}
	if got := a.unreadFor(t, a.e.Rep2); got != 0 {
		t.Fatalf("a colleague who made no promise has %d notices", got)
	}
	// The shape the dispatcher wrote for private mail before every request had
	// to name its promiser: the mailbox owner was asked for a colleague's promise.
	a.e.WsExec(t, `UPDATE approval SET proposed_change = jsonb_set(proposed_change, '{promiser_seat_id}', to_jsonb($2::text))
		 WHERE id = $1`, wrong, a.e.Rep2.String())

	recipient := a.e.As(a.e.Rep1, nil, integration.AdminPerms)
	for id, sent := range map[ids.ApprovalID]bool{kept: true, wrong: false} {
		attempt := notices.EmailAttempt{ApprovalID: &id, Notice: notices.Notice{Kind: notices.KindApprovalPending}, Recipient: ids.From[ids.UserKind](a.e.Rep1)}
		if _, _, send, err := refreshApprovalNotice(recipient, a.e.Pool, attempt, clocktest.Now(t), forSingleEmail); err != nil || send != sent {
			t.Errorf("the queued email for %s: send=%v err=%v, want send=%v", id, send, err, sent)
		}
	}

	sweep := principal.SystemActing(a.e.Admin(), approvalNoticeReconcileActor)
	if _, changed, err := newApprovalNoticeReconcileWorker(a.e.Pool).reconcilePage(sweep, ids.Nil); err != nil || changed != 1 {
		t.Fatalf("the reconcile sweep took back %d notices (err %v), want the one for the wrong request", changed, err)
	}
	if got := a.unreadFor(t, a.e.Rep1); got != 1 {
		t.Fatalf("the promiser has %d notices after the sweep, want the one for their own request", got)
	}

	if changed := withdrawUnaskedCommitments(t, a.e); changed != 1 {
		t.Fatalf("the data task withdrew %d requests, want the one sent to the wrong colleague", changed)
	}
	if changed := withdrawUnaskedCommitments(t, a.e); changed != 0 {
		t.Fatalf("a second run withdrew %d more requests", changed)
	}
	if status := a.e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, wrong); status != "expired" {
		t.Errorf("the wrong request is %s, want withdrawn", status)
	}
	if status := a.e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, kept); status != "pending" {
		t.Errorf("the promiser's own request is %s, want pending", status)
	}
	if n := a.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'approval' AND entity_id = $1
		 AND evidence->>'reason' = $2`, wrong, "the colleague who made this promise cannot hold the task"); n != 1 {
		t.Errorf("%d audit rows record the withdrawal, want one", n)
	}
	if n := a.e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'approval.decided'
		 AND envelope->'entity'->>'id' = $1`, wrong.String()); n != 0 {
		t.Errorf("the withdrawal sent %d decided events, so webhooks hear of a request that was never theirs", n)
	}
}

// withdrawUnaskedCommitments runs the withdrawal step to the end, one row a
// batch, and returns how many requests it closed.
func withdrawUnaskedCommitments(t *testing.T, e *integration.Env) int {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
	changed := 0
	for range 100 {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = approvals.WithdrawUnaskedCommitmentsTx(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		changed += out.Changed
		if out.Done {
			return changed
		}
		in.Cursor = out.Cursor
	}
	t.Fatal("the withdrawal did not finish")
	return changed
}

// The staging engine refuses a commitment that is not for the colleague who
// made it, before it can join, supersede or remember another member's row.
func TestACommitmentIsStagedOnlyForItsPromiser(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	source := ids.NewV7()
	for name, staged := range map[string]struct {
		stager     ids.UUID
		seat, made ids.UUID
	}{
		"for a colleague who did not make the promise": {a.e.Rep1, a.e.Rep1, a.e.Rep2},
		"on behalf of another colleague":               {a.e.Rep2, a.e.Rep1, a.e.Rep1},
		"for nobody":                                   {ids.Nil, ids.Nil, ids.Nil},
	} {
		t.Run(name, func(t *testing.T) {
			proposal := CommitmentTaskProposal{SourceActivityID: source, Summary: "Send the pricing", PromiserSeatID: &staged.made, Locator: name}
			if !staged.seat.IsZero() {
				proposal.SeatID = &staged.seat
			}
			body, err := json.Marshal(proposal)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := a.svc.Stage(a.proposerCtx(staged.stager), approvals.StageInput{
				Kind: CommitmentTaskKind, ProposedChange: body, DiffHash: name,
				TargetType: string(recordTypeActivity), TargetID: source, Summary: "Send the pricing",
			}); err == nil {
				t.Error("the commitment was staged")
			}
		})
	}
	if n := a.e.WsCount(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Errorf("%d requests written for promises staged for the wrong colleague", n)
	}
}

// A request staged while its promiser had no seat yet is the same member's
// request as one naming their seat. Restaging the promise replaces it.
func TestARequestWithoutASeatIsReplacedByTheSamePromisersNext(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	source := ids.NewV7()
	identity := json.RawMessage(`{"locator":"same-promise"}`)
	for i, seat := range []*ids.UUID{nil, &a.e.Rep1} {
		body, err := json.Marshal(CommitmentTaskProposal{
			SourceActivityID: source, Summary: "Send the pricing", SeatID: seat, PromiserSeatID: &a.e.Rep1,
			Locator: "same-promise", DueDate: []string{"", "2027-01-01"}[i],
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.svc.Stage(a.proposerCtx(a.e.Rep1), approvals.StageInput{
			Kind: CommitmentTaskKind, ProposedChange: body, DiffHash: string(rune('a' + i)), Identity: identity, JoinPending: true,
			TargetType: string(recordTypeActivity), TargetID: source, Summary: "Send the pricing",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if n := a.e.WsCount(t, `SELECT count(*) FROM approval WHERE status = 'pending' AND expires_at > now()`); n != 1 {
		t.Errorf("%d requests wait for one promise, want the newer one alone", n)
	}
}
