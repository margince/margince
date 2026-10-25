// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestTranscriptAdmissionProposesLateLogsAndFutureDeadlines(t *testing.T) {
	for _, future := range []bool{false, true} {
		t.Run(map[bool]string{false: "late log", true: "future deadline"}[future], func(t *testing.T) {
			e := setupTranscript(t)
			now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
			old := now.AddDate(-2, 0, 0)
			if _, err := e.Activities.UpdateActivity(e.ctx, e.activity, activities.UpdateActivityInput{OccurredAt: &old}); err != nil {
				t.Fatal(err)
			}
			due := ""
			if future {
				due = "2026-11-20"
			}
			payload, err := json.Marshal(map[string]any{"proposals": []map[string]any{{"summary": "Send revised pricing", "owner": "Priya", "promiser_ref": "seat:" + e.Rep1.String(), "source_lines": []int{3}, "confidence": 0.9, "due_date": due, "commitment": openCommitmentMeaning(due, "Friday")}}})
			if err != nil {
				t.Fatal(err)
			}
			proposer := NewTranscriptProposer(e.Pool, cannedBrain{reply: string(payload)}, e.svc, func() time.Time { return now }, slog.Default())
			for range 2 {
				started, _, err := e.Activities.StartTranscriptReadQueued(e.ctx, e.activity, "human:"+e.Rep1.String(), nil)
				if err != nil {
					t.Fatal(err)
				}
				worker := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), started.ID)
				if err := proposer.Read(worker, e.Activities, started.ID, e.activity); err != nil {
					t.Fatal(err)
				}
				done, err := e.Activities.GetTranscriptRead(e.ctx, e.activity, started.ID)
				if err != nil {
					t.Fatal(err)
				}
				if done.Status != activities.TranscriptReadDone {
					t.Fatalf("read = %#v", done)
				}
			}
			source, err := e.Activities.GetActivity(e.ctx, e.activity, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if source.CommitmentAdmissions == nil || len(*source.CommitmentAdmissions) != 1 {
				t.Fatalf("decisions = %#v", source.CommitmentAdmissions)
			}
			decision := (*source.CommitmentAdmissions)[0]
			wantOutcome := CommitmentHistoricalProposed
			if future {
				wantOutcome = CommitmentProposed
			}
			if !decision.CurrentWork || decision.Outcome != string(wantOutcome) {
				t.Fatalf("decision = %#v, want %s held open for the card", decision, wantOutcome)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval`); got != 1 {
				t.Fatalf("proposals = %d, want 1", got)
			}
			if e.taskCount(t) != 0 {
				t.Fatal("a promise its colleague did not say aloud became an automatic task")
			}
		})
	}
}
