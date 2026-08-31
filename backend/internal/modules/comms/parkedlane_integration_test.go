// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

// The undelivered-lane read against rows the real writers produced: staged
// with StageTx, abandoned with Park, transmitted-then-parked with
// ParkTransmitted — never hand-inserted. The predicates under test are SQL
// (the stamp, the window, ownership, the person join), which a unit double
// proves nothing about.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestParkedSendsForCarriesTheCallersAbandonedSendsOnly(t *testing.T) {
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

	// The send that was given up on.
	abandoned := e.stage(t, e.baseInput(e.activity, "abandoned@myco.test"))
	if err := e.store.Park(e.ctx, abandoned, "the mailbox is no longer send-capable"); err != nil {
		t.Fatalf("parking the delivery: %v", err)
	}

	// A send whose MESSAGE went out and whose receipt could not be written
	// wears the same status and must never reach the lane: the recipient has
	// it, and telling the sender it was never sent would be a lie that
	// invites a second copy.
	transmitted := e.stage(t, e.baseInput(e.activity, "transmitted@myco.test"))
	if err := e.store.ParkTransmitted(e.ctx, transmitted, "receipt lost", "prov-transmitted"); err != nil {
		t.Fatalf("parking the transmitted delivery: %v", err)
	}

	// A send that actually left is not on it either.
	if err := e.store.RecordSent(e.ctx, e.stage(t, e.baseInput(e.activity, "sent@myco.test")),
		connector.SendReceipt{ProviderMessageID: "prov-sent"}); err != nil {
		t.Fatalf("recording the send: %v", err)
	}

	since := e.clockValue.Add(-7 * 24 * time.Hour)
	reader := readerCtx(e.ws, e.user)
	parked, err := e.store.ParkedSendsFor(reader, since, 8)
	if err != nil {
		t.Fatalf("ParkedSendsFor: %v", err)
	}
	if len(parked) != 1 {
		t.Fatalf("ParkedSendsFor = %+v, want only the send that was given up on", parked)
	}
	got := parked[0]
	if got.ID != abandoned {
		t.Errorf("carried send %s, want the abandoned one %s", got.ID, abandoned)
	}
	if got.Reason != "the mailbox is no longer send-capable" {
		t.Errorf("reason = %q, want the dispatcher's own words", got.Reason)
	}
	if got.PersonID != person {
		t.Errorf("person = %s, want the one the activity is filed under (%s)", got.PersonID, person)
	}
	if got.Subject == "" {
		t.Error("the send's subject line did not travel, so the card cannot name the send")
	}
	if got.ParkedAt.IsZero() {
		t.Error("the send carries no time, so the lane has nothing to window or order on")
	}

	// A window opening after the park drops it: the lane ages out.
	none, err := e.store.ParkedSendsFor(reader, e.clockValue.Add(time.Hour), 8)
	if err != nil {
		t.Fatalf("ParkedSendsFor with a later window: %v", err)
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
	othersView, err := e.store.ParkedSendsFor(readerCtx(e.ws, stranger), since, 8)
	if err != nil {
		t.Fatalf("ParkedSendsFor as another person: %v", err)
	}
	if len(othersView) != 0 {
		t.Fatalf("another person reads %+v, want nothing of this caller's", othersView)
	}
}

// A caller whose role lacks the activity read grant is refused like any other
// timeline read: the subject lines of their sends are activity content.
func TestParkedSendsForRefusesAReaderWithoutTheActivityGrant(t *testing.T) {
	e := setupStore(t)
	if _, err := e.store.ParkedSendsFor(e.ctx, e.clockValue.Add(-time.Hour), 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("ParkedSendsFor without the activity grant = %v, want the permission sentinel", err)
	}
}
