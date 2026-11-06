// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedHeldThread writes one ledger row, the parties it saw, and the message it
// was raised about at `at`, or no message when erased.
func seedHeldThread(t *testing.T, e *integration.Env, user ids.UUID, key, status string, attempts int, at time.Time, erased bool, seen ...string) ids.UUID {
	t.Helper()
	var opening ids.UUID
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
			message, opening = &id, id
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO capture_thread_verdict (thread_key, user_id, status, attempts, first_activity_id, seen_addresses, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`, key, user, status, attempts, message, seen, at)
		return err
	}); err != nil {
		t.Fatalf("seeding held thread %s: %v", key, err)
	}
	return opening
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
	limit := 2
	query := capture.HeldThreadQuery{Limit: &limit}
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
	if pending.Counts != (capture.HeldThreadCounts{Pending: 1, Held: 1}) {
		t.Errorf("a filtered page counted %+v, want the whole list", pending.Counts)
	}
}

// A search reaches a party on a readable message by address and by name.
func TestASearchMatchesTheParticipantsOfAReadableMessage(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Lena Prospect", &e.Rep1)
	message := seedHeldThread(t, e, e.Rep1, "with-lena", "held", 1, time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), false)
	seedHeldThread(t, e, e.Rep1, "with-nobody", "held", 1, time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), false)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, address, display_name, role)
		VALUES ($1, $2, 'lena@prospect.test', 'Lena Prospect', 'from')`, message, contact)

	for _, query := range []capture.HeldThreadQuery{{Q: "lena@prospect"}, {Q: "Lena Pro"}, {ContactID: &contact}} {
		page := heldPage(t, e, e.Rep1, query)
		if len(page.Threads) != 1 || page.Threads[0].ThreadKey != "with-lena" {
			t.Errorf("%+v listed %+v, want the thread with Lena alone", query, page.Threads)
		}
	}
}

// A message the reader may not read keeps its parties from every filter, or a
// match would confirm who is on it. The row itself stays theirs to see.
func TestAFilterCannotTestThePartiesOfAWithheldMessage(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Hidden Party", &e.Rep1)
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, is_primary, source, captured_by)
		VALUES ($1, 'hidden@party.test', true, 'manual', 'human:x')`, contact)
	message := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, audience)
		VALUES ($1, 'email', 'Withheld', now(), 'gmail', $2, 'participants')`, message, "human:"+e.Rep2.String())
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, address, display_name, role)
		VALUES ($1, $2, 'hidden@party.test', 'Hidden Party', 'from')`, message, contact)
	e.WsExec(t, `INSERT INTO capture_thread_verdict (thread_key, user_id, status, attempts, first_activity_id, seen_addresses)
		VALUES ('withheld', $1, 'held', 1, $2, ARRAY['hidden@party.test'])`, e.Rep1, message)

	if page := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{}); len(page.Threads) != 1 {
		t.Fatalf("listed %d threads, want the reader's own row", len(page.Threads))
	}
	for _, query := range []capture.HeldThreadQuery{{Q: "hidden@party"}, {Q: "Hidden Party"}, {ContactID: &contact}} {
		if page := heldPage(t, e, e.Rep1, query); len(page.Threads) != 0 {
			t.Errorf("%+v matched a message the reader may not read: %+v", query, page.Threads)
		}
	}
}

// The ledger's addresses answer only where the opening message is gone. An
// archived message keeps its parties from a search, and a readable one matches
// through its participants alone.
func TestTheLedgerAnswersOnlyForAnErasedMessage(t *testing.T) {
	e := integration.Setup(t)
	day := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	archived := seedHeldThread(t, e, e.Rep1, "archived", "held", 1, day, false, "archived@party.test")
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	seedHeldThread(t, e, e.Rep1, "readable", "held", 1, day, false, "later@party.test")

	for _, term := range []string{"archived@party", "later@party"} {
		if page := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Q: term}); len(page.Threads) != 0 {
			t.Errorf("searching %q matched the ledger: %+v", term, page.Threads)
		}
	}
}

// Erasing a contact takes their address out of the ledger through the real
// writer, so a search no longer finds it. Another party on the row stays.
func TestAnErasedAddressIsNoLongerSearchable(t *testing.T) {
	e := integration.Setup(t)
	seedHeldThread(t, e, e.Rep1, "erased", "held", 1, time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC), true, "Lena@Prospect.test", "sam@northwind.test")
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return capture.NewPendingStore(InstallationDB(e.Pool)).PurgePartySourcesForEmailsTx(e.Admin(), tx, []string{" LENA@PROSPECT.TEST "})
	}); err != nil {
		t.Fatalf("erasing the address: %v", err)
	}
	if page := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Q: "lena"}); len(page.Threads) != 0 {
		t.Errorf("a search found an erased address: %+v", page.Threads)
	}
	if page := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Q: "sam@northwind"}); len(page.Threads) != 1 {
		t.Errorf("erasing one party lost the other: %+v", page.Threads)
	}
}

// Two threads at one instant page apart on the row id, each once.
func TestThreadsAtOneInstantPageEachOnce(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	seedHeldThread(t, e, e.Rep1, "first", "held", 1, at, false)
	seedHeldThread(t, e, e.Rep1, "second", "held", 1, at, false)
	limit := 1
	first := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Limit: &limit})
	second := heldPage(t, e, e.Rep1, capture.HeldThreadQuery{Limit: &limit, Cursor: first.Next})
	if len(first.Threads) != 1 || len(second.Threads) != 1 || first.Threads[0].ThreadKey == second.Threads[0].ThreadKey {
		t.Fatalf("pages listed %+v then %+v, want each thread once", first.Threads, second.Threads)
	}
	if second.Next != "" {
		t.Errorf("the last page offered a cursor %q", second.Next)
	}
}

// A cursor the list did not mint is refused, even one that names a row.
func TestTheHeldListRefusesACursorItDidNotMint(t *testing.T) {
	e := integration.Setup(t)
	incomplete, err := storekit.EncodeOpaque(map[string]ids.UUID{"id": ids.NewV7()})
	if err != nil {
		t.Fatalf("encoding a cursor: %v", err)
	}
	for _, cursor := range []string{"not-a-cursor", incomplete} {
		_, err := capture.HeldThreadsFor(purgeCtx(e, e.Rep1), InstallationDB(e.Pool), capture.HeldThreadQuery{Cursor: cursor})
		if _, malformed := errors.AsType[*storekit.MalformedCursorError](err); !malformed {
			t.Errorf("cursor %q answered %v", cursor, err)
		}
	}
}

// An over-long search is a 422 naming the field, as the contract declares.
func TestAnOverlongSearchIsRefusedByField(t *testing.T) {
	e := integration.Setup(t)
	handlers := captureSenderHandlers{db: InstallationDB(e.Pool)}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/capture/held-threads", nil).WithContext(purgeCtx(e, e.Rep1))
	q := strings.Repeat("x", capture.HeldSearchMax+1)
	handlers.ListHeldThreads(w, r, crmcontracts.ListHeldThreadsParams{Q: &q})
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), `"q"`) {
		t.Errorf("status %d: %s", w.Code, w.Body.String())
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

	noContactGrant := e.As(e.Rep1, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	_, err = capture.HeldThreadsFor(noContactGrant, InstallationDB(e.Pool), capture.HeldThreadQuery{ContactID: &lena})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("filtering with no contact grant answered %v, want not found", err)
	}

	e.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, lena)
	_, err = capture.HeldThreadsFor(purgeCtx(e, e.Rep1), InstallationDB(e.Pool), capture.HeldThreadQuery{ContactID: &lena})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("filtering by an archived contact answered %v, want not found", err)
	}
}
