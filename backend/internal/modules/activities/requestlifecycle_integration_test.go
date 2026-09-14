// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
