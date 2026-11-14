// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// summaryTranscript is two lines, so a summary point citing line 3 cites
// nothing the meeting holds.
const summaryTranscript = "Anna: Let's ship by Friday.\nBen: Works for me."

func logSummaryTranscript(ctx context.Context, t *testing.T, store *Store) ids.ActivityID {
	t.Helper()
	body, sourceSystem := summaryTranscript, "transcript"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "meeting", Body: &body, SourceSystem: &sourceSystem, Source: "manual",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := store.LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(activity.Id))
}

// claimReading starts a reading and claims it, the way the worker does
// before it puts the transcript to the model.
func claimReading(ctx context.Context, t *testing.T, store *Store, activityID ids.ActivityID) ids.UUID {
	t.Helper()
	started, _, err := store.StartTranscriptReadQueued(ctx, activityID, "human:test", nil)
	if err != nil {
		t.Fatalf("starting the reading: %v", err)
	}
	if _, err := store.BeginTranscriptRead(ctx, started.ID, TranscriptReadLease); err != nil {
		t.Fatalf("claiming the reading: %v", err)
	}
	return started.ID
}

func finishWith(ctx context.Context, store *Store, readID ids.UUID, facts *TranscriptReadFacts) error {
	return store.FinishTranscriptRead(ctx, readID, TranscriptReadOutcome{
		Status: TranscriptReadDone, Detail: "states no next steps", LineCount: 2, Facts: facts,
	})
}

func TestAMeetingKeepsItsSummaryWhenALaterReadingWritesNone(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	activityID := logSummaryTranscript(ctx, t, store)
	summary := TranscriptSummary{
		KeyPoints: []TranscriptSummaryPoint{{Text: "They agreed to ship by Friday.", SourceLines: []int{1, 2}}},
		NextSteps: []TranscriptSummaryPoint{{Text: "Ship by Friday.", SourceLines: []int{1}}},
	}

	first := claimReading(ctx, t, store, activityID)
	if err := finishWith(ctx, store, first, &TranscriptReadFacts{Summary: &summary}); err != nil {
		t.Fatalf("finishing the first reading: %v", err)
	}
	second := claimReading(ctx, t, store, activityID)
	if err := finishWith(ctx, store, second, &TranscriptReadFacts{SummaryRefused: true}); err != nil {
		t.Fatalf("finishing the second reading: %v", err)
	}

	latest, err := store.LatestTranscriptRead(ctx, activityID)
	if err != nil {
		t.Fatalf("reading the latest reading: %v", err)
	}
	if latest.ID != second || !latest.Outcome.SummaryRefused {
		t.Fatalf("the latest reading must be the second and say its summary was refused, got %+v", latest)
	}
	if latest.Summary == nil || latest.Summary.ReadID != first || latest.Summary.NextSteps[0].Text != "Ship by Friday." {
		t.Fatalf("a reading that wrote no summary must leave the meeting's summary as the first wrote it, got %+v", latest.Summary)
	}
	if latest.Summary.GeneratedAt.IsZero() {
		t.Error("the standing summary must say when it was written")
	}
}

func TestAReadingSeesNoSummaryALaterReadingWrote(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	activityID := logSummaryTranscript(ctx, t, store)
	first := claimReading(ctx, t, store, activityID)
	if err := finishWith(ctx, store, first, &TranscriptReadFacts{SummaryRefused: true}); err != nil {
		t.Fatalf("finishing the first reading: %v", err)
	}
	second := claimReading(ctx, t, store, activityID)
	summary := TranscriptSummary{KeyPoints: []TranscriptSummaryPoint{{Text: "Friday.", SourceLines: []int{1}}}}
	if err := finishWith(ctx, store, second, &TranscriptReadFacts{Summary: &summary}); err != nil {
		t.Fatalf("finishing the second reading: %v", err)
	}

	earlier, err := store.GetTranscriptRead(ctx, activityID, first)
	if err != nil {
		t.Fatalf("reading the first reading: %v", err)
	}
	if earlier.Summary != nil {
		t.Errorf("the first reading's report must not show a summary written after it, got %+v", earlier.Summary)
	}
}

func TestTheStoreRefusesASummaryThatCitesALineTheTranscriptLacks(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	store := e.store(nil)
	activityID := logSummaryTranscript(ctx, t, store)
	readID := claimReading(ctx, t, store, activityID)
	summary := TranscriptSummary{KeyPoints: []TranscriptSummaryPoint{{Text: "Friday.", SourceLines: []int{3}}}}

	if err := finishWith(ctx, store, readID, &TranscriptReadFacts{Summary: &summary}); err == nil {
		t.Fatal("a summary citing line 3 of a two-line transcript must be refused")
	}
	read, err := store.GetTranscriptRead(ctx, activityID, readID)
	if err != nil {
		t.Fatalf("reading the reading back: %v", err)
	}
	if read.Status != TranscriptReadRunning || read.Summary != nil {
		t.Errorf("a refused summary must leave the reading running and the meeting without a summary, got %s %+v",
			read.Status, read.Summary)
	}
}
