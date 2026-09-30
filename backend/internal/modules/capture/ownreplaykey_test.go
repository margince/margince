// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Which collisions may be resolved by filing the seat's own row. The three
// answers are the whole of the rule, and two of them are refusals — a fallback
// that fired on the wrong record would invent a duplicate of something that
// really is one object.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// seatCaptureCtx is a capture running under one seat's grant.
func seatCaptureCtx(seat ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "connector:gmail:" + seat.String(), UserID: seat,
	})
}

func mailRecord(sourceSystem, sourceID string) connector.NormalizedRecord {
	return connector.NormalizedRecord{
		NaturalKey: connector.NaturalKey{SourceSystem: sourceSystem, SourceID: sourceID},
	}
}

func TestAMailCollisionUnderASeatIsScopedToThatSeat(t *testing.T) {
	seat := ids.NewV7()
	const messageID = "contested@acme.example"

	scoped, ok := scopedToSeat(seatCaptureCtx(seat), mailRecord(connector.EmailSourceSystem, messageID))
	if !ok {
		t.Fatal("a mail collision under a seat must be resolvable by filing that seat's own row")
	}
	if scoped.NaturalKey.SourceID == messageID {
		t.Error("the record kept the shared key; two seats would collide on it again")
	}
	if got := connector.SharedMailKey(scoped.NaturalKey.SourceID); got != messageID {
		t.Errorf("the shared key behind %q is %q, want %q", scoped.NaturalKey.SourceID, got, messageID)
	}
	if scoped.NaturalKey.SourceSystem != connector.EmailSourceSystem {
		t.Errorf("the source system moved to %q; only the key is the seat's", scoped.NaturalKey.SourceSystem)
	}
}

// A provider-issued id is not sender-controlled, so a collision on it means two
// seats reached ONE object. Filing a second row would invent a duplicate of a
// record that really is one.
func TestACollisionOnAProviderIssuedKeyIsNeverScoped(t *testing.T) {
	ctx := seatCaptureCtx(ids.NewV7())
	if _, ok := scopedToSeat(ctx, mailRecord("hubspot", "engagement-4471")); ok {
		t.Error("a HubSpot engagement was scoped to a seat; its id is the provider's, not a typed header")
	}
	if _, ok := scopedToSeat(ctx, mailRecord(connector.EmailSourceSystem, "")); ok {
		t.Error("a record with no natural key was scoped; there is no key to scope")
	}
}

// A capture with no seat behind it claims nothing, so it can never reach an
// unprovable collision — and a key scoped to the nil seat would be a second
// shared key rather than anybody's own.
func TestACaptureWithNoSeatIsNeverScoped(t *testing.T) {
	rec := mailRecord(connector.EmailSourceSystem, "contested@acme.example")
	if _, ok := scopedToSeat(context.Background(), rec); ok {
		t.Error("a capture with no principal was scoped to a seat that does not exist")
	}
	if _, ok := scopedToSeat(seatCaptureCtx(ids.Nil), rec); ok {
		t.Error("a capture whose principal carries no user was scoped to the nil seat")
	}
}

// And the refusal is enforced where it matters, not only where it is decided: a
// collision this sink may not resolve by filing the seat's own row falls back to
// the skip it always was, before the transaction is touched at all.
//
// A nil transaction is the assertion. If the mail-only line were ever dropped
// from here, this would panic on the first write rather than passing quietly.
func TestAProviderIssuedKeyFallsBackToTheSkipWithoutWriting(t *testing.T) {
	sink := &Sink{}
	rec := mailRecord("hubspot", "engagement-4471")

	_, created, _, err := sink.fileUnderOwnReplayKey(
		seatCaptureCtx(ids.NewV7()), nil, rec, ActivityFields{}, birthDecision{}, false)

	if created {
		t.Error("a provider-issued collision created a row; its id is the provider's, not a typed header")
	}
	if !errors.Is(err, connector.ErrSkip) {
		t.Errorf("answered %v, want the skip a collision on a provider-issued key has always taken", err)
	}
}
