// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSchedulingRequestSurvivesAcknowledgementAndCreatesOneTask(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send meeting slots", "meeting", OwedVerdictAsksUs)
	store := storeKnowing(e)
	source, err := store.GetActivity(e.as(), ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	direction, subject, at := "outbound", "Thanks, I will check", requestInstant.Add(time.Minute)
	if _, _, err := store.LogActivity(asClassifier(e), LogActivityInput{Kind: "email", Direction: &direction, Subject: &subject, ThreadKey: *source.ThreadKey, Source: "test", OccurredAt: &at}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_activity_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("scheduling request produced %d reminders", count)
	}
	summaries, err := store.EmailSummariesByID(e.as(), []ids.UUID{id})
	if err != nil {
		t.Fatal(err)
	}
	if summaries[id].Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("acknowledgement settled the request")
	}
}

func TestHistoricalRequestIsReviewableAndExplicitAcceptanceIsIdempotent(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send slots", "meeting", OwedVerdictAsksUs)
	store := storeKnowing(e)
	asOf := requestInstant.AddDate(0, 0, 200)
	if err := store.CaptureEmailRequests(asClassifier(e), asOf); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_activity_id=$1`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("historical import silently assigned work")
	}
	waiting, err := store.WaitingReplies(e.as(), asOf)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range waiting {
		found = found || row.ActivityID == id
	}
	if !found {
		t.Fatal("historical request disappeared from review")
	}
	review, _, err := store.ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &asOf})
	if err != nil {
		t.Fatal(err)
	}
	found = false
	for _, row := range review {
		found = found || ids.UUID(row.Id) == id
	}
	if !found {
		t.Fatal("record review disagrees with waiting queue")
	}
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	reader := principal.WithActor(e.as(), actor)
	request := LogActivityInput{Kind: "task", Source: "ui", RequestActivityID: &id}
	task, created, err := store.LogActivity(reader, request)
	if err != nil {
		t.Fatal(err)
	}
	if !created || task.SourceActivityId == nil || ids.UUID(*task.SourceActivityId) != id {
		t.Fatal("acceptance lost source evidence")
	}
	replay, created, err := store.LogActivity(reader, request)
	if err != nil {
		t.Fatal(err)
	}
	if created || replay.Id != task.Id {
		t.Fatal("review created duplicate task")
	}
	if _, err := store.ArchiveActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)), nil); err != nil {
		t.Fatal(err)
	}
	restored, created, err := store.LogActivity(reader, request)
	if err != nil {
		t.Fatal(err)
	}
	if created || restored.Id != task.Id || restored.ArchivedAt != nil {
		t.Fatal("explicit acceptance did not restore the original unfinished reminder")
	}
	done := true
	if _, err := store.UpdateActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)), UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	review, _, err = store.ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &asOf})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range review {
		if ids.UUID(row.Id) == id {
			t.Fatal("explicit completion did not settle the request")
		}
	}
	summaries, err := store.EmailSummariesByID(e.as(), []ids.UUID{id})
	if err != nil {
		t.Fatal(err)
	}
	if summaries[id].Move != crmcontracts.EmailSummaryMoveNone {
		t.Fatal("completed request remains actionable in email")
	}
}

// TestArchivingAStaleTaskRefusesRatherThanHidingTheAnswer.
//
// ArchiveActivity takes a version pin and nothing proved what it does with a
// stale one — which matters because a sweep that reads a batch of tasks and
// archives them later is relying on exactly that: between the read and the
// write a rep can complete the task, and an unpinned archive would bury their
// answer without either of them seeing it.
//
// So the pin refuses, and the caller decides. The compose-side sweep that
// settles duplicate assurance tasks reads that refusal as "somebody wrote to
// this; leave it alone".
func TestArchivingAStaleTaskRefusesRatherThanHidingTheAnswer(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{
		Read: true, Create: true, Update: true, Delete: true,
	}
	reader := principal.WithActor(e.as(), actor)

	subject := "Chase the retrofit quote"
	task, _, err := store.LogActivity(reader, LogActivityInput{
		Kind: "task", Source: "ui", Subject: &subject,
	})
	if err != nil {
		t.Fatalf("logging the task: %v", err)
	}
	stale := (*int64)(task.Version)
	if stale == nil {
		t.Fatal("a freshly written task carries no version, so nothing can be pinned to it")
	}

	// The rep answers it, which moves the version — the write the sweep did
	// not see.
	done := true
	if _, err := store.UpdateActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)),
		UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatalf("completing the task: %v", err)
	}

	if _, err := store.ArchiveActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)), stale); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("archiving on the stale version answered %v, want ErrVersionSkew — an unpinned archive "+
			"here buries an answer the rep had already given", err)
	}

	// And the answer stands: refusing is only worth anything if it left the
	// row alone.
	after, err := store.GetActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("re-reading the task: %v", err)
	}
	if after.ArchivedAt != nil {
		t.Error("the refused archive still archived the task")
	}
	if after.IsDone == nil || !*after.IsDone {
		t.Error("the refused archive lost the rep's completion")
	}
}
