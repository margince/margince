// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// promisedDecision is one proposed commitment task waiting on the admin, its
// open promise, the note the promise was read from, and the notice about it.
type promisedDecision struct {
	approval ids.UUID
	claim    ids.UUID
	source   ids.UUID
	notice   ids.UUID
	summary  string
}

func stagePromisedDecision(t *testing.T, a approvalNotifyEnv, seat ids.UUID, summary string) promisedDecision {
	t.Helper()
	quote := "We will " + strings.ToLower(summary) + "."
	note, _, err := a.e.Activities.LogActivity(a.e.Admin(), activities.LogActivityInput{Kind: "note", Body: &quote, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	source := ids.UUID(note.Id)
	contact := ids.From[ids.ContactKind](a.e.SeedContact(t, summary+" contact", &a.e.AdminUser))
	claim := a.e.RecordClaim(a.e.Admin(), t, contacts.ClaimInput{ContactID: contact, Kind: "commitment_ours", Body: summary, ActivityID: source, Quote: quote, Source: "manual", Locator: summary})
	claimID := ids.UUID(claim.Id)
	body, err := json.Marshal(CommitmentTaskProposal{
		SourceActivityID: source, Summary: summary, Quote: quote, SeatID: &seat, PromiserSeatID: &seat,
		Locator: summary, ClaimID: &claimID,
	})
	if err != nil {
		t.Fatal(err)
	}
	id, env := a.stage(a.proposerCtx(seat), t, approvals.StageInput{
		Kind: CommitmentTaskKind, ProposedChange: body, DiffHash: summary,
		TargetType: string(recordTypeActivity), TargetID: source, Summary: summary,
	})
	a.deliver(t, env)
	decision := promisedDecision{approval: id.UUID, claim: claimID, source: source, summary: summary}
	for _, n := range a.delivered(t) {
		if n.subject == summary {
			decision.notice = n.id
		}
	}
	if decision.notice.IsZero() {
		t.Fatalf("no notice was written for %q", summary)
	}
	return decision
}

func newDecisionMailEnv(t *testing.T) (approvalNotifyEnv, *countingMailer, *notificationMailWorker) {
	t.Helper()
	a := newApprovalNotifyEnv(t)
	a.grantCommitmentRole(t, a.e.AdminUser)
	a.grantCommitmentRole(t, a.e.Rep1)
	relay := &countingMailer{}
	worker := newNotificationMailWorker(a.e.Pool, NotificationMailConfig{Mailer: relay, PublicBaseURL: noticeMailOrigin},
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	return a, relay, worker
}

func mailNotice(t *testing.T, a approvalNotifyEnv, worker *notificationMailWorker, notice ids.UUID) {
	t.Helper()
	if err := worker.Work(context.Background(), &river.Job[SendNotificationEmailArgs]{
		Args: SendNotificationEmailArgs{Workspace: a.e.WS, NoticeID: notice.String()},
	}); err != nil {
		t.Fatalf("the notification mail worker refused the job: %v", err)
	}
}

func sentWithout(t *testing.T, relay *countingMailer, kept, gone string) {
	t.Helper()
	all := strings.Join(relay.bodies, "\n")
	if strings.Contains(all, gone) {
		t.Fatalf("a decision whose basis is gone was mailed:\n%s", all)
	}
	if !strings.Contains(all, kept) {
		t.Fatalf("the decision still waiting was not mailed:\n%s", all)
	}
}

// A promise settled after its decision was queued takes that decision out of
// the mail; the other decision still waiting is mailed.
func TestADecisionWhosePromiseWasSettledIsNotMailedWhileTheOtherIs(t *testing.T) {
	a, relay, worker := newDecisionMailEnv(t)
	gone := stagePromisedDecision(t, a, a.e.AdminUser, "Send the pricing")
	kept := stagePromisedDecision(t, a, a.e.AdminUser, "Send the contract")
	if err := a.e.Contacts.SettleConversationClaim(a.e.Admin(), gone.claim, "done"); err != nil {
		t.Fatal(err)
	}
	mailNotice(t, a, worker, gone.notice)
	mailNotice(t, a, worker, kept.notice)
	sentWithout(t, relay, kept.summary, gone.summary)
}

// A source the decider may no longer open takes its decision out of the mail.
func TestADecisionWhoseSourceTheDeciderMayNoLongerOpenIsNotMailed(t *testing.T) {
	a, relay, worker := newDecisionMailEnv(t)
	gone := stagePromisedDecision(t, a, a.e.Rep1, "Send the pricing")
	kept := stagePromisedDecision(t, a, a.e.Rep1, "Send the contract")
	if _, err := a.e.Activities.SetAudience(a.e.Admin(), ids.From[ids.ActivityKind](gone.source), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: a.e.AdminUser}},
	}); err != nil {
		t.Fatal(err)
	}
	mailNotice(t, a, worker, gone.notice)
	mailNotice(t, a, worker, kept.notice)
	sentWithout(t, relay, kept.summary, gone.summary)
}

// One decision shown in a morning's body and quoted by its digest is one row
// of the basis. The basis takes it once, excludes nothing, and holds.
func TestOneDecisionInTheBodyAndTheDigestIsOneBasisRow(t *testing.T) {
	a, _, _ := newDecisionMailEnv(t)
	decision := stagePromisedDecision(t, a, a.e.AdminUser, "Send the pricing")
	items := []morningItem{{source: "approval", id: decision.approval.String()}}
	digest := digestLines{total: 1, quotable: []digestItem{{decision: true, target: notices.Target{Type: notices.TargetApproval, ID: decision.approval}}}}
	reader := a.e.As(a.e.AdminUser, nil, integration.AdminPerms)
	var basis sendBasis
	if err := database.WithWorkspaceTx(reader, a.e.Pool, func(tx pgx.Tx) error {
		var err error
		if basis, err = readSendBasis(reader, tx, a.e.AdminUser, nil, morningRefs(items, digest), clocktest.Now(t)); err != nil {
			return err
		}
		return holdSendBasisTx(reader, tx, basis, func() time.Time { return clocktest.Now(t) })
	}); err != nil {
		t.Fatalf("one decision in both parts refused the reservation: %v", err)
	}
	decisions := 0
	for _, row := range basis.rows {
		if row.table == "approval" {
			decisions++
		}
	}
	if decisions != 1 || len(basis.excluded) != 0 {
		t.Fatalf("basis = %+v, want the decision once and nothing excluded", basis)
	}
}
