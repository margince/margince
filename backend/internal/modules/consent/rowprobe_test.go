// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which row probe a recorded state has to clear.
//
// The split is by STATE, not by subject, and it is the reason the erasure story
// works: a withdrawal stays recordable against an archived — including an
// Art. 17 anonymized — subject, because suppression is what you most want still
// working once somebody has asked to be forgotten. A grant does not, or an
// erased subject would go on accruing consent rows the erasure has just
// destroyed the means to stop.
//
// Held here because getting it backwards is invisible: both probes return nil
// for the unbounded actor most tests bind, so a swap would pass every suite
// that does not seat a bounded one.

import (
	"errors"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAWithdrawalTakesThePermissiveProbeAndAGrantTheLiveOne(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		state ConsentState
		want  any
		why   string
	}{
		{
			StateWithdrawn, auth.EnsureWritable,
			"a withdrawal must stay recordable against an archived subject: suppression is what " +
				"you most want still working once somebody has asked to be forgotten",
		},
		{
			StateGranted, auth.EnsureWritableLive,
			"a grant against an archived subject accrues consent, event, audit and outbox rows " +
				"for somebody whose means to stop it the erasure has already destroyed",
		},
	} {
		probe, err := rowProbeFor(c.state)
		if err != nil {
			t.Fatalf("%s: %v", c.state, err)
		}
		if reflect.ValueOf(probe).Pointer() != reflect.ValueOf(c.want).Pointer() {
			t.Errorf("%s took the other probe — %s", c.state, c.why)
		}
	}
}

// A state nobody has decided about REFUSES rather than guessing.
//
// ParseRecordableState admits the value, so arriving here means the vocabulary
// grew and this decision was not made. Defaulting either way would answer a
// question nobody asked: to the permissive probe and an erased subject takes
// the new state; to the live one and a suppression stops working exactly when
// it matters.
func TestAStateWithNoProbeDecisionIsRefused(t *testing.T) {
	t.Parallel()

	probe, err := rowProbeFor(ConsentState("pondered"))
	if err == nil {
		t.Fatal("an undecided state was given a probe: the vocabulary grew and the choice between " +
			"a lawful-basis claim and a suppression was made by a default rather than by a person")
	}
	if probe != nil {
		t.Error("the refusal still handed back a probe")
	}
	// Not one of the request-shaped sentinels: this is a defect in the code
	// rather than in anything a caller sent, and reporting it as a 4xx would
	// tell an operator to fix their input.
	if errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Error("the refusal reads as a bad request; it is a defect in the code, not in the input")
	}
}
