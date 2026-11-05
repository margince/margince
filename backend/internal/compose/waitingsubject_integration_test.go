// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func addressedRequest(t *testing.T, e *integration.Env, recipient ids.UUID, at time.Time, index int) ids.ActivityID {
	t.Helper()
	var address string
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "SELECT email FROM app_user WHERE id=$1", recipient).Scan(&address)
	}); err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, fmt.Sprintf("Buyer %d", index), &e.AdminUser)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Direction: new("inbound"), Subject: new(fmt.Sprintf("Specification %d", index)), Body: new("Please send the specification."), OccurredAt: &at, Source: "manual", ThreadKey: fmt.Sprintf("request-%d", index), EmailFrom: fmt.Sprintf("buyer%d@customer.test", index), EmailTo: []string{address}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := e.Activities.SetOwedVerdict(principal.SystemActing(e.Admin(), "system:owed_verdict"), id.UUID, activities.OwedVerdictAsksUs, "request", at); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSelectedWaitingUsesTheSelectedSeatsDisposition(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	source := addressedRequest(t, e, e.Rep1, now.Add(-time.Hour), 1)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	for _, state := range []string{"manager set aside", "recipient set aside"} {
		t.Run(state, func(t *testing.T) {
			actor := e.Admin()
			want := 1
			if state == "recipient set aside" {
				actor = reader
				want = 0
			}
			if err := e.Activities.SetMessageNotMine(actor, source); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := e.Activities.ClearMessageDisposition(actor, source); err != nil {
					t.Error(err)
				}
			})
			own, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			selected, err := feed.Worklist(e.Admin(), "all", "all", e.Rep1, 100, "")
			if err != nil {
				t.Fatal(err)
			}
			if len(own.Queue) != want || len(selected.Queue) != want || own.Summary.Total != selected.Summary.Total {
				t.Fatalf("personal=%+v selected=%+v want%d", own.Queue, selected.Queue, want)
			}
			if err := e.Activities.ClearMessageDisposition(actor, source); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHiddenWaitingDoesNotCountAnotherSeatsMailAsPersonalWork(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	addressedRequest(t, e, e.AdminUser, now.Add(-time.Hour), 1)
	addressedRequest(t, e, e.Rep1, now.Add(-time.Hour), 2)
	store := NewOwedClassifier(e.Pool, nil, func() time.Time { return now }, nil).store
	hidden, err := store.HiddenWaiting(e.Admin(), now)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Shown != 1 {
		t.Fatalf("personal waiting=%d, want1; colleague's mail is not personal backlog", hidden.Shown)
	}
}

func TestWaitingContinuationKeepsMessagesSharingTheBoundaryInstant(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	for i := range activities.WaitingScanCap + 1 {
		addressedRequest(t, e, e.AdminUser, now.Add(-time.Hour), i)
	}
	store := NewOwedClassifier(e.Pool, nil, func() time.Time { return now }, nil).store
	first, err := store.WaitingRepliesForOwnerBefore(e.Admin(), now, activities.WaitingPosition{}, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != activities.WaitingScanCap {
		t.Fatalf("first page has%d rows", len(first))
	}
	last := first[len(first)-1]
	second, err := store.WaitingRepliesForOwnerBefore(e.Admin(), now, activities.WaitingPosition{At: last.OccurredAt, ID: last.ActivityID}, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("same-instant continuation has%d rows, want1", len(second))
	}
	for _, row := range first {
		if row.ActivityID == second[0].ActivityID {
			t.Fatal("continuation repeated a row")
		}
	}
}
