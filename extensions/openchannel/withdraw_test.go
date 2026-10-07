// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// archived is the bus delivery this unit listens for.
func archived() extension.Delivery {
	return extension.Delivery{
		EventID:    "9c1f5b30-4a72-4e18-8d05-3b6e2c7a1f94",
		Type:       "activity.archived",
		Entity:     extension.EntityRef{Type: "activity", ID: landedActivity},
		OccurredAt: signedAt,
	}
}

// The declaration has to name the event the handler acts on. A type this
// installation cannot ROUTE is refused at boot; a type it can route and nothing
// publishes simply never arrives, which is the silence this pairing prevents.
func TestTheSubscriptionNamesTheEventTheHandlerActsOn(t *testing.T) {
	t.Parallel()
	declared := New().Subscriptions
	if len(declared) != 1 {
		t.Fatalf("the unit declares %d subscriptions; this test knows about one", len(declared))
	}
	if declared[0].Handle == nil {
		t.Fatal("the declared subscription carries no handler, so a landed request's claim stays true about an entry nobody can see")
	}
	if declared[0].Events[0] != archived().Type {
		t.Fatalf("the declaration listens for %q and the handler acts on %q", declared[0].Events[0], archived().Type)
	}
}

// Archiving the timeline entry takes the entry away; without this the queue row
// keeps saying a message reached the CRM when nothing on any screen shows it did.
func TestArchivingTheEntryWithdrawsEveryRequestThatLandedIt(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()
	rt.tx.queryRows = [][]any{{firstRequestID}, {secondRequestID}}
	if err := withdrawCaptured(context.Background(), rt, archived()); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	sql, args := rt.tx.statementMentioning(t, "activity_id = $1::uuid")
	// Matching on `ingested` is what makes a redelivery a no-op, which the bus
	// requires: the entries are at-least-once. The shape is asserted before the
	// arguments are read, so a statement that dropped the predicate reports what
	// it did rather than running off the end of its own argument list.
	if !strings.Contains(sql, "state = $3") || len(args) != 3 {
		t.Fatalf("the update does not restrict itself to landed requests:\n%s\nbound %v", sql, args)
	}
	if args[0] != landedActivity || args[1] != stateWithdrawn || args[2] != stateLanded {
		t.Fatalf("it withdrew activity %v from state %v to state %v", args[0], args[2], args[1])
	}
	if len(rt.tx.audited) != 2 {
		t.Fatalf("%d ledger row(s) for two withdrawn requests — a loop that handled one leaves the others claiming an entry that is gone", len(rt.tx.audited))
	}
	if rt.tx.published[0].Verb != eventRequestWithdrawn {
		t.Fatalf("it published %q", rt.tx.published[0].Verb)
	}
}

// A redelivery matches nothing, writes no ledger row and publishes nothing. An
// audit trail of writes that did not happen would be worse than no trail at all.
func TestARedeliveryWritesNothing(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()
	if err := withdrawCaptured(context.Background(), rt, archived()); err != nil {
		t.Fatalf("a redelivery reported %v", err)
	}
	if len(rt.tx.audited) != 0 || len(rt.tx.published) != 0 {
		t.Fatal("a redelivery that matched nothing still recorded a change")
	}
}

// A delivery this handler cannot act on is ACKED rather than failed: failing it
// puts it back in the pending set forever, and says "this failed" about an event
// that is simply not this handler's business.
func TestADeliveryThisHandlerCannotActOnIsAcked(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		event extension.Delivery
	}{
		{"a subject that is not an activity", extension.Delivery{
			Type: "activity.archived", Entity: extension.EntityRef{Type: "contact", ID: landedActivity},
		}},
		{"an event naming no subject at all", extension.Delivery{Type: "activity.archived"}},
		{"a subject id that is not a uuid", extension.Delivery{
			Type: "activity.archived", Entity: extension.EntityRef{Type: "activity", ID: "not-an-id"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rt := newRuntime().unattended()
			if err := withdrawCaptured(context.Background(), rt, tc.event); err != nil {
				t.Fatalf("it was answered %v, which re-delivers forever", err)
			}
			if len(rt.tx.statements) != 0 {
				t.Fatalf("it issued a statement for an event it cannot act on:\n%s", strings.Join(rt.tx.statements, "\n"))
			}
		})
	}
}

// THE TWO ARRIVAL ORDERS END THE SAME WAY, which is the whole point of the
// marker. Archiving after the landing withdraws the row directly; archiving
// before it leaves the id where the landing looks, and the landing withdraws it
// instead of claiming it.
//
// ASSERTED ON THE STATEMENTS, because this unit's suite has no database: the
// fake records SQL and cannot run it, so the end state is not reachable from
// here. What these hold is that each order issues the write the other order
// depends on — the half a reader cannot check by looking at one path alone.
func TestAnArchiveBeforeAnyLandingIsKeptForTheLandingToAnswer(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()
	// No rows come back, so nothing has claimed this activity yet — the drain
	// is between Ingest and its landing write, or it was never ours.
	if err := withdrawCaptured(context.Background(), rt, archived()); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	sql, args := rt.tx.statementMentioning(t, archivedFirstTable)
	if !strings.Contains(sql, "ON CONFLICT") {
		t.Fatalf("the marker is not written idempotently, and the bus is at-least-once:\n%s", sql)
	}
	if len(args) != 1 || args[0] != landedActivity {
		t.Fatalf("the marker records %v; it has to name the archived activity", args)
	}
	if len(rt.tx.audited) != 0 || len(rt.tx.published) != 0 {
		t.Fatal("recording a marker announced a change; nothing was withdrawn yet")
	}
}

// An archive that DID match writes no marker: the request is already withdrawn,
// and a marker left behind would withdraw the next landing onto that activity
// for a reason that has already been dealt with.
func TestAnArchiveThatWithdrewSomethingKeepsNoMarker(t *testing.T) {
	t.Parallel()
	rt := newRuntime().unattended()
	rt.tx.queryRows = [][]any{{firstRequestID}}

	if err := withdrawCaptured(context.Background(), rt, archived()); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}

	for _, stmt := range rt.tx.statements {
		if strings.Contains(stmt, archivedFirstTable) {
			t.Fatalf("an archive that withdrew a request also kept a marker:\n%s", stmt)
		}
	}
}
