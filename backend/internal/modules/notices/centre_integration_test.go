// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// The notification centre against a real database: the reader's whole history
// rather than the unread lane, paged by a token they cannot forge, settled in
// one act — and a class the seat switched off never reaching the table at all,
// which is the only place suppression can be proved, because a notice the
// filter merely hides is still a row somebody can read.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestNotificationCentreShowsTheReadersOwnHistoryAndPagesIt(t *testing.T) {
	e := setupNotices(t)
	mine := []ids.UUID{
		e.seedNotice(t, e.recipient, "First"),
		e.seedNotice(t, e.recipient, "Second"),
		e.seedNotice(t, e.recipient, "Third"),
	}
	theirs := e.seedNotice(t, e.other, "A colleague's own line")
	// One of the three settled, so the page proves it carries READ notices too:
	// an unread-only lane already exists, and a centre that repeated it would
	// lose the history the moment a reader cleared it.
	if err := e.store.MarkRead(e.asUser(e.recipient), mine[1]); err != nil {
		t.Fatalf("settling one notice: %v", err)
	}

	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatalf("ListFor: %v", err)
	}
	// Newest first, and the settled one still on it.
	want := []ids.UUID{mine[2], mine[1], mine[0]}
	if got := itemIDs(page); !sameIDs(got, want) {
		t.Fatalf("the centre holds %v, want the reader's three notices newest first %v", got, want)
	}
	if page.UnreadCount != 2 {
		t.Errorf("the centre counts %d unread, want 2 — the settled notice is history, not a badge", page.UnreadCount)
	}
	for _, item := range page.Items {
		switch {
		case item.ID == mine[1] && item.ReadAt == nil:
			t.Errorf("the settled notice comes back unread — a reader cannot tell what they have already answered")
		case item.ID != mine[1] && item.ReadAt != nil:
			t.Errorf("notice %s comes back settled, and nobody settled it", item.ID)
		case item.ID == theirs:
			t.Errorf("a colleague's notice is on this reader's centre")
		}
	}
	if page.NextCursor != "" {
		t.Errorf("a page holding everything offered a continuation token %q", page.NextCursor)
	}

	// The token walks the same list: two, then the last one, with nothing
	// repeated and nothing skipped.
	first, err := e.store.ListFor(e.asUser(e.recipient), 2, "")
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if got := itemIDs(first); !sameIDs(got, want[:2]) {
		t.Fatalf("the first page holds %v, want %v", got, want[:2])
	}
	if first.NextCursor == "" {
		t.Fatal("a page with a remainder offered no continuation token — the rest of the history is unreachable")
	}
	second, err := e.store.ListFor(e.asUser(e.recipient), 2, first.NextCursor)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if got := itemIDs(second); !sameIDs(got, want[2:]) {
		t.Fatalf("the second page holds %v, want %v", got, want[2:])
	}
	if second.NextCursor != "" {
		t.Errorf("the last page offered a continuation token %q", second.NextCursor)
	}
	// The badge counts the reader's whole unread set and not the page's share
	// of it, or it would fall as the reader scrolled.
	if second.UnreadCount != 2 {
		t.Errorf("the last page counts %d unread, want the reader's whole 2", second.UnreadCount)
	}
}

func TestNotificationCentreSettlesEverythingOnceAndOnlyForItsReader(t *testing.T) {
	e := setupNotices(t)
	e.seedNotice(t, e.recipient, "First")
	e.seedNotice(t, e.recipient, "Second")
	e.seedNotice(t, e.other, "A colleague's own line")

	settled, err := e.store.MarkAllRead(e.asUser(e.recipient))
	if err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if settled != 2 {
		t.Fatalf("MarkAllRead settled %d notices, want the reader's 2", settled)
	}
	// Idempotent: a second tap on a lane already clear moves nothing and is
	// still a success, the way settling one notice twice is.
	again, err := e.store.MarkAllRead(e.asUser(e.recipient))
	if err != nil {
		t.Fatalf("second MarkAllRead: %v", err)
	}
	if again != 0 {
		t.Errorf("a second MarkAllRead settled %d notices, want none", again)
	}
	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatalf("ListFor after settling: %v", err)
	}
	if page.UnreadCount != 0 || len(page.Items) != 2 {
		t.Errorf("after settling the centre holds %d notices with %d unread, want 2 notices and no badge — "+
			"settling reads them, it does not delete them", len(page.Items), page.UnreadCount)
	}

	// A colleague's lane is untouched: "mark everything read" is scoped to the
	// caller's own row set, never to the table.
	theirs, err := e.store.UnreadFor(e.asUser(e.other), 10)
	if err != nil {
		t.Fatalf("a colleague reading their unread: %v", err)
	}
	if len(theirs) != 1 {
		t.Errorf("a colleague holds %d unread notices, want their own 1", len(theirs))
	}

	// One ledger entry carrying the count, and no announcement: one act settled
	// N rows, and N notice.read events for one tap would flood the bus.
	var audits, events, count int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_log
		         WHERE entity_type = 'notice' AND entity_id = $1 AND action = 'update'),
		       (SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'notice.read'),
		       (SELECT coalesce((after->>'count')::int, -1) FROM audit_log
		         WHERE entity_type = 'notice' AND entity_id = $1 AND action = 'update'
		         ORDER BY occurred_at DESC, id DESC LIMIT 1)`,
		e.recipient).Scan(&audits, &events, &count); err != nil {
		t.Fatalf("reading what the settle wrote: %v", err)
	}
	if audits != 1 {
		t.Errorf("settling everything wrote %d ledger entries, want exactly 1 — including for the second tap that moved nothing", audits)
	}
	if events != 0 {
		t.Errorf("settling everything announced %d notice.read events, want none", events)
	}
	if count != 2 {
		t.Errorf("the ledger entry records %d settled notices, want 2 — the count is the only thing that says how much one entry covers", count)
	}
}

// A class this seat switched off is not written at all. Filtering it on the
// read would leave the row in the table for every other reader of it — the
// digest sweep, an export, a support query — and the seat's decision would hold
// only where somebody remembered it.
func TestNotificationCentreNeverRecordsAClassTheSeatSwitchedOff(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.SaveNotificationPreference(
		e.asUser(e.recipient), classAutomation, DeliveryOff); err != nil {
		t.Fatalf("switching a class off: %v", err)
	}

	suppressed, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "automation", Subject: "A deal you own changed stage",
	})
	if err != nil {
		t.Fatalf("Create of a suppressed notice: %v", err)
	}
	if !suppressed.IsZero() {
		t.Errorf("a suppressed notice answered id %s, want the zero id — a caller holding an id for a row nobody wrote", suppressed)
	}

	// A class the seat did NOT switch off still lands, so suppression is the
	// seat's decision and not an outage.
	landed, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "lead_sla", Subject: "A lead's first response is overdue",
	})
	if err != nil {
		t.Fatalf("Create of a delivered notice: %v", err)
	}
	if landed.IsZero() {
		t.Fatal("a class the seat never switched off was suppressed")
	}

	// Nothing at all: no row, no ledger entry, no announcement. Each is a
	// separate way a suppressed notice reaches somebody.
	var rows, audits, events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND kind = 'automation'),
		       (SELECT count(*) FROM audit_log
		         WHERE entity_type = 'notice' AND action = 'create'
		           AND after->>'kind' = 'automation'),
		       (SELECT count(*) FROM event_outbox
		         WHERE envelope->>'type' = 'notice.created'
		           AND envelope->'payload'->>'kind' = 'automation')`,
		e.recipient).Scan(&rows, &audits, &events); err != nil {
		t.Fatalf("counting what the suppressed notice left behind: %v", err)
	}
	if rows != 0 || audits != 0 || events != 0 {
		t.Errorf("a suppressed notice left %d rows, %d ledger entries and %d announcements, want none of each", rows, audits, events)
	}

	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatalf("ListFor: %v", err)
	}
	if got := itemIDs(page); !sameIDs(got, []ids.UUID{landed}) {
		t.Errorf("the centre holds %v, want only the notice that was delivered (%s)", got, landed)
	}
}

// An agent carrying its grantor's id is not the grantor. Reading and settling a
// seat's notices are that human's own acts, the same ruling their notification
// settings make about the same inbox.
func TestNotificationCentreRefusesAPrincipalThatIsNotTheSeat(t *testing.T) {
	e := setupNotices(t)
	e.seedNotice(t, e.recipient, "First")

	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"an agent acting for the seat", e.asAgentFor(e.recipient)},
		{"the automation engine", e.engineCtx()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := e.store.ListFor(tc.ctx, 10, ""); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("ListFor = %v, want the permission sentinel", err)
			}
			if _, err := e.store.MarkAllRead(tc.ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("MarkAllRead = %v, want the permission sentinel", err)
			}
		})
	}
	// The refusal refused rather than half-ran: nothing was settled.
	unread, err := e.store.UnreadFor(e.asUser(e.recipient), 10)
	if err != nil {
		t.Fatalf("UnreadFor: %v", err)
	}
	if len(unread) != 1 {
		t.Errorf("the seat holds %d unread notices after the refusals, want their own 1", len(unread))
	}
}

// asAgentFor is an agent principal carrying the seat's user id — the case a
// user-id-only check would admit.
func (e *noticeEnv) asAgentFor(u ids.UserID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:planner", UserID: u.UUID,
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// seedNotice records one notice the way a system flow does, failing the test
// rather than answering a zero id: a seeded row every assertion below depends
// on is not a value to check twice.
func (e *noticeEnv) seedNotice(t *testing.T, to ids.UserID, subject string) ids.UUID {
	t.Helper()
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: to, Kind: "lead_sla", Subject: subject, Body: "seeded",
	})
	if err != nil {
		t.Fatalf("seeding %q: %v", subject, err)
	}
	if id.IsZero() {
		t.Fatalf("seeding %q answered the zero id", subject)
	}
	return id
}

func itemIDs(page CentrePage) []ids.UUID {
	out := make([]ids.UUID, 0, len(page.Items))
	for _, item := range page.Items {
		out = append(out, item.ID)
	}
	return out
}

func sameIDs(got, want []ids.UUID) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
