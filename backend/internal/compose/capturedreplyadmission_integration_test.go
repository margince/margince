// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCapturedMailNeedsConfirmedResponsibilityBeforeItAsksForAReply(t *testing.T) {
	for _, tc := range []struct {
		name, body, label, verdict string
		owed                       bool
	}{
		{"newsletter", "Here is our monthly newsletter. No action is requested.", "noise", activities.OwedVerdictInformsUs, false},
		{"customer promise", "I will send our forecast volumes tomorrow.", "commitment", activities.OwedVerdictInformsUs, false},
		{"buyer request", "Please send the proposal for our integration project.", "commitment", activities.OwedVerdictAsksUs, true},
		{"conflicting noise", "Message with conflicting automatic judgments.", "noise", activities.OwedVerdictAsksUs, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := setupOwed(t)
			o.contact(t, "Buyer", "buyer@customer.example")
			message := o.capture(t, owedMail{from: "buyer@customer.example", to: o.seat, subject: tc.name, body: tc.body, at: o.now.Add(-time.Hour), messageID: ids.NewV7().String() + "@customer.example"})
			if _, err := o.e.Activities.SetCaptureLabel(o.judge(), message, tc.label); err != nil {
				t.Fatal(err)
			}
			if o.owed(t, message) {
				t.Fatal("unjudged captured mail manufactured a reply obligation")
			}
			brain := &owedBrainStub{verdict: tc.verdict, confidence: .99}
			runOwedWorker(t, o.e, brain)
			if len(brain.prompts) == 0 {
				t.Fatal("unjudged captured mail disappeared from the classifier backlog")
			}
			if got := verdictOf(t, o.e, message); got == nil || *got != tc.verdict {
				t.Fatalf("verdict=%v", got)
			}
			if got := o.badged(t, message); got != tc.owed {
				t.Fatalf("reply obligation=%v want=%v", got, tc.owed)
			}
			if tc.owed {
				assertCapturedRequestTask(t, o, message)
			} else if o.waiting(t, message) {
				t.Fatal("non-request entered the waiting lane")
			}
			if tc.name == "conflicting noise" {
				title := "Send the requested figures"
				if _, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{Kind: "task", RequestActivityID: &message, Subject: &title, Source: "manual"}); err != nil {
					t.Fatal(err)
				}
				if !o.badged(t, message) {
					t.Fatal("explicit human acceptance did not override noise")
				}
			}
		})
	}
}

// Answer evidence remains useful for settlement even before intent is judged.
func (o *owedEnv) unanswered(t *testing.T, message ids.UUID) bool {
	t.Helper()
	var answers []activities.OffThreadAnswer
	ctx := o.judge()
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		answers, err = activities.OffThreadAnswersTx(ctx, tx, message, o.now, 1500, 100)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return len(answers) == 0
}

func assertCapturedRequestTask(t *testing.T, o *owedEnv, message ids.UUID) {
	t.Helper()
	var task ids.UUID
	if err := database.WithWorkspaceTx(o.reader(), o.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(o.reader(), `SELECT id FROM activity WHERE source_system=$1 AND source_activity_id=$2 AND archived_at IS NULL`, activities.EmailRequestTaskSource, message).Scan(&task)
	}); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(o.e.Pool, approvals.NewService(o.e.DB()), func() time.Time { return o.now })
	page, err := feed.Worklist(o.reader(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.Summary.Total != 1 || len(page.Queue) != 1 || page.Queue[0].Source != "task" || page.Queue[0].Id != task.String() {
		t.Fatalf("confirmed request must have one actionable task: %+v", page)
	}
	if o.waiting(t, message) {
		t.Fatal("materialized request also remains in the waiting lane")
	}
}
