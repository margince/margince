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
	bounced, err := e.store.HardBouncesFor(e.ctx, since, 8)
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
		t.Errorf("person = %s, want the one the activity is filed under (%s)", got.PersonID, person)
	}
	if got.Subject == "" {
		t.Error("the send's subject line did not travel, so the card cannot name the send")
	}

	// A window opening after the report drops it: the lane ages out.
	none, err := e.store.HardBouncesFor(e.ctx, e.clockValue.Add(time.Hour), 8)
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
	othersView, err := e.store.HardBouncesFor(actorCtx(e.ws, stranger), since, 8)
	if err != nil {
		t.Fatalf("HardBouncesFor as another person: %v", err)
	}
	if len(othersView) != 0 {
		t.Fatalf("another person reads %+v, want nothing of this caller's", othersView)
	}
}

func TestHardBouncesForRefusesACallerWithNoPersonBehindIt(t *testing.T) {
	e := setupStore(t)
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	if _, err := e.store.HardBouncesFor(ctx, e.clockValue.Add(-time.Hour), 8); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("HardBouncesFor with no person = %v, want the permission sentinel", err)
	}
}
