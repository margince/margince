// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestCachedDealCardRefreshesTheRequestEvaluationWithoutRebuildingItsWords(t *testing.T) {
	e := setupVerdicts(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	at := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
	mailAt := at.AddDate(0, 0, -200)
	direction, subject := "inbound", "Please send the scope"
	source, _, err := store.LogActivity(reader, activities.LogActivityInput{
		Kind: "email", Source: "manual", Direction: &direction, Subject: &subject,
		OccurredAt: &mailAt, ThreadKey: "request-evaluation",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: e.booked}},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestID := ids.UUID(source.Id)
	if _, _, err := store.LogActivity(reader, activities.LogActivityInput{Kind: "task", Source: "manual", RequestActivityID: &requestID}); err != nil {
		t.Fatal(err)
	}
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	first, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.booked))
	if err != nil {
		t.Fatal(err)
	}
	if first.RequestBasis == nil || first.RequestBasis.EvaluatedAt != at || first.RequestBasis.SourceVersion == nil {
		t.Fatalf("fresh card lost current evidence: %+v", first.RequestBasis)
	}
	at = at.Add(time.Minute)
	second, err := cards.ReadFacts(reader, ids.From[ids.DealKind](e.booked))
	if err != nil {
		t.Fatal(err)
	}
	if second.GeneratedAt != first.GeneratedAt {
		t.Fatal("unchanged facts rebuilt the cached words")
	}
	if second.RequestBasis == nil || second.RequestBasis.EvaluatedAt != at || second.RequestBasis.SourceActivityId != source.Id {
		t.Fatalf("cached card reused an old evaluation: %+v", second.RequestBasis)
	}
}
