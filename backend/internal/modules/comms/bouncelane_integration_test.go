// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

// The bounce-lane read against rows the real writers produced: staged with
// StageTx, sent with RecordSent, marked by RecordBounce — never hand-inserted.
// The predicates under test are SQL (kind, window, ownership, the person
// join), which a unit double proves nothing about.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestHardBouncesForCarriesTheCallersHardBouncesOnly(t *testing.T) {
	e := setupStore(t)

	// The send that hard-bounced, filed under a person.
	person := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, source, captured_by) VALUES ($1, 'Anna Weber', 'test', 'human:x')`, person); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, person_id) VALUES ($1, $2, 'person', $3)`,
		ids.NewV7(), e.activity, person); err != nil {
		t.Fatal(err)
	}
	// A second, capture-private person on the SAME activity, owned by someone
	// else, with a UUID sorting below the visible one: the join must never
	// pick them, because owning the send licenses nothing about who its
	// activity touches.
	otherOwner := ids.New[ids.UserKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Owner')`,
		otherOwner, "owner-"+otherOwner.String()+"@comms.test"); err != nil {
		t.Fatal(err)
	}
	// Minted BEFORE the visible person below would sort first under the
	// join's ORDER BY, but v7 ids are time-ordered — so pin a literal that
	// sorts below every fresh id instead, making the private link the one
	// an unscoped join would pick.
	hidden := ids.MustParse("00000000-0000-7000-8000-000000000001")
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, source, captured_by, visibility, owner_id) VALUES ($1, 'Private Contact', 'test', 'human:x', 'owner', $2)`,
		hidden, otherOwner); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, person_id) VALUES ($1, $2, 'person', $3)`,
		ids.NewV7(), e.activity, hidden); err != nil {
		t.Fatal(err)
	}
	hard := e.sentDelivery(t, "hard@myco.test")
	if marked, err := e.store.RecordBounce(e.asCapturingConnector(e.user.UUID),
		bounceFor("hard@myco.test", connector.BounceHard, "550 5.1.1 user unknown")); err != nil || !marked {
		t.Fatalf("recording the hard bounce: marked=%v err=%v", marked, err)
	}

	// A soft bounce stays a stamp on the row and never reaches the lane.
	e.sentDelivery(t, "soft@myco.test")
	if marked, err := e.store.RecordBounce(e.asCapturingConnector(e.user.UUID),
		bounceFor("soft@myco.test", connector.BounceSoft, "452 mailbox full")); err != nil || !marked {
		t.Fatalf("recording the soft bounce: marked=%v err=%v", marked, err)
	}

	since := e.clockValue.Add(-7 * 24 * time.Hour)
	reader := readerCtx(e.ws, e.user)
	bounced, err := e.store.HardBouncesFor(reader, since, 8)
	if err != nil {
		t.Fatalf("HardBouncesFor: %v", err)
	}
	if len(bounced) != 1 {
		t.Fatalf("HardBouncesFor = %+v, want only the hard bounce", bounced)
	}
	got := bounced[0]
	if got.ID != hard {
		t.Errorf("carried send %s, want the hard-bounced one %s", got.ID, hard)
	}
	if got.Reason != "550 5.1.1 user unknown" {
		t.Errorf("reason = %q, want the receiving side's own words", got.Reason)
	}
	if got.PersonID != person {
		t.Errorf("person = %s, want the VISIBLE one the activity is filed under (%s), never the private link", got.PersonID, person)
	}
	if got.Subject == "" {
		t.Error("the send's subject line did not travel, so the card cannot name the send")
	}

	// A window opening after the report drops it: the lane ages out.
	none, err := e.store.HardBouncesFor(reader, e.clockValue.Add(time.Hour), 8)
	if err != nil {
		t.Fatalf("HardBouncesFor with a later window: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("aged window = %+v, want empty", none)
	}

	// Another person's context reads nothing of this caller's.
	stranger := ids.New[ids.UserKind]()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Other')`,
		stranger, "other-"+stranger.String()+"@comms.test"); err != nil {
		t.Fatal(err)
	}
	othersView, err := e.store.HardBouncesFor(readerCtx(e.ws, stranger), since, 8)
	if err != nil {
		t.Fatalf("HardBouncesFor as another person: %v", err)
	}
	if len(othersView) != 0 {
		t.Fatalf("another person reads %+v, want nothing of this caller's", othersView)
	}
}

// readerCtx is actorCtx plus the activity read grant the lane requires — the
// permission every seat that sees the Worklist carries.
func readerCtx(ws ids.UUID, user ids.UserID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ws)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user.UUID,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A caller whose role lacks the activity read grant is refused like any other
// timeline read: the subject lines of their sends are activity content.
func TestHardBouncesForRefusesAReaderWithoutTheActivityGrant(t *testing.T) {
	e := setupStore(t)
	if _, err := e.store.HardBouncesFor(e.ctx, e.clockValue.Add(-time.Hour), 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("HardBouncesFor without the activity grant = %v, want the permission sentinel", err)
	}
}

// WHICH address refused, carried from the row the send was written to.
//
// A database test rather than a unit one because the address is read from
// `comms_outbound.recipients`, a jsonb column, through a scan whose column
// count has to agree with the statement's. Nothing in Go checks that agreement:
// add a column to the SELECT and forget the scan target and the read fails at
// runtime, on a lane whose whole job is to report other failures.
//
// Seeded through sentDelivery — the same writer the lane reads behind — so the
// recipients column holds what a real send puts there rather than what this
// test thinks it should.
func TestHardBouncesForNamesTheAddressThatRefused(t *testing.T) {
	e := setupStore(t)
	person := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO person (id, full_name, source, captured_by) VALUES ($1, 'Anna Weber', 'test', 'human:x')`, person); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(),
		`INSERT INTO activity_link (id, activity_id, entity_type, person_id) VALUES ($1, $2, 'person', $3)`,
		ids.NewV7(), e.activity, person); err != nil {
		t.Fatal(err)
	}
	// Staged with an EXPLICIT recipient rather than through sentDelivery, whose
	// shared fixture hardcodes buyer@example.com. A test seeded from that would
	// pass against a read that returned the constant, and prove nothing about
	// the address travelling. The message id is what the bounce is matched on;
	// the recipient is the separate fact under test.
	// TWO recipients, and the one that bounced is NOT the first. A read taking
	// `recipients->>0` would name the bystander — the exact defect the
	// bounce_recipient column was added for, whose migration says a send with a
	// CC would otherwise mark every address on it as the one that refused.
	staged := e.stage(t, func() StageInput {
		in := e.baseInput(e.activity, "dana-msg")
		in.Recipients = []string{"colleague@turbinenbau.de", "dana@turbinenbau.de"}
		return in
	}())
	if err := e.store.RecordSent(e.ctx, staged,
		connector.SendReceipt{ProviderMessageID: "prov-dana-msg"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}
	// The report names the same address the send was aimed at. bounceFor's
	// shared fixture hardcodes buyer@example.com, and RecordBounce matches on
	// the pair, so borrowing it here would leave the bounce unmatched.
	report := connector.BounceReport{
		MessageID: "dana-msg", Recipient: "dana@turbinenbau.de",
		Kind: connector.BounceHard, Reason: "550 5.1.1 user unknown",
	}
	if marked, err := e.store.RecordBounce(
		e.asCapturingConnector(e.user.UUID), report); err != nil || !marked {
		t.Fatalf("recording the hard bounce: marked=%v err=%v", marked, err)
	}

	bounced, err := e.store.HardBouncesFor(
		readerCtx(e.ws, e.user), e.clockValue.Add(-7*24*time.Hour), 8)
	if err != nil {
		t.Fatalf("reading the bounce lane: %v", err)
	}
	if len(bounced) != 1 {
		t.Fatalf("the lane carries %d bounces, want the one just recorded", len(bounced))
	}
	if bounced[0].Recipient != "dana@turbinenbau.de" {
		t.Errorf("the bounce names %q as the address that refused, want dana@turbinenbau.de — "+
			"without it a reader opening a contact with three addresses cannot tell which is dead",
			bounced[0].Recipient)
	}
}
