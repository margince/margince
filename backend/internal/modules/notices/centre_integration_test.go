// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package notices

// The notification centre against a real database: the reader's whole history
// rather than the unread lane, paged by a token they cannot forge, settled in
// one act — and a class the seat switched off recorded like any other, listed
// in the centre and absent from the two surfaces that interrupt somebody.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	if got := itemIDs(page); !slices.Equal(got, want) {
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
	if got := itemIDs(first); !slices.Equal(got, want[:2]) {
		t.Fatalf("the first page holds %v, want %v", got, want[:2])
	}
	if first.NextCursor == "" {
		t.Fatal("a page with a remainder offered no continuation token — the rest of the history is unreachable")
	}
	second, err := e.store.ListFor(e.asUser(e.recipient), 2, first.NextCursor)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if got := itemIDs(second); !slices.Equal(got, want[2:]) {
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
	// A stage move this reader made themselves: unread in the table, and shown
	// to them nowhere. Both halves of what the settle does with it are asserted
	// below, because they pull in opposite directions.
	ownStageMove := e.seedOwnStageMove(t)

	// The centre never shows it, so the count below cannot be about it.
	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatalf("ListFor: %v", err)
	}
	for _, item := range page.Items {
		if item.ID == ownStageMove {
			t.Fatal("the centre shows a stage change this reader made themselves")
		}
	}

	settled, err := e.store.MarkAllRead(e.asUser(e.recipient))
	if err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	// The LINES THE READER SAW, and not every row the statement touched: a
	// figure a reader is ever shown may not exceed what they saw, and "3
	// notifications marked read" over two visible lines is a lie to them.
	if settled != 2 {
		t.Fatalf("MarkAllRead answered %d, want the 2 notices the reader was actually shown", settled)
	}
	// And yet the hidden one IS settled. Leaving it unread would strand a row
	// in the partial unread index that no act of the reader's could clear.
	var stillUnread bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT read_at IS NULL FROM notice WHERE id = $1`, ownStageMove).Scan(&stillUnread); err != nil {
		t.Fatalf("reading the hidden notice's read state: %v", err)
	}
	if stillUnread {
		t.Errorf("a stage change the reader made themselves is still unread after settling everything — " +
			"nothing they can do will ever clear it")
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
	page, err = e.store.ListFor(e.asUser(e.recipient), 10, "")
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

	// One ledger entry and no announcement: one act settled N rows, and N
	// notice.read events for one tap would flood the bus.
	//
	// The entry is about the SEAT — the act settled a set of notices and no
	// single one of them, so an entry naming a notice id would name one that
	// does not exist.
	var audits, events int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT (SELECT count(*) FROM audit_log
		         WHERE entity_type = 'user' AND entity_id = $1 AND action = 'update'),
		       (SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'notice.read')`,
		e.recipient).Scan(&audits, &events); err != nil {
		t.Fatalf("reading what the settle wrote: %v", err)
	}
	// And the entry itself: the figure it covers, and the lane on either side
	// of the act. Read as one row rather than as three subqueries over the
	// same one, so the three numbers cannot come from different entries.
	var count, unreadBefore, unreadAfter int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT coalesce((after->>'count')::int, -1),
		       coalesce((before->>'unread')::int, -1),
		       coalesce((after->>'unread')::int, -1)
		  FROM audit_log
		 WHERE entity_type = 'user' AND entity_id = $1 AND action = 'update'
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`,
		e.recipient).Scan(&count, &unreadBefore, &unreadAfter); err != nil {
		t.Fatalf("reading the ledger entry the settle wrote: %v", err)
	}
	// It says what it changed FROM: three notices stood unread, and none does
	// now. Without the before-image the entry records that a seat's lane moved
	// and nothing can say what it moved from.
	if unreadBefore != 3 || unreadAfter != 0 {
		t.Errorf("the entry images the lane as %d unread before and %d after, "+
			"want the 3 that stood unread and none left", unreadBefore, unreadAfter)
	}
	if audits != 1 {
		t.Errorf("settling everything wrote %d ledger entries, want exactly 1 — including for the second tap that moved nothing", audits)
	}
	var dangling int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log a
		 WHERE a.entity_type = 'notice' AND a.action = 'update'
		   AND NOT EXISTS (SELECT 1 FROM notice n WHERE n.id = a.entity_id)`).Scan(&dangling); err != nil {
		t.Fatalf("looking for ledger entries about notices that do not exist: %v", err)
	}
	if dangling != 0 {
		t.Errorf("%d ledger entries name a notice that does not exist — an operator resolving one gets nothing", dangling)
	}
	if events != 0 {
		t.Errorf("settling everything announced %d notice.read events, want none", events)
	}
	// THREE, where the reader was answered two: the ledger records what the
	// write did, including the row the reader was never shown, because it is
	// what an operator reads to know what happened to the table.
	if count != 3 {
		t.Errorf("the ledger entry records %d settled notices, want all 3 that moved — "+
			"the count is what says how much one entry covers", count)
	}
}

// A CLASS THIS SEAT SWITCHED OFF IS RECORDED AND NOT DELIVERED, which is the
// whole of what a preference governs.
//
// Dropping the row instead would be the product deciding that switching a lane
// off means never being told — and the line is still a thing that changed what
// this reader believes about their data, which is the one kind of notice the
// product raises at all. So the centre lists it, and the two surfaces that
// interrupt somebody — the Worklist's lane and the badge over it — do not
// count it.
func TestAMutedClassIsListedInTheCentreAndReachesNoLane(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.SaveNotificationPreference(
		e.asUser(e.recipient), classAutomation, DeliveryOff); err != nil {
		t.Fatalf("switching a class off: %v", err)
	}

	muted, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "automation", Subject: "A deal you own changed stage",
	})
	if err != nil {
		t.Fatalf("Create of a muted notice: %v", err)
	}
	if muted.IsZero() {
		t.Fatal("a muted class was discarded — a preference decides delivery, never whether the line exists")
	}
	// A class the seat did NOT switch off, so every difference below is the
	// preference and not the lane failing to carry anything at all.
	heard := e.seedNotice(t, e.recipient, "A lead's first response is overdue")

	// The row, its ledger entry and its announcement all stand: a muted notice
	// is an ordinary write, and anything less would leave the centre showing a
	// line the audit spine cannot account for.
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
		t.Fatalf("counting what the muted notice left behind: %v", err)
	}
	if rows != 1 || audits != 1 || events != 1 {
		t.Errorf("a muted notice left %d rows, %d ledger entries and %d announcements, want one of each",
			rows, audits, events)
	}

	// The centre is where it stays findable, newest first beside the rest.
	page, err := e.store.ListFor(e.asUser(e.recipient), 10, "")
	if err != nil {
		t.Fatalf("ListFor: %v", err)
	}
	if got := itemIDs(page); !slices.Contains(got, muted) {
		t.Errorf("the centre holds %v, and the muted notice (%s) is not among them", got, muted)
	}
	// The badge counts the lane and not the list, so it is the narrower of the
	// two: one interruption standing, over two unread lines on the page.
	if page.UnreadCount != 1 {
		t.Errorf("the badge counts %d, want only the 1 notice this reader did not mute", page.UnreadCount)
	}

	// The Worklist's lane is the other surface that interrupts, and it declines
	// the muted line for the same reason the badge does.
	lane, err := e.store.UnreadFor(e.asUser(e.recipient), 10)
	if err != nil {
		t.Fatalf("UnreadFor: %v", err)
	}
	if len(lane) != 1 || lane[0].ID != heard {
		t.Errorf("the lane holds %+v, want only the notice this reader did not mute (%s)", lane, heard)
	}
}

// Settling the centre clears the muted lines too, and says it cleared only what
// the reader could see.
//
// Leaving them unread would strand rows in the partial unread index that no act
// of the reader's could reach: they are hidden from the badge, so nothing they
// can press would ever settle them. The FIGURE is the other half — it may not
// exceed the badge it replaces, or a reader watching "1" is told three things
// were answered.
func TestSettlingTheCentreClearsMutedLinesAndCountsOnlyTheBadge(t *testing.T) {
	e := setupNotices(t)
	if _, err := e.store.SaveNotificationPreference(
		e.asUser(e.recipient), classAutomation, DeliveryOff); err != nil {
		t.Fatalf("switching a class off: %v", err)
	}
	if _, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "automation", Subject: "A deal you own changed stage",
	}); err != nil {
		t.Fatalf("raising the muted notice: %v", err)
	}
	e.seedNotice(t, e.recipient, "A lead's first response is overdue")

	shown, err := e.store.MarkAllRead(e.asUser(e.recipient))
	if err != nil {
		t.Fatalf("settling the centre: %v", err)
	}
	if shown != 1 {
		t.Errorf("the settle reports %d notices, want the 1 the badge was carrying", shown)
	}

	var stillUnread int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM notice WHERE recipient_user_id = $1 AND read_at IS NULL`,
		e.recipient).Scan(&stillUnread); err != nil {
		t.Fatalf("counting what the settle left unread: %v", err)
	}
	if stillUnread != 0 {
		t.Errorf("%d notice(s) are still unread after the settle — a muted line nothing shows is one nobody can clear", stillUnread)
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

// A token this package never minted is the CALLER's mistake, and the sentinel
// has to survive the trip out: httperr answers 422 for MalformedCursorError and
// 500 for everything else, so a wrapped one would send an operator looking for
// an outage that is not there.
func TestNotificationCentreRefusesACursorItNeverMinted(t *testing.T) {
	e := setupNotices(t)
	_, err := e.store.ListFor(e.asUser(e.recipient), 10, "not-a-token-this-package-minted")
	var malformed *storekit.MalformedCursorError
	if !errors.As(err, &malformed) {
		t.Fatalf("a forged cursor answered %v, want the malformed-cursor refusal the transport turns into 422", err)
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

// seedOwnStageMove records a stage change the recipient made themselves — the
// one notice in the table that is theirs and that no read of theirs shows.
//
// Through the real writer with the origin a stage-change delivery carries, so
// the row matches what automation.stageChangeNotify actually produces rather
// than what this test believes the filter looks for.
func (e *noticeEnv) seedOwnStageMove(t *testing.T) ids.UUID {
	t.Helper()
	origin := &crmcontracts.NoticeOrigin{
		EventId:    openapi_types.UUID(ids.NewV7()),
		ActorType:  "human",
		ActorId:    "human:" + e.recipient.String(),
		OccurredAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}
	origin.StageChange = &struct {
		FromName *string `json:"from_name,omitempty"`
		ToName   *string `json:"to_name,omitempty"`
	}{}
	id, err := e.store.Create(e.engineCtx(), NewNotice{
		Recipient: e.recipient, Kind: "automation", Subject: "A deal you own changed stage",
		Origin: origin, Target: Target{Type: "deal", ID: ids.NewV7()},
		DedupeKey: "stage_change_notify:" + ids.NewV7().String(),
	})
	if err != nil {
		t.Fatalf("seeding the reader's own stage move: %v", err)
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

// The centre is history: an overtaken line is still listed, and it says why it
// stopped standing rather than pretending the reader answered it.
func TestTheCentreListsAnOvertakenLineAndNamesWhoDecided(t *testing.T) {
	e := setupNotices(t)
	approval := ids.New[ids.ApprovalKind]()
	line := e.stageApprovalLine(t, e.recipient, approval)
	decider := e.other
	if _, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: approval, By: &decider}}); err != nil {
		t.Fatalf("a colleague deciding the approval: %v", err)
	}

	item := lineOf(t, e.centreThrough(e.asUser(e.recipient), t, seatsNamed{decider.UUID: "Dana Fuchs"}), line)
	if item.OvertakenAt == nil {
		t.Fatal("the line comes back still standing — the reader is told a decision waits on them " +
			"when a colleague already made it")
	}
	if item.OvertakenBy == nil || ids.UUID(*item.OvertakenBy) != decider.UUID {
		t.Fatalf("the line names %v as the decider, want %s", item.OvertakenBy, decider)
	}
	if item.OvertakenByName == nil || *item.OvertakenByName != "Dana Fuchs" {
		t.Errorf("the line names the decider %v, want the display name a reader recognises", item.OvertakenByName)
	}
	// The two facts are independent, and this is the one a client rendering
	// read_at alone gets wrong: nobody opened this line.
	if item.ReadAt != nil {
		t.Errorf("the line reads as settled at %v — being overtaken is not the reader answering it", item.ReadAt)
	}
}

// A colleague who has since left resolves to no name, and the id still stands.
// This is the null path the contract promises.
func TestAnOvertakenLineWhoseDeciderHasLeftCarriesNoName(t *testing.T) {
	e := setupNotices(t)
	approval := ids.New[ids.ApprovalKind]()
	line := e.stageApprovalLine(t, e.recipient, approval)
	departed := e.other
	if _, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: approval, By: &departed}}); err != nil {
		t.Fatalf("the colleague deciding before they left: %v", err)
	}

	// The directory holds nobody, which is what it answers for a seat the
	// installation no longer has.
	item := lineOf(t, e.centreThrough(e.asUser(e.recipient), t, seatsNamed{}), line)
	if item.OvertakenByName != nil {
		t.Errorf("the line names %q for a colleague the directory no longer holds", *item.OvertakenByName)
	}
	// The id is the fact the row holds; only the name was ever a lookup, so
	// losing the lookup may not lose the line or the id under it.
	if item.OvertakenBy == nil || ids.UUID(*item.OvertakenBy) != departed.UUID {
		t.Fatalf("the line names %v as the decider, want %s — the row says so whoever has left", item.OvertakenBy, departed)
	}
	if item.OvertakenAt == nil {
		t.Error("the line comes back still standing because its decider has left")
	}
}

// The reader is told who decided only because they could have decided it
// themselves — that is what put the line in front of them.
//
// Nothing in the centre asks who may decide: the fan-out asked once, per seat,
// and wrote a line only where the answer was yes. So the boundary is the set of
// lines, and a seat outside it holds nothing for a decider's name to ride on.
func TestOnlyASeatThatCouldHaveDecidedIsToldWhoDid(t *testing.T) {
	e := setupNotices(t)
	bystander := e.seedSeat(t)
	approval := ids.New[ids.ApprovalKind]()
	e.stageApprovalLine(t, e.recipient, approval)
	e.stageApprovalLine(t, e.other, approval)
	// The bystander's centre is not empty, so a silent one below is the
	// boundary holding and not the read failing to carry anything at all.
	e.seedNotice(t, bystander, "A lead's first response is overdue")
	decider := e.other
	if _, err := e.store.OvertakeApprovalNotices(e.sweepCtx(),
		[]Overtaking{{Approval: approval, By: &decider}}); err != nil {
		t.Fatalf("a colleague deciding the approval: %v", err)
	}

	named := seatsNamed{decider.UUID: "Dana Fuchs"}
	page := e.centreThrough(e.asUser(bystander), t, named)
	if len(page.Items) != 1 {
		t.Fatalf("the bystander's centre holds %d lines, want the 1 unrelated notice they were sent", len(page.Items))
	}
	for _, item := range page.Items {
		if item.OvertakenBy != nil || item.OvertakenByName != nil {
			t.Errorf("a seat that was never asked to decide this approval is told %v decided it",
				item.OvertakenByName)
		}
	}
	// And the seat who COULD have decided is told, so the case above is the
	// boundary and not the name failing to resolve for anybody.
	told := e.centreThrough(e.asUser(e.recipient), t, named)
	if len(told.Items) != 1 || told.Items[0].OvertakenByName == nil {
		t.Fatalf("the seat who could have decided is told %+v, want the decider's name", told.Items)
	}
}

// seatsNamed is the directory read the centre resolves a decider through. A
// seat it holds has a name and one it does not is simply absent, which is what
// identity.SeatNames answers for a colleague who has left — that module holds
// the rule against real rows, and what these cases turn on is what the centre
// does with either answer.
type seatsNamed map[ids.UUID]string

func (s seatsNamed) SeatNames(_ context.Context, seats []ids.UserID) (map[ids.UUID]string, error) {
	named := map[ids.UUID]string{}
	for _, seat := range seats {
		if name, held := s[seat.UUID]; held {
			named[seat.UUID] = name
		}
	}
	return named, nil
}

// centreThrough reads the centre through the TRANSPORT, because the decider's
// name is resolved there: ListFor is a store method holding no collaborator to
// ask, so a test stopping at it would prove nothing about what a reader sees.
func (e *noticeEnv) centreThrough(ctx context.Context, t *testing.T, named seatsNamed) crmcontracts.NotificationPage {
	t.Helper()
	limit := 10
	rec := httptest.NewRecorder()
	NewHandlers(e.store, teammatesSaying(false), named).ListNotices(
		rec, httptest.NewRequest(http.MethodGet, "/v1/notifications", nil).WithContext(ctx),
		crmcontracts.ListNoticesParams{Limit: &limit})
	if rec.Code != http.StatusOK {
		t.Fatalf("the centre answered %d: %s", rec.Code, rec.Body.String())
	}
	var page crmcontracts.NotificationPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decoding the centre: %v", err)
	}
	return page
}

// seedSeat adds a colleague beyond the two the environment starts with, for the
// cases that need a seat the fan-out never wrote to.
func (e *noticeEnv) seedSeat(t *testing.T) ids.UserID {
	t.Helper()
	seat := ids.New[ids.UserKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Rep')`,
		seat, "rep-"+seat.String()+"@notices.test"); err != nil {
		t.Fatalf("seeding a colleague: %v", err)
	}
	return seat
}

// lineOf is the one line under test, failing rather than returning a zero item:
// every assertion after it is about fields, and a zero value would read as the
// feature being absent instead of the line being.
func lineOf(t *testing.T, page crmcontracts.NotificationPage, id ids.UUID) crmcontracts.NotificationItem {
	t.Helper()
	for _, item := range page.Items {
		if ids.UUID(item.Id) == id {
			return item
		}
	}
	t.Fatalf("the centre holds %d line(s) and none of them is %s", len(page.Items), id)
	return crmcontracts.NotificationItem{}
}
