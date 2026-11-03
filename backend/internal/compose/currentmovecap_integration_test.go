// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEveryDealKeepsItsNextFollowUpBeyondTheFeedScanCap(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Demo expansion", pipeline, stage, &e.Rep1)
	otherDeal := e.SeedDeal(t, "Demo independent", pipeline, stage, &e.Rep1)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
	meetingAt, status, duration, subject := at.Add(-72*time.Hour), "held", 3600, "Scope review"
	host := ids.From[ids.UserKind](e.Rep1)
	for index := range activities.AwaitingReplyScanCap + 2 {
		currentDeal := deal
		if index == activities.AwaitingReplyScanCap+1 {
			currentDeal = otherDeal
		}
		contact := e.SeedContact(t, "Demo attendee", &e.Rep1)
		if _, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "meeting", Subject: &subject, OccurredAt: &meetingAt, MeetingStatus: &status,
			DurationSeconds: &duration, HostUserID: &host, Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: currentDeal}, {EntityType: "contact", EntityID: contact}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	moves, err := cards.CurrentMoves(reader, []ids.UUID{deal, otherDeal})
	if err != nil || len(moves) != 2 || moves[deal].Action != dealstatus.ActionDraftEmail || moves[otherDeal].Action != dealstatus.ActionDraftEmail {
		t.Fatalf("the feed cap hid a deal's current follow-up: %+v, %v", moves, err)
	}
	record, err := cards.ReadFacts(reader, ids.From[ids.DealKind](deal))
	if err != nil || record.Next == nil || record.Next.Recommendation.BasisFingerprint != moves[deal].Recommendation.BasisFingerprint {
		t.Fatalf("record and batch follow-ups disagree beyond the feed cap: %+v, %v", record.Next, err)
	}
	if got := e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='meeting'"); got != activities.AwaitingReplyScanCap+2 {
		t.Fatalf("advice checking changed source history: %d meetings", got)
	}
}
