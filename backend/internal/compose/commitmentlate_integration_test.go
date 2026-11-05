// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A meeting typed up after the horizon is still evidence a colleague supplied
// today. Its promise is put to the promiser and, once accepted, stays on the
// Worklist past the horizon.
func TestALateLoggedMeetingPromiseIsProposedAndAcceptedWorkStaysOnTheWorklist(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE activity SET occurred_at = now() - interval '35 days' WHERE id = $1`, e.activity)

	read := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("a 35-day-old hand-logged meeting staged %d proposals, want 1 (%v)", len(read.ProposalIDs), read.StatusDetail)
	}
	if n := e.taskCount(t); n != 0 {
		t.Fatalf("the proposal wrote %d tasks before anyone accepted", n)
	}
	logged, err := e.Activities.GetActivity(e.ctx, e.activity, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if logged.CommitmentAdmissions == nil || len(*logged.CommitmentAdmissions) != 1 {
		t.Fatalf("admissions = %#v, want one", logged.CommitmentAdmissions)
	}
	if got := (*logged.CommitmentAdmissions)[0]; got.Outcome != string(CommitmentHistoricalProposed) || !got.CurrentWork {
		t.Fatalf("admission = %q current=%v, want historical_proposed held open for the card", got.Outcome, got.CurrentWork)
	}

	e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9, e.Rep1)})
	if n := e.pendingProposals(t); n != 1 {
		t.Fatalf("reading the same meeting again left %d pending proposals, want 1", n)
	}

	if _, err := e.svc.Decide(e.ctx, ids.From[ids.ApprovalKind](read.ProposalIDs[0]), true, nil); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if n := e.taskCount(t); n != 1 {
		t.Fatalf("accepting wrote %d tasks, want 1", n)
	}

	later := clocktest.Now(t).AddDate(0, 0, 60)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return later })
	day, err := feed.Worklist(e.ctx, "mine", "", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range day.Queue {
		if row.Source == "task" {
			return
		}
	}
	t.Fatal("the accepted task left the Worklist 60 days after the meeting")
}

// The rule keys on who supplied the evidence, not on the kind of activity.
// Captured sources, private mail and promises of the customer keep the
// horizon's old outcome.
func TestOnlyHandLoggedEvidenceIsProposedPastTheHorizon(t *testing.T) {
	asOf := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	old := asOf.AddDate(0, 0, -35)
	cases := []struct {
		name, kind, source string
		private            bool
		want               CommitmentOutcome
		approvals          int
	}{
		{"manual meeting", "meeting", "manual", false, CommitmentHistoricalProposed, 1},
		{"manual call", "call", "manual", false, CommitmentHistoricalProposed, 1},
		{"manual note", "note", "manual", false, CommitmentHistoricalProposed, 1},
		{"manual mail", "email", "manual", false, CommitmentHistoricalProposed, 1},
		{"imported meeting", "meeting", "import", false, CommitmentOutcome(activities.CommitmentHistorical), 0},
		{"imported note", "note", "import", false, CommitmentOutcome(activities.CommitmentHistorical), 0},
		{"private manual mail", "email", "manual", true, CommitmentOutcome(activities.CommitmentHistorical), 0},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			e := integration.Setup(t)
			body := "I will send the pricing."
			direction := "outbound"
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: test.kind, Body: &body, Direction: &direction, OccurredAt: &old, ThreadKey: "late@buyer.test", Source: test.source,
			})
			if err != nil {
				t.Fatal(err)
			}
			sourceID := ids.UUID(source.Id)
			c := Commitment{
				Meaning: openCommitmentMeaning("", body), AsOf: asOf, ReadMessages: []ids.UUID{sourceID}, Extractor: transcriptProposalActor,
				SourceActivityID: sourceID, Summary: "Send the pricing", Quote: body, Seat: &e.Rep1, Confidence: 0.95,
				Evidence: approvals.Evidence{Snippet: body, SourceType: "activity", SourceID: sourceID},
				Locator:  commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), body), Body: body,
			}
			if test.private {
				c.PrivateTo = &e.Rep1
			}
			ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
			dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
				if err == nil && outcome != test.want {
					t.Errorf("outcome = %q, want %q", outcome, test.want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval`); got != test.approvals {
				t.Errorf("approvals = %d, want %d", got, test.approvals)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != 0 {
				t.Errorf("tasks = %d, want none written without asking", got)
			}
		})
	}
}
