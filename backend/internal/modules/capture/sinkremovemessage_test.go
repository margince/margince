// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture_test

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// seatContext binds the seat a sync runs as — the contact whose connection it
// is, which is the only seat whose captured copy a removal may reach.
func seatContext(seat ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:   principal.PrincipalHuman,
		ID:     "human:" + seat.String(),
		UserID: seat,
	})
}

func aKey() connector.NaturalKey {
	return connector.NaturalKey{SourceSystem: "gmail", SourceID: "msg-1"}
}

// A Sink composed without the seam succeeds and does nothing. The connector's
// alternative is failing a whole mail pull over a verb this deployment never
// wired, which is why this is not an error.
func TestARemovalWithoutTheSeamSucceedsAndPurgesNothing(t *testing.T) {
	sink := capture.NewSink(nil)

	if err := sink.RemoveMessage(seatContext(ids.NewV7()), aKey()); err != nil {
		t.Fatalf("a Sink with no purger must succeed: %v", err)
	}
}

// The seat comes from the PRINCIPAL, never from the connector — a connector
// naming its own owner would be naming a seat it inferred from an address.
func TestARemovalPurgesUnderTheSeatOnTheContext(t *testing.T) {
	seat := ids.NewV7()
	var gotSeat ids.UUID
	var gotSystem, gotID string
	calls := 0
	sink := capture.NewSink(nil).WithMessagePurger(
		func(_ context.Context, s ids.UUID, sourceSystem, sourceID string) error {
			calls++
			gotSeat, gotSystem, gotID = s, sourceSystem, sourceID
			return nil
		})

	if err := sink.RemoveMessage(seatContext(seat), aKey()); err != nil {
		t.Fatalf("RemoveMessage: %v", err)
	}

	if calls != 1 {
		t.Fatalf("want exactly one purge, got %d", calls)
	}
	if gotSeat != seat {
		t.Errorf("purged under seat %s, want the one on the context (%s)", gotSeat, seat)
	}
	if gotSystem != "gmail" || gotID != "msg-1" {
		t.Errorf("purged %s/%s, want the natural key the connector named", gotSystem, gotID)
	}
}

// No seat on the context is a WIRING fault, not a message to drop: a removal
// acted on under nobody's authority would destroy whichever copy the query
// happened to find.
func TestARemovalWithNoSeatRefusesRatherThanPurging(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"no actor at all", context.Background()},
		{"an actor carrying no seat", seatContext(ids.Nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			purged := false
			sink := capture.NewSink(nil).WithMessagePurger(
				func(context.Context, ids.UUID, string, string) error {
					purged = true
					return nil
				})

			err := sink.RemoveMessage(tc.ctx, aKey())

			if err == nil {
				t.Fatal("a removal under no seat must be refused")
			}
			if purged {
				t.Error("the purge ran anyway — refusing after destroying the copy is not refusing")
			}
		})
	}
}

// A removal needs both halves of the natural key. Either half missing selects
// by the other alone, which is a different message.
func TestARemovalWithoutABothPartNaturalKeyRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  connector.NaturalKey
	}{
		{"no source system", connector.NaturalKey{SourceID: "msg-1"}},
		{"no source id", connector.NaturalKey{SourceSystem: "gmail"}},
		{"neither", connector.NaturalKey{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			purged := false
			sink := capture.NewSink(nil).WithMessagePurger(
				func(context.Context, ids.UUID, string, string) error {
					purged = true
					return nil
				})

			if err := sink.RemoveMessage(seatContext(ids.NewV7()), tc.key); err == nil {
				t.Fatal("an incomplete natural key must be refused")
			}
			if purged {
				t.Error("the purge ran on an incomplete key")
			}
		})
	}
}

// The purger's failure is the caller's: a deletion that could not be acted on
// must not report success, or the captured copy outlives the owner's deletion
// with nothing left to notice it by.
func TestAFailedPurgeReachesTheConnector(t *testing.T) {
	wanted := errors.New("privacy: the copy is held by a legal duty")
	sink := capture.NewSink(nil).WithMessagePurger(
		func(context.Context, ids.UUID, string, string) error { return wanted })

	err := sink.RemoveMessage(seatContext(ids.NewV7()), aKey())

	if !errors.Is(err, wanted) {
		t.Fatalf("want the purger's own error, got %v", err)
	}
}

// WithMessagePurger returns a COPY. A fixture that wires a purger must not
// arm every other Sink built from the same base — the seam is opt-in per Sink,
// and sharing one would act on deletions a caller never asked it to.
func TestWithMessagePurgerLeavesTheOriginalUnarmed(t *testing.T) {
	base := capture.NewSink(nil)
	armed := base.WithMessagePurger(
		func(context.Context, ids.UUID, string, string) error {
			t.Error("the base Sink's removal reached the copy's purger")
			return nil
		})

	if err := base.RemoveMessage(seatContext(ids.NewV7()), aKey()); err != nil {
		t.Fatalf("the unarmed base must still succeed: %v", err)
	}
	if armed == base {
		t.Error("WithMessagePurger returned the same Sink rather than a copy")
	}
}
