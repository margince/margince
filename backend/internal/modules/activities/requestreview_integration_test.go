// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnotherReaderSeesCoverageWithoutReadingAPrivateReminder(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please send slots", "meeting", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	other, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	other.UserID, other.ID = e.other, "human:"+e.other.String()
	other.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true}
	reader := principal.WithActor(e.as(), other)
	summaries, err := store.EmailSummariesByID(reader, []ids.UUID{id})
	if err != nil {
		t.Fatal(err)
	}
	summary, found := summaries[id]
	if !found || summary.RequestHasReminder == nil || !*summary.RequestHasReminder {
		t.Fatal("visible source lost its coverage fact")
	}
	if summary.Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("assignment was mistaken for completion")
	}
	if _, _, err := store.LogActivity(reader, LogActivityInput{Kind: "task", RequestActivityID: &id, Source: "ui"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("private replay = %v, want conflict without task disclosure", err)
	}
	kind := "task"
	rows, _, err := store.ListActivities(reader, ListActivitiesInput{Kind: &kind})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.SourceActivityId != nil && ids.UUID(*row.SourceActivityId) == id && row.Subject != nil {
			t.Fatal("coverage disclosed private task content")
		}
	}
}

func TestConflictingNoiseLabelStaysReviewableWithoutAutomaticAssignment(t *testing.T) {
	e := setupLoad(t)
	id := seedEmailRequest(t, e, "Please confirm the schedule", "noise", OwedVerdictAsksUs)
	store := storeKnowing(e)
	at := requestInstant.Add(time.Hour)
	if err := store.CaptureEmailRequests(asClassifier(e), at); err != nil {
		t.Fatal(err)
	}
	source, err := store.GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](id), nil)
	if err != nil {
		t.Fatal(err)
	}
	if source.Summary.RequestHasReminder == nil || *source.Summary.RequestHasReminder || source.Summary.Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("conflicting topic either assigned or lost the confirmed request")
	}
	review, _, err := store.ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &at})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range review {
		if ids.UUID(row.Id) == id {
			return
		}
	}
	t.Fatal("conflicting topic hid the request from review")
}

func TestUnthreadedUnclassifiedMailNeedsRequestEvidence(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	subject, direction := "A message", "inbound"
	source, _, err := store.LogActivity(asClassifier(e), LogActivityInput{Kind: "email", Subject: &subject, Direction: &direction, Source: "test", OccurredAt: &requestInstant, Links: []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}}})
	if err != nil {
		t.Fatal(err)
	}
	at := requestInstant.Add(time.Hour)
	assertListed := func(want bool) {
		t.Helper()
		rows, _, err := store.ListActivities(e.as(), ListActivitiesInput{RequestReviewAsOf: &at})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			found = found || row.Id == source.Id
		}
		if found != want {
			t.Fatalf("unthreaded review = %t, want %t", found, want)
		}
	}
	assertListed(false)
	if _, err := store.SetCaptureLabel(asClassifier(e), ids.UUID(source.Id), "meeting"); err != nil {
		t.Fatal(err)
	}
	assertListed(true)
}

func TestAcceptanceKeepsTheClassifierVerdictAndHonorsTheTaskText(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	reader := principal.WithActor(e.as(), actor)
	subject, direction := "Can you send slots?", "inbound"
	source, _, err := store.LogActivity(reader, LogActivityInput{Kind: "email", Subject: &subject, Direction: &direction, ThreadKey: "private-request", Source: "test", OccurredAt: &requestInstant})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(source.Id)
	if _, err := store.SetAudience(reader, ids.From[ids.ActivityKind](id), SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	title, body := "Send three slots", "Check the calendar first"
	task, _, err := store.LogActivity(reader, LogActivityInput{Kind: "task", RequestActivityID: &id, Subject: &title, Body: &body, Source: "ui"})
	if err != nil {
		t.Fatal(err)
	}
	if task.Subject == nil || *task.Subject != title || task.Body == nil || *task.Body != body {
		t.Fatal("acceptance discarded task text")
	}
	retained, err := store.GetActivity(reader, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	var verdict *string
	args := []any{id}
	if err := e.owner.QueryRow(reader, fmt.Sprintf("SELECT owed_verdict FROM activity WHERE id=$%d", len(args)), args...).Scan(&verdict); err != nil {
		t.Fatal(err)
	}
	if verdict != nil {
		t.Fatal("acceptance rewrote the source classifier verdict")
	}
	if retained.EmailSummary == nil || retained.EmailSummary.Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("accepted request lost its obligation without a classifier verdict")
	}
}

func TestDifferentRequestsInOneConversationNeedSeparateResolution(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	reader := principal.WithActor(e.as(), actor)
	contact := e.buyer(t)
	subject, direction := "Re: rollout", "inbound"
	requests := []ids.UUID{}
	for _, body := range []string{"Please send meeting slots.", "Please also send the signed DPA."} {
		source, _, err := store.LogActivity(reader, LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, ThreadKey: "two-requests", OccurredAt: &requestInstant, Source: "test", Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.UUID(source.Id)
		requests = append(requests, id)
		if _, err := store.SetOwedVerdict(asClassifier(e), id, OwedVerdictAsksUs, "prompts-test", dbNow(t, e)); err != nil {
			t.Fatal(err)
		}
	}
	at := requestInstant.Add(time.Hour)
	waiting, err := store.WaitingReplies(e.as(), at)
	if err != nil {
		t.Fatal(err)
	}
	found := map[ids.UUID]bool{}
	for _, row := range waiting {
		found[row.ActivityID] = true
	}
	if !found[requests[0]] || !found[requests[1]] {
		t.Fatal("matching thread/subject discarded a distinct obligation")
	}
	task, _, err := store.LogActivity(reader, LogActivityInput{Kind: "task", RequestActivityID: &requests[0], Source: "ui"})
	if err != nil {
		t.Fatal(err)
	}
	done := true
	if _, err := store.UpdateActivity(reader, ids.From[ids.ActivityKind](ids.UUID(task.Id)), UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	waiting, err = store.WaitingReplies(e.as(), at)
	if err != nil {
		t.Fatal(err)
	}
	found = map[ids.UUID]bool{}
	for _, row := range waiting {
		found[row.ActivityID] = true
	}
	if found[requests[0]] || !found[requests[1]] {
		t.Fatal("settling one message also settled another ask")
	}
}

func TestPrivateReplyStillAnswersAnUnclassifiedThread(t *testing.T) {
	e := setupLoad(t)
	store := storeKnowing(e)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing actor")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true}
	reader := principal.WithActor(e.as(), actor)
	contact := e.buyer(t)
	subject, direction := "A conversation", "inbound"
	source, _, err := store.LogActivity(reader, LogActivityInput{Kind: "email", Subject: &subject, Direction: &direction, ThreadKey: "private-answer", OccurredAt: &requestInstant, Source: "test", Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	at := requestInstant.Add(time.Minute)
	direction = "outbound"
	reply, _, err := store.LogActivity(reader, LogActivityInput{Kind: "email", Subject: &subject, Direction: &direction, ThreadKey: "private-answer", OccurredAt: &at, Source: "test", Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetAudience(reader, ids.From[ids.ActivityKind](ids.UUID(reply.Id)), SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	reader = principal.WithActor(e.as(), actor)
	hidden, err := store.GetActivity(reader, ids.From[ids.ActivityKind](ids.UUID(reply.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Subject != nil {
		t.Fatal("test reply is not private")
	}
	review, _, err := store.ListActivities(reader, ListActivitiesInput{RequestReviewAsOf: &at})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range review {
		if row.Id == source.Id {
			t.Fatal("private reply was ignored by the conversation fallback")
		}
	}
}
