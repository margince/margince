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
)

func TestSharedMailIdentityMustResolveBeforeItAssignsPersonalWork(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	first := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	second := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
	identities := capture.NewOwnerIdentityStore(e.DB())
	alias := "shared-sales@ourco.test"
	if _, err := identities.Add(first, "address", alias); err != nil {
		t.Fatal(err)
	}
	overlap, err := identities.Add(second, "address", alias)
	if err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Buyer", &e.AdminUser)
	subject, body, direction := "Please send the plan", "Please send the plan for review.", "inbound"
	message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual", EmailFrom: "buyer@customer.test", EmailTo: []string{alias}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(message.Id)
	if _, err := e.Activities.SetOwedVerdict(first, id, activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
	check := func(ctx context.Context, scope string, owner ids.UUID, want int) {
		t.Helper()
		page, err := feed.Worklist(ctx, scope, "all", owner, 100, "")
		if err != nil {
			t.Fatal(err)
		}
		if page.Summary.Total != want || len(page.Queue) != want {
			t.Fatalf("scope %s owner %s: summary=%d queue=%d want%d", scope, owner, page.Summary.Total, len(page.Queue), want)
		}
	}
	check(first, "mine", ids.Nil, 0)
	check(second, "mine", ids.Nil, 0)
	check(first, "all", e.Rep2, 0)
	check(first, "all", ids.Nil, 1)
	board, err := feed.TeamBoard(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range board.Members {
		if member.Counts.Waiting != 0 {
			t.Fatal("shared mailbox assigned a personal TeamBoard wait")
		}
	}
	if err := e.Activities.MarkMessageHandled(first, ids.From[ids.ActivityKind](id)); err != nil {
		t.Fatal(err)
	}
	checkHidden := func(want int) {
		t.Helper()
		counts, err := e.Activities.HiddenWaiting(first, at.Add(time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		rows, err := e.Activities.HiddenWaitingRows(first, at.Add(time.Hour), activities.HiddenRuleSetAside)
		if err != nil {
			t.Fatal(err)
		}
		if counts.SetAside != want || len(rows) != want {
			t.Fatalf("hidden counts=%+v rows=%d want%d", counts, len(rows), want)
		}
		for _, row := range rows {
			if row.ActivityID != id || row.AddressedElsewhere {
				t.Fatalf("hidden recipient differs from current personal responsibility: %+v", row)
			}
		}
	}
	checkHidden(0)
	if err := identities.Remove(second, overlap.ID); err != nil {
		t.Fatal(err)
	}
	checkHidden(1)
	if err := e.Activities.ClearMessageDisposition(first, ids.From[ids.ActivityKind](id)); err != nil {
		t.Fatal(err)
	}
	check(first, "mine", ids.Nil, 1)
	check(second, "mine", ids.Nil, 0)
	check(second, "all", e.Rep1, 1)
	// Two independent To addresses still name two seats, unlike one shared address.
	var secondAddress string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "SELECT email FROM app_user WHERE id=$1", e.Rep2).Scan(&secondAddress)
	}); err != nil {
		t.Fatal(err)
	}
	other, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual", EmailFrom: "buyer@customer.test", EmailTo: []string{alias, secondAddress}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Activities.SetOwedVerdict(first, ids.UUID(other.Id), activities.OwedVerdictAsksUs, "fixture", at); err != nil {
		t.Fatal(err)
	}
	check(first, "mine", ids.Nil, 2)
	check(second, "mine", ids.Nil, 1)
	check(first, "all", e.Rep2, 1)
	page, err := feed.Worklist(first, "all", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range page.Queue {
		if row.Id != other.Id.String() {
			continue
		}
		found = true
		if row.Owner == nil || row.Owner.Kind != "shared" || row.Owner.Users == nil || len(*row.Owner.Users) != 2 {
			t.Fatalf("multiple direct recipients lost shared ownership: %+v", row.Owner)
		}
	}
	if !found {
		t.Fatal("shared request disappeared from Team monitoring")
	}
	check(first, "unassigned", ids.Nil, 0)
}

func TestVerifiedMailIdentityOutranksClaimsAndDeliveryDoesNotProveDuty(t *testing.T) {
	for _, tc := range []struct {
		name string
		want bool
	}{
		{"login", true}, {"provider", true}, {"declared", true}, {"delivered", false}, {"delivered_then_declared", true}, {"disconnected", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
			target := "mail-authority@ourco.test"
			first := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			second := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
			identities := capture.NewOwnerIdentityStore(e.DB())
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				switch tc.name {
				case "login":
					return tx.QueryRow(context.Background(), `SELECT email FROM app_user WHERE id=$1`, e.Rep1).Scan(&target)
				case "provider", "disconnected":
					status := "connected"
					if tc.name == "disconnected" {
						status = "disconnected"
					}
					_, err := tx.Exec(context.Background(), `INSERT INTO capture_connection(user_id,provider,status,account_label) VALUES($1,'gmail',$2,$3)`, e.Rep1, status, "First Reader <"+target+">")
					return err
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if tc.name == "declared" || tc.name == "delivered" || tc.name == "delivered_then_declared" {
				claim, err := identities.Add(first, "address", target)
				if err != nil {
					t.Fatal(err)
				}
				if tc.name == "delivered" || tc.name == "delivered_then_declared" {
					if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
						_, err := tx.Exec(context.Background(), `UPDATE capture_owner_identity SET source='delivered_to' WHERE id=$1`, claim.ID)
						return err
					}); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.name == "delivered_then_declared" {
				declared, err := identities.Add(first, "address", target)
				if err != nil {
					t.Fatal(err)
				}
				if declared.Source != capture.IdentitySourceUser {
					t.Fatalf("explicit declaration retained inferred source: %s", declared.Source)
				}
			}
			if tc.name == "login" || tc.name == "provider" {
				if _, err := identities.Add(second, "address", target); err != nil {
					t.Fatal(err)
				}
			}
			contact := e.SeedContact(t, "Buyer", &e.AdminUser)
			subject, body, direction := "Please send the plan", "Please send the plan.", "inbound"
			message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual", EmailFrom: "buyer@customer.test", EmailTo: []string{target}, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.Activities.SetOwedVerdict(first, ids.UUID(message.Id), activities.OwedVerdictAsksUs, "fixture", at); err != nil {
				t.Fatal(err)
			}
			feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return at.Add(time.Hour) })
			for i, reader := range []context.Context{first, second} {
				page, err := feed.Worklist(reader, "mine", "all", ids.Nil, 100, "")
				if err != nil {
					t.Fatal(err)
				}
				want := 0
				if i == 0 && tc.want {
					want = 1
				}
				if page.Summary.Total != want || len(page.Queue) != want {
					t.Fatalf("reader%d: total=%d queue=%d want%d", i, page.Summary.Total, len(page.Queue), want)
				}
			}
		})
	}
}
