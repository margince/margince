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
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCommitmentMeaningControlsTheRealWriterAndSurvivesReplay(t *testing.T) {
	asOf := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	sources := map[string]string{
		"open":        "I will send the scope next week.",
		"completed":   "I sent the scope.",
		"conditional": "I will send the scope next week if the buyer agrees.",
		"optional":    "I could send the scope next week.",
		"request":     "Please send the scope next week.",
		"unclear":     "We should discuss the scope next week.",
	}
	for _, state := range commitmentMeaningStates {
		t.Run(state, func(t *testing.T) {
			e := integration.Setup(t)
			body, direction := sources[state], "outbound"
			if body == "" {
				t.Fatal("semantic state has no source fixture")
			}
			source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "email", Body: &body, Direction: &direction, OccurredAt: &asOf, Source: "manual",
			})
			if err != nil {
				t.Fatal(err)
			}
			sourceID := ids.UUID(source.Id)
			meaning := &commitmentMeaning{State: state, DeadlinePrecision: "range", DeadlineQuote: "next week"}
			if state == "completed" {
				meaning.DeadlinePrecision, meaning.DeadlineQuote = "none", ""
			}
			if state == "conditional" {
				meaning.Condition = "if the buyer agrees"
			}
			c := Commitment{
				AsOf: asOf, ReadMessages: []ids.UUID{sourceID}, Extractor: signalScanActor,
				SourceActivityID: sourceID, Summary: "Send the scope", Quote: body, Body: body,
				Seat: &e.Rep1, Confidence: 1, Meaning: meaning,
				Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), body),
			}
			ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
			dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
			want := meaning.withheldOutcome()
			if state == "open" {
				want = CommitmentTaskWritten
			}
			for pass := range 2 {
				err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
					got, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
					if err == nil && got != want && (pass != 1 || state != "open" || got != CommitmentRemembered) {
						t.Errorf("dispatch = %s, want %s", got, want)
					}
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			activity, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](sourceID), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if activity.CommitmentAdmissions == nil || len(*activity.CommitmentAdmissions) != 1 {
				t.Fatal("replay lost the single admission decision")
			}
			decision := (*activity.CommitmentAdmissions)[0]
			if decision.CurrentWork != (state == "open") || decision.RuleVersion != activities.CommitmentAdmissionVersion || decision.Outcome != string(want) {
				t.Fatalf("wrong admission: %+v", decision)
			}
			if decision.Meaning == nil || commitmentMeaning(*decision.Meaning) != *meaning {
				t.Fatalf("meaning changed: %+v", decision.Meaning)
			}
			wantTasks := 0
			if state == "open" {
				wantTasks = 1
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != wantTasks {
				t.Fatalf("tasks = %d, want %d", got, wantTasks)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM approval`); got != 0 {
				t.Fatalf("non-action staged %d approvals", got)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND due_at IS NOT NULL`); got != 0 {
				t.Fatal("a range became an exact task date")
			}
			subject := e.SeedContact(t, "Message participant", &e.Rep1)
			integration.LinkActivity(t, integration.OwnerConn(t), sourceID, "contact", subject)
			if err := privacy.NewEraser(e.DB()).WithRawCapturePurger(RawCapturePurgerFor(e.DB())).EraseContact(e.Admin(), subject, "Subject requested erasure"); err != nil {
				t.Fatal(err)
			}
			if got := e.WsCount(t, `SELECT count(*) FROM activity_commitment_admission WHERE source_activity_id=$1 AND meaning IS NOT NULL`, sourceID); got != 0 {
				t.Fatal("erasure retained source words in semantic evidence")
			}
		})
	}
}
