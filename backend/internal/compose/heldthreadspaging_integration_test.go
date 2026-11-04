// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedHeldThread writes one ledger row, the parties it saw, and the message it
// was raised about at `at`, or no message when erased.
func seedHeldThread(t *testing.T, e *integration.Env, user ids.UUID, key, status string, attempts int, at time.Time, erased bool, seen ...string) {
	t.Helper()
	if seen == nil {
		seen = []string{}
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		var message *ids.UUID
		if !erased {
			id := ids.NewV7()
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
				VALUES ($1, 'email', $2, $3, 'gmail', $4, 'participants')`,
				id, "Subject of "+key, at, "human:"+user.String()); err != nil {
				return err
			}
			message = &id
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, attempts, first_activity_id, seen_addresses, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, key, user, status, attempts, message, seen, at)
		return err
	}); err != nil {
		t.Fatalf("seeding held thread %s: %v", key, err)
	}
}

func heldPage(t *testing.T, e *integration.Env, user ids.UUID, query capture.HeldThreadQuery) capture.HeldThreadPage {
	t.Helper()
	page, err := capture.HeldThreadsFor(purgeCtx(e, user), InstallationDB(e.Pool), query)
	if err != nil {
		t.Fatalf("listing held threads under %+v: %v", query, err)
	}
	return page
}

// A long list arrives a page at a time, each thread once, pending first and
// then newest. The counts count the whole list on every page.
func TestTheHeldListPagesEachThreadOnceInOrder(t *testing.T) {
	e := integration.Setup(t)
	day := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	seedHeldThread(t, e, e.Rep1, "held-old", "held", 1, day, false)
	seedHeldThread(t, e, e.Rep1, "held-new", "held", 1, day.Add(48*time.Hour), false)
	seedHeldThread(t, e, e.Rep1, "pending-stalled", "pending", capture.PendingMaxAttempts, day, false)
	seedHeldThread(t, e, e.Rep1, "pending-new", "pending", 0, day.Add(24*time.Hour), false)
	seedHeldThread(t, e, e.Rep1, "unsure", "unsure", 2, day.Add(12*time.Hour), false)

	var keys []string
	query := capture.HeldThreadQuery{Limit: 2}
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("the pages never end")
		}
		page := heldPage(t, e, e.Rep1, query)
		if page.Counts != (capture.HeldThreadCounts{Pending: 2, Unsure: 1, Held: 2, Stalled: 1}) {
			t.Fatalf("page %d counts %+v, want the whole list's", pages, page.Counts)
		}
		for _, thread := range page.Threads {
			keys = append(keys, thread.ThreadKey)
		}
		if page.Next == "" {
			break
		}
		query.Cursor = page.Next
	}
	want := []string{"pending-new", "pending-stalled", "held-new", "unsure", "held-old"}
	if len(keys) != len(want) {
		t.Fatalf("the pages listed %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("the pages listed %v, want %v", keys, want)
		}
	}
}

// A search matches the parties a hold saw, so a thread whose message was erased
// is still found by who it was with. A status narrows the list.
func TestASearchFindsAThreadByItsPartyEvenWhenItsMessageIsGone(t *testing.T) {
	e := integration.Setup(t)
	day := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	seedHeldThread(t, e, e.Rep1, "erased-with-lena", "held", 1, day, true, "lena@prospect.test")
	seedHeldThread(t, e, e.Rep1, "about-the-renewal", "pending", 0, day, false, "sam@northwind.test")

	found := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Q: "LENA@prospect"})
	if len(found.Threads) != 1 || found.Threads[0].ThreadKey != "erased-with-lena" {
		t.Errorf("searching a party's address found %+v, want the erased thread", found.Threads)
	}
	bySubject := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Q: "about-the-renewal"})
	if len(bySubject.Threads) != 1 || bySubject.Threads[0].ThreadKey != "about-the-renewal" {
		t.Errorf("searching a subject found %+v, want the renewal thread", bySubject.Threads)
	}
	pending := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Status: "pending"})
	if len(pending.Threads) != 1 || pending.Threads[0].ThreadKey != "about-the-renewal" {
		t.Errorf("the pending filter listed %+v, want the pending thread alone", pending.Threads)
	}
}

// The contact filter keeps the threads with that contact, by the addresses its
// hold saw. A contact the reader may not read is a missing row.
func TestTheContactFilterKeepsTheThreadsWithThatContact(t *testing.T) {
	e := integration.Setup(t)
	day := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	lena := e.SeedContact(t, "Lena Prospect", &e.Rep1)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by) VALUES ($1, 'lena@prospect.test', true, 'manual', 'human:x')`, lena)
	seedHeldThread(t, e, e.Rep1, "with-lena", "held", 1, day, true, "Lena@Prospect.test")
	seedHeldThread(t, e, e.Rep1, "with-sam", "held", 1, day, false, "sam@northwind.test")

	page := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{ContactID: &lena})
	if len(page.Threads) != 1 || page.Threads[0].ThreadKey != "with-lena" {
		t.Errorf("the contact filter listed %+v, want the thread with Lena alone", page.Threads)
	}

	hidden := e.SeedContact(t, "Private Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", hidden, e.Rep3)
	_, err := capture.HeldThreadsFor(purgeCtx(e, e.Rep1), InstallationDB(e.Pool), capture.HeldThreadQuery{ContactID: &hidden})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("filtering by a contact the reader may not read answered %v, want not found", err)
	}
}
