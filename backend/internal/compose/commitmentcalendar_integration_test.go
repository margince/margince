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
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestCancellingOneCalendarOccurrenceLeavesTheHeldMeetingsPromiseCurrent(t *testing.T) {
	e := integration.Setup(t)
	asOf := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	ctx := withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7())
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	for _, test := range []struct {
		name   string
		start  time.Time
		cancel bool
		want   CommitmentOutcome
	}{
		{"monthly-held", asOf.AddDate(0, 0, -2), false, CommitmentTaskWritten},
		{"monthly-future", asOf.AddDate(0, 0, 12), true, CommitmentOutcome(activities.CommitmentMeetingCancelled)},
	} {
		key := connector.NaturalKey{SourceSystem: calendarSystem, SourceID: test.name}
		source, err := calendarSink(e).Upsert(calendarOwnerCtx(e, e.Rep1), connector.NormalizedRecord{
			EntityType: "activity", NaturalKey: key,
			Fields: capture.ActivityFields{Kind: "meeting", Subject: "Monthly pricing review", Body: "I will send the pricing.", OccurredAt: test.start},
			Source: calendarSystem + ":" + test.name, CapturedBy: "connector:" + calendarSystem, Raw: []byte(`{"series":"monthly"}`),
		})
		if err != nil {
			t.Fatal(err)
		}
		if test.cancel {
			if err := calendarSink(e).CancelMeeting(calendarOwnerCtx(e, e.Rep1), key, test.start); err != nil {
				t.Fatal(err)
			}
		} else {
			if _, err := e.Activities.UpdateActivity(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), ids.From[ids.ActivityKind](source.ID), activities.UpdateActivityInput{MeetingStatus: new("held")}); err != nil {
				t.Fatal(err)
			}
		}
		c := Commitment{
			AsOf: asOf, Extractor: transcriptProposalActor, SourceActivityID: source.ID,
			Seat: &e.Rep1, Summary: "Send pricing", Quote: "I will send the pricing.", Confidence: 0.95,
			Locator: commitmentLocator(source.ID, claimKindOurs, e.Rep1.String(), "I will send the pricing."),
		}
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			outcome, _, err := dispatcher.DispatchTx(ctx, tx, c, ids.NewV7())
			if err == nil && outcome != test.want {
				t.Errorf("%s: outcome = %q, want %q", test.name, outcome, test.want)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND archived_at IS NULL AND NOT is_done`); got != 1 {
		t.Fatalf("current tasks = %d, want only the held meeting's promise", got)
	}
}
