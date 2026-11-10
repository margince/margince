// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReplacingAnAskRequiresAFreshDecisionAndKeepsItsHistoryPrivate(t *testing.T) {
	e := setupIntro(t)
	ctx := e.asUser(e.requester)
	original, err := e.store.Create(ctx, e.ask())
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.Decide(e.asUser(e.introducer), original, StatusAccepted, "", nil, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET status='deactivated' WHERE id=@id`, pgx.StrictNamedArgs{"id": e.introducer}); err != nil {
		t.Fatal(err)
	}
	fresh := e.ask()
	fresh.IntroducerUser = e.stranger
	fresh.InternalReason = "Please review this separate route."
	replacement, err := e.store.Replace(ctx, original, "The original colleague is unavailable.", 2, fresh)
	if err != nil {
		t.Fatal(err)
	}
	old, err := e.store.Get(ctx, original)
	if err != nil {
		t.Fatal(err)
	}
	if old.Status != StatusCancelled || old.IntroducerUser != e.introducer || old.Version != 3 {
		t.Fatalf("original history changed: %+v", old)
	}
	newAsk, err := e.store.Get(ctx, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if newAsk.Status != StatusRequested || newAsk.Version != 1 || newAsk.DecidedAt != nil || newAsk.IntroducerUser != e.stranger || newAsk.RequesterUserID != e.requester {
		t.Fatalf("replacement inherited approval: %+v", newAsk)
	}
	if _, err := e.store.Get(e.asUser(e.stranger), original); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("new colleague can read the old private ask: %v", err)
	}
	if _, err := e.store.Get(e.asUser(e.introducer), replacement); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("old colleague can read the new private ask: %v", err)
	}
}

func TestReplacingAnAskRollsBackItsWithdrawalWhenTheNewRouteIsRefused(t *testing.T) {
	for _, scenario := range []string{"same route", "inactive receiver", "duplicate route", "stale version", "not the requester"} {
		t.Run(scenario, func(t *testing.T) {
			e := setupIntro(t)
			ctx := e.asUser(e.requester)
			original, err := e.store.Create(ctx, e.ask())
			if err != nil {
				t.Fatal(err)
			}
			fresh := e.ask()
			fresh.IntroducerUser = e.stranger
			version := 1
			switch scenario {
			case "same route":
				fresh = e.ask()
			case "inactive receiver":
				if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET status='deactivated' WHERE id=@id`, pgx.StrictNamedArgs{"id": e.stranger}); err != nil {
					t.Fatal(err)
				}
			case "duplicate route":
				if _, err := e.store.Create(ctx, fresh); err != nil {
					t.Fatal(err)
				}
			case "stale version":
				version = 9
			case "not the requester":
				ctx = e.asUser(e.introducer)
			}
			var countBefore int
			if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM event_outbox`).Scan(&countBefore); err != nil {
				t.Fatal(err)
			}
			if replacement, err := e.store.Replace(ctx, original, "Review another route.", version, fresh); err == nil || replacement != (ids.UUID{}) {
				t.Fatalf("refused replacement = %s, %v", replacement, err)
			}
			current, err := e.store.Get(e.asUser(e.requester), original)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != StatusRequested || current.Version != 1 {
				t.Fatalf("refusal closed the original ask: %+v", current)
			}
			var countAfter int
			if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM event_outbox`).Scan(&countAfter); err != nil {
				t.Fatal(err)
			}
			if countAfter != countBefore {
				t.Fatalf("refusal emitted %d events", countAfter-countBefore)
			}
		})
	}
}
