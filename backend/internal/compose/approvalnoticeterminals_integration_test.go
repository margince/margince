// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNoticeReconciliationRetractsDecidedExpiredAndSupersededProposals(t *testing.T) {
	for _, terminal := range []string{"decided", "expired", "superseded"} {
		t.Run(terminal, func(t *testing.T) {
			a := newApprovalNotifyEnv(t)
			a.grantRole(t, a.e.AdminUser)
			id, staged := a.stageCorrection(t)
			a.deliver(t, staged)
			switch terminal {
			case "decided":
				if _, err := a.svc.Decide(a.e.Admin(), id, false, nil); err != nil {
					t.Fatal(err)
				}
			case "expired":
				a.e.WsExec(t, `UPDATE approval SET expires_at = TIMESTAMPTZ '2000-01-01' WHERE id = @id`, pgx.StrictNamedArgs{"id": id})
				expired, err := a.svc.ExpireDue(expiryCtx(a.e))
				if err != nil || len(expired) != 1 || expired[0].ID != id {
					t.Fatalf("expiry pass=%v err=%v", expired, err)
				}
			case "superseded":
				stageRevisedNoticeCorrection(t, a)
			}
			ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
			_, changed, err := newApprovalNoticeReconcileWorker(a.e.Pool).reconcilePage(ctx, ids.Nil)
			if err != nil || changed != 1 {
				t.Fatalf("terminal notice reconciliation changed=%d err=%v", changed, err)
			}
			assertReconciledHomeNotices(t, a, 0)
			if got := a.unreadFor(t, a.e.AdminUser); got != 0 {
				t.Fatalf("terminal proposal has %d unread notices", got)
			}
		})
	}
}

func stageRevisedNoticeCorrection(t *testing.T, a approvalNotifyEnv) {
	t.Helper()
	proposal, err := json.Marshal(deals.CloseDateCorrection{
		DealID: ids.From[ids.DealKind](a.deal), ExpectedCloseDate: "2027-01-01", Basis: "The buyer confirmed a later date",
	})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := json.Marshal(map[string]string{"deal_id": a.deal.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.svc.Stage(a.proposerCtx(ids.Nil), approvals.StageInput{
		Kind: deals.CloseDateCorrectionKind, ProposedChange: proposal, DiffHash: "revised-date",
		TargetType: "deal", TargetID: a.deal, JoinPending: true, Identity: identity,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNoticeReconciliationDoesNotRestoreErasedProposalEvidence(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	a.e.WsExec(t, `UPDATE role SET permissions = jsonb_set(permissions, '{objects,activity}', '{"read":true}'::jsonb) WHERE key = @key`, pgx.StrictNamedArgs{"key": "notifydecider-" + a.e.AdminUser.String()})
	subject, activity, _ := transcriptSubject(t, a.e)
	_, staged := a.stage(a.proposerCtx(ids.Nil), t, approvals.StageInput{
		Kind: deals.CloseDateCorrectionKind, TargetType: "deal", TargetID: a.deal,
		ProposedChange: json.RawMessage(`{"deal_id":"` + a.deal.String() + `","expected_close_date":"2027-01-01","basis":"The buyer confirmed a later date"}`),
		DiffHash:       "private-quote", Summary: quotedTranscriptLine,
		Evidence: []approvals.Evidence{{Snippet: quotedTranscriptLine, SourceType: transcriptTargetType, SourceID: activity}},
	})
	a.deliver(t, staged)
	assertReconciledHomeNotices(t, a, 1)
	ctx := principal.SystemActing(a.e.Admin(), approvalNoticeRetractActor)
	if err := privacy.NewEraser(a.db).EraseContact(a.e.Admin(), subject.UUID, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}
	_, changed, err := newApprovalNoticeReconcileWorker(a.e.Pool).reconcilePage(ctx, ids.Nil)
	if err != nil || changed != 0 {
		t.Fatalf("erased proposal reconciliation changed=%d err=%v", changed, err)
	}
	if got := a.unreadFor(t, a.e.AdminUser); got != 0 {
		t.Fatalf("erased proposal has %d unread notices", got)
	}
	a.deliver(t, staged)
	assertReconciledHomeNotices(t, a, 0)
	page, err := notices.NewStore(a.db).ListFor(a.e.Admin(), 10, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("notice history count=%d, want 1", len(page.Items))
	}
	for _, notice := range page.Items {
		if count := a.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'notice' AND entity_id = @id AND action = 'erase'`, pgx.StrictNamedArgs{"id": notice.ID}); count != 1 {
			t.Fatalf("notice has %d erase tombstones, want 1", count)
		}
		if notice.Subject == quotedTranscriptLine || notice.Body != "" {
			t.Fatalf("erased text survives in notice history: %+v", notice)
		}
	}
	if got := countQuoting(t, a.e, quotedTranscriptLine); got != 0 {
		t.Fatalf("reconciliation restored evidence to %d proposals", got)
	}
}

func TestRepeatedSubjectErasureRecordsEachNoticeOnce(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	a.grantRole(t, a.e.AdminUser)
	a.e.WsExec(t, `UPDATE role SET permissions = jsonb_set(permissions, '{objects,contact}', '{"read":true,"update":true}'::jsonb) WHERE key = @key`, pgx.StrictNamedArgs{"key": "notifydecider-" + a.e.AdminUser.String()})
	subject := a.e.SeedContact(t, "Dana Buyer", &a.e.AdminUser)
	_, staged := a.stage(a.proposerCtx(ids.Nil), t, approvals.StageInput{
		Kind: "update_record", TargetType: "contact", TargetID: subject,
		ProposedChange: json.RawMessage(`{"record_type":"contact","id":"` + subject.String() + `","fields":{"title":"Buyer"}}`),
		DiffHash:       "contact-title", Summary: "Update contact title",
	})
	a.deliver(t, staged)
	assertReconciledHomeNotices(t, a, 1)
	for range 2 {
		if err := privacy.NewEraser(a.db).EraseContact(a.e.Admin(), subject, "Subject requested erasure"); err != nil {
			t.Fatal(err)
		}
	}
	page, err := notices.NewStore(a.db).ListFor(a.e.Admin(), 10, "")
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("history=%+v, %v", page, err)
	}
	if count := a.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'notice' AND entity_id = @id AND action = 'erase'`, pgx.StrictNamedArgs{"id": page.Items[0].ID}); count != 1 {
		t.Fatalf("notice has %d erase tombstones, want 1", count)
	}
}
