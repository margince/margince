// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What a caller may NAME and what an unguided sweep VISITS are two questions.
// They were one list once, and the cost of that was silent: a type kept out of
// the sweep on purpose stopped being reachable by name at all, so a tool could
// advertise a record type its own search verb refused. These tests hold the
// two apart.

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The case the conflation broke: partner is not in the sweep, and naming it
// still works.
func TestNamingATypeTheSweepSkipsIsAdmitted(t *testing.T) {
	if slices.Contains(searchable, datasource.EntityPartner) {
		t.Fatal("partner is in searchable — this test guards the type that is NOT swept; pick another or delete it")
	}
	walk, err := sweepOrder([]datasource.EntityType{datasource.EntityPartner})
	if err != nil {
		t.Fatalf("naming partner: %v", err)
	}
	if len(walk) != 1 || walk[0] != datasource.EntityPartner {
		t.Fatalf("walk = %v, want exactly [partner]", walk)
	}
}

// Excluding it from the sweep is the other half, and it must still hold: an
// untyped query must not visit partner, or one company comes back twice.
func TestAnUntypedSweepDoesNotVisitPartner(t *testing.T) {
	walk, err := sweepOrder(nil)
	if err != nil {
		t.Fatalf("untyped sweep: %v", err)
	}
	if slices.Contains(walk, datasource.EntityPartner) {
		t.Fatal("an untyped sweep visits partner; every word it would match lives on the organization, so the same company answers twice")
	}
	if !slices.Equal(walk, searchable) {
		t.Fatalf("untyped walk = %v, want searchable %v", walk, searchable)
	}
}

// nameable must cover the sweep. If a swept type were ever left out of it, an
// untyped query would walk a type the caller could not ask for by name.
func TestEverySweptTypeCanAlsoBeNamed(t *testing.T) {
	for _, et := range searchable {
		if !slices.Contains(nameable, et) {
			t.Errorf("%s is swept but not nameable — a caller cannot ask for a type the sweep already visits", et)
		}
	}
}

// Every nameable type must actually be served, or the tool enum advertises a
// refusal. This is the assertion that would have caught the bug this file
// exists for, one level up from sweepOrder.
func TestEveryNameableTypeIsServedBySomeModule(t *testing.T) {
	for _, et := range nameable {
		walk, err := sweepOrder([]datasource.EntityType{et})
		if err != nil {
			t.Errorf("nameable type %s is refused by sweepOrder: %v", et, err)
			continue
		}
		if len(walk) != 1 || walk[0] != et {
			t.Errorf("naming %s resolved to %v, want exactly that one type", et, walk)
		}
	}
}

// A type outside both lists is still refused — widening nameable must not turn
// the allowlist into a pass-through.
func TestATypeOutsideTheVocabularyIsStillRefused(t *testing.T) {
	if _, err := sweepOrder([]datasource.EntityType{datasource.EntityActivity}); err == nil {
		t.Fatal("activity was admitted; a type no search verb serves must be refused, not walked")
	}
	if _, err := sweepOrder([]datasource.EntityType{datasource.EntityType("nonsense")}); err == nil {
		t.Fatal("an unknown type was admitted")
	}
}

// A cursor minted in the partner stream must survive being presented back. It
// is judged against nameable for the same reason the allowlist is: judging it
// against the sweep's default set refuses this seam's own token as malformed
// on the second page.
func TestACursorInANamedOnlyStreamIsNotMalformed(t *testing.T) {
	token, err := storekit.EncodeSweepCursor(storekit.SweepCursor{
		Stream: string(datasource.EntityPartner), Inner: "opaque-keyset",
	})
	if err != nil {
		t.Fatalf("minting a partner cursor: %v", err)
	}
	position, err := resumeStream(token)
	if err != nil {
		t.Fatalf("a partner cursor was refused: %v", err)
	}
	if position.Stream != string(datasource.EntityPartner) {
		t.Fatalf("resumed stream = %q, want partner", position.Stream)
	}
	if position.Inner != "opaque-keyset" {
		t.Fatalf("resumed keyset = %q, want the one that was minted", position.Inner)
	}
}

// The refusal still works: a stream this seam never serves is malformed, so
// widening the check to nameable did not turn it into a pass-through.
func TestACursorInAnUnservedStreamIsStillMalformed(t *testing.T) {
	token, err := storekit.EncodeSweepCursor(storekit.SweepCursor{Stream: "nonsense", Inner: "x"})
	if err != nil {
		t.Fatalf("minting a nonsense-stream cursor: %v", err)
	}
	if _, err := resumeStream(token); err == nil {
		t.Fatal("a cursor naming an unserved stream was accepted")
	}
}

// A cursor minted in a named-only stream must ADVANCE a mixed walk, not restart
// it. resumeIndex ranks streams by their position in the walk order, and a
// stream missing from the list it ranks against answers -1 — which compares
// below every position and sends the walk back to its first type, serving
// records the caller already holds. That is the shape this asserts against.
func TestResumingAMixedWalkAtAPartnerCursorAdvancesPastPerson(t *testing.T) {
	walk, err := sweepOrder([]datasource.EntityType{datasource.EntityPerson, datasource.EntityPartner})
	if err != nil {
		t.Fatalf("naming person and partner: %v", err)
	}
	if len(walk) != 2 || walk[1] != datasource.EntityPartner {
		t.Fatalf("walk = %v, want person then partner", walk)
	}
	at := resumeIndex(walk, string(datasource.EntityPartner))
	if at != 1 {
		t.Fatalf("resumed at index %d, want 1 — index 0 re-serves every person the caller already read", at)
	}
}

// The ordinary case still holds: a cursor in the FIRST stream re-enters it,
// carrying its own keyset rather than skipping the type.
func TestResumingAtTheFirstStreamReentersIt(t *testing.T) {
	walk, err := sweepOrder([]datasource.EntityType{datasource.EntityPerson, datasource.EntityPartner})
	if err != nil {
		t.Fatalf("naming person and partner: %v", err)
	}
	if at := resumeIndex(walk, string(datasource.EntityPerson)); at != 0 {
		t.Fatalf("resumed at index %d, want 0 — the person stream has its own keyset to continue", at)
	}
}
