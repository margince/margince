// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A message set aside as "not mine" comes back when the record changes hands.
//
// The judgement carries no moment and nothing used to end it, so a rep who
// handed a deal on and later inherited it back still had the thread hidden and
// had to remember they once dismissed it. What ends it is the hand-off, and
// that is a claim about two readers over one fixture — which is why it is here
// rather than in a unit test: the anti-join in the waiting lane is what a
// reader actually experiences, and asserting the delete alone would prove a row
// went away without proving anybody's day changed.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// seedOwnedPerson creates a person a message can be filed under, owned by the
// named rep — the owner is the whole subject here, and the shared seed leaves
// it null.
func seedOwnedPerson(t *testing.T, e *Env, owner ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := OwnerConn(t).Exec(context.Background(), `
		INSERT INTO person (id, full_name, owner_id, source, captured_by, version, created_at, updated_at)
		VALUES ($1, 'Handed Over', $2, 'system', $3, 1, now(), now())`,
		id, owner, "human:"+e.AdminUser.String()); err != nil {
		t.Fatalf("seeding the person a thread is filed under: %v", err)
	}
	return id
}

func TestAHandOffGivesTheMessageBackToEveryoneButItsNewOwner(t *testing.T) {
	e := Setup(t)
	// The person is owned by Rep2 — they are who it was just handed to.
	person := seedOwnedPerson(t, e, e.Rep2)
	seedWaitingMessageLinked(t, e, "thread-handoff", "inbound", "Re: the renewal",
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, "Re: the renewal")

	store := activities.NewStore(e.DB())
	// Both readers put it down: the colleague who does not own it, and the
	// incoming owner who dismissed it before it was theirs.
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2} {
		if err := store.SetMessageNotMine(e.As(rep, nil, AdminPerms), id); err != nil {
			t.Fatalf("a rep handing the message on: %v", err)
		}
	}
	if waitsFor(t, e, e.Rep1, "Re: the renewal") || waitsFor(t, e, e.Rep2, "Re: the renewal") {
		t.Fatal("a not-mine did not take, so this proves nothing about ending one")
	}

	cleared, err := store.ClearNotMineOnHandOff(e.AutomationCtx(e.AdminUser), datasource.EntityPerson, person)
	if err != nil {
		t.Fatalf("re-arming on the hand-off: %v", err)
	}
	if cleared != 1 {
		t.Errorf("cleared %d set-aside(s), want the one belonging to the reader who is not the owner", cleared)
	}

	if !waitsFor(t, e, e.Rep1, "Re: the renewal") {
		t.Error("the colleague's dismissal survived the hand-off: they set it aside because somebody " +
			"else owned the record, and that arrangement has ended")
	}
	if waitsFor(t, e, e.Rep2, "Re: the renewal") {
		t.Error("the incoming owner's OWN dismissal was withdrawn for them — being handed a record " +
			"does not undo a statement they made about it themselves")
	}
}

// A record nobody owns clears nothing. An unowned record is ordinarily one
// whose routing has not run yet, and re-arming every Worklist on the way past
// would be work nobody asked for.
func TestAnUnownedRecordEndsNobodysJudgement(t *testing.T) {
	e := Setup(t)
	person := seedWaitingPerson(t, e)
	seedWaitingMessageLinked(t, e, "thread-unowned", "inbound", "Re: the pilot",
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, "Re: the pilot")

	store := activities.NewStore(e.DB())
	if err := store.SetMessageNotMine(e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("the rep handing the message on: %v", err)
	}

	cleared, err := store.ClearNotMineOnHandOff(e.AutomationCtx(e.AdminUser), datasource.EntityPerson, person)
	if err != nil {
		t.Fatalf("re-arming on an unowned record: %v", err)
	}
	if cleared != 0 {
		t.Errorf("cleared %d set-aside(s) on a record nobody owns", cleared)
	}
	if waitsFor(t, e, e.Rep1, "Re: the pilot") {
		t.Error("an unowned record put the message back on a rep's day")
	}
}

// A SNOOZE IS NOT A CLAIM ABOUT OWNERSHIP, and a hand-off must not lift one.
//
// It names a moment still ahead and ends on that moment. Clearing it here would
// hand somebody back work they had deliberately put down until Thursday, which
// is a different judgement about a different question.
func TestAHandOffLeavesASnoozeAlone(t *testing.T) {
	e := Setup(t)
	person := seedOwnedPerson(t, e, e.Rep2)
	seedWaitingMessageLinked(t, e, "thread-handoff-snooze", "inbound", "Re: the invoice",
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, "Re: the invoice")

	twoDays := waitingInstant.Add(48 * time.Hour)
	if err := atWaitingInstant(e).SnoozeMessage(
		e.As(e.Rep1, nil, AdminPerms), id, values.ReopenOnTime, &twoDays, nil); err != nil {
		t.Fatalf("the rep snoozing the message: %v", err)
	}

	cleared, err := activities.NewStore(e.DB()).ClearNotMineOnHandOff(
		e.AutomationCtx(e.AdminUser), datasource.EntityPerson, person)
	if err != nil {
		t.Fatalf("re-arming on the hand-off: %v", err)
	}
	if cleared != 0 {
		t.Errorf("cleared %d judgement(s), and none of them was a not-mine", cleared)
	}
	if waitsFor(t, e, e.Rep1, "Re: the invoice") {
		t.Error("the hand-off lifted a snooze: the rep put this down until Thursday, which is a " +
			"different judgement about a different question")
	}
}

// A HUMAN MAY NOT RE-ARM A COLLEAGUE'S SET-ASIDE.
//
// The judgements this clears belong to other readers, and the record is merely
// named by the caller. Left open, any authenticated user could put a record's
// whole thread back on every colleague's Worklist at once — and each row would
// look exactly like one a hand-off produced, which is what makes the refusal
// worth a test rather than a comment.
func TestAHumanMayNotReArmAColleaguesSetAside(t *testing.T) {
	e := Setup(t)
	person := seedOwnedPerson(t, e, e.Rep2)
	seedWaitingMessageLinked(t, e, "thread-handoff-human", "inbound", "Re: the contract",
		waitingInstant.Add(-3*24*time.Hour), person)
	id := waitingMessageID(t, e, "Re: the contract")

	store := activities.NewStore(e.DB())
	if err := store.SetMessageNotMine(e.As(e.Rep1, nil, AdminPerms), id); err != nil {
		t.Fatalf("the rep handing the message on: %v", err)
	}

	// An ADMIN, so the refusal is about who is asking rather than about what
	// they hold: no grant in this product admits this call.
	_, err := store.ClearNotMineOnHandOff(e.Admin(), datasource.EntityPerson, person)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a human re-arming a colleague's set-aside got %v, want permission denied", err)
	}
	if waitsFor(t, e, e.Rep1, "Re: the contract") {
		t.Error("the refusal did not hold: the message is back on the rep's day")
	}
}
