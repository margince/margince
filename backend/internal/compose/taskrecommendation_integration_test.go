// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASuggestedTaskIsCheckedAgainInsideItsWriter(t *testing.T) {
	for _, change := range []string{"unchanged", "task", "meeting", "dismissed", "changed_subject"} {
		t.Run(change, func(t *testing.T) {
			e := setupVerdicts(t)
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
			cards := newDealStatusService(e.Pool, func() time.Time { return at })
			card, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
			if err != nil || card.Next == nil || card.Next.Recommendation == nil {
				t.Fatalf("current recommendation = %+v, %v", card.Next, err)
			}
			in := suggestedTaskInput(t, card.Next)
			if change == "task" || change == "meeting" {
				subject, status, when := "The agreed next step", "booked", at.Add(24*time.Hour)
				logged := activities.LogActivityInput{
					Kind: change, Source: "manual", Subject: &subject,
					Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.calm}},
				}
				if change == "meeting" {
					logged.OccurredAt, logged.MeetingStatus = &when, &status
				}
				if _, _, err := e.Activities.LogActivity(reader, logged); err != nil {
					t.Fatal(err)
				}
			}
			if change == "dismissed" {
				if card.Next.Fingerprint == nil {
					t.Fatal("current move has no dismissal identity")
				}
				if err := cards.DismissMove(reader, ids.From[ids.DealKind](e.calm), *card.Next.Fingerprint); err != nil {
					t.Fatal(err)
				}
			}
			if change == "changed_subject" {
				in.Subject = new("A different task")
			}
			before := e.WsCount(t, "SELECT count(*) FROM activity")
			store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at }).WithTaskRecommendation(taskRecommendationFrom(cards))
			_, created, err := store.LogActivity(reader, in)
			if change == "unchanged" {
				if err != nil || !created || e.WsCount(t, "SELECT count(*) FROM activity") != before+1 {
					t.Fatalf("current suggestion was not accepted: created=%t, error=%v", created, err)
				}
				return
			}
			if !errors.Is(err, apperrors.ErrVersionSkew) || e.WsCount(t, "SELECT count(*) FROM activity") != before {
				t.Fatalf("obsolete or altered suggestion wrote work: created=%t, error=%v", created, err)
			}
			if change == "task" {
				in.Recommendation = nil
				if _, created, err := store.LogActivity(reader, in); err != nil || !created {
					t.Fatalf("intentional manual work was refused: %t, %v", created, err)
				}
			}
		})
	}
}

func suggestedTaskInput(t *testing.T, move *crmcontracts.DealStatusCardMove) activities.LogActivityInput {
	t.Helper()
	if move.Action != "create_task" || move.Arguments == nil {
		t.Fatalf("not a suggested task: %+v", move)
	}
	encoded, err := json.Marshal(*move.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	var request crmcontracts.CreateTaskRequest
	if err := json.Unmarshal(encoded, &request); err != nil {
		t.Fatal(err)
	}
	in, err := activities.TaskInputFrom(request)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestAReceiptReplayCannotCreateAnotherTaskAfterItsFirstTaskIsCompleted(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	card, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.calm))
	if err != nil || card.Next == nil {
		t.Fatalf("initial recommendation: %+v, %v", card.Next, err)
	}
	in := suggestedTaskInput(t, card.Next)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at }).WithTaskRecommendation(taskRecommendationFrom(cards))
	first, created, err := store.LogActivity(reader, in)
	if err != nil || !created {
		t.Fatalf("first acceptance: created=%t, %v", created, err)
	}
	liveReplay, created, err := store.LogActivity(reader, in)
	if err != nil || created || liveReplay.Id != first.Id {
		t.Fatalf("live acceptance was not idempotent: created=%t id=%s err=%v", created, liveReplay.Id, err)
	}
	done := true
	if _, err := store.UpdateActivity(reader, ids.From[ids.ActivityKind](ids.UUID(first.Id)), activities.UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	before := e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task'")
	replay, created, err := store.LogActivity(reader, in)
	if err != nil || created || replay.Id != first.Id || e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task'") != before {
		t.Fatalf("receipt replay minted another task: original=%s replay=%s created=%t err=%v", first.Id, replay.Id, created, err)
	}
}
