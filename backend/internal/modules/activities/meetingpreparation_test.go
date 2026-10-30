// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var prepStart = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)

func prepFacts(start time.Time, agenda string, guests ...string) PreparationFacts {
	return preparationFactsOf(start, "instant", agenda, guests)
}

func TestTheRevisionIgnoresParticipantOrder(t *testing.T) {
	a := prepFacts(prepStart, "invite-1", "a:buyer@customer.test", "u:rep")
	b := prepFacts(prepStart, "invite-1", "u:rep", "a:buyer@customer.test", "u:rep")
	if a.Revision() != b.Revision() {
		t.Error("the same meeting read in another order computed another revision")
	}
}

func TestEachPreparationFactChangesTheRevisionAndIsNamed(t *testing.T) {
	confirmed := prepFacts(prepStart, "invite-1", "u:rep")
	for _, c := range []struct {
		fact    PreparationFact
		current PreparationFacts
	}{
		{FactStartTime, prepFacts(prepStart.Add(time.Hour), "invite-1", "u:rep")},
		{FactStartTime, preparationFactsOf(prepStart, "day", "invite-1", []string{"u:rep"})},
		{FactAgenda, prepFacts(prepStart, "invite-2", "u:rep")},
		{FactParticipants, prepFacts(prepStart, "invite-1", "u:rep", "a:cfo@customer.test")},
	} {
		if c.current.Revision() == confirmed.Revision() {
			t.Errorf("a change to the %s kept the revision", c.fact)
		}
		if got := c.current.changedSince(confirmed); !slices.Equal(got, []PreparationFact{c.fact}) {
			t.Errorf("a change to the %s was named as %v", c.fact, got)
		}
	}
}

func TestReadyHoldsOnlyAtTheConfirmedRevision(t *testing.T) {
	id := ids.NewV7()
	confirmed := prepFacts(prepStart, "invite-1", "u:rep")
	stored := &storedReadiness{revision: confirmed.Revision(), facts: confirmed}

	if got := preparationAt(id, confirmed, nil); got.Ready || len(got.Changed) != 0 {
		t.Errorf("never confirmed reads %+v, want needs review with nothing changed", got)
	}
	if got := preparationAt(id, confirmed, stored); !got.Ready || len(got.Changed) != 0 {
		t.Errorf("confirmed at this revision reads %+v, want ready", got)
	}
	moved := prepFacts(prepStart.Add(time.Hour), "invite-1", "u:rep")
	if got := preparationAt(id, moved, stored); got.Ready || !slices.Equal(got.Changed, []PreparationFact{FactStartTime}) {
		t.Errorf("confirmed before a move reads %+v, want needs review naming the start", got)
	}
}

// A brief written about the meeting, generated again or not, has no way into
// the revision: these four facts are all it is computed from.
func TestTheRevisionReadsOnlyTheMeetingsOwnFacts(t *testing.T) {
	var names []string
	for field := range reflect.TypeFor[PreparationFacts]().Fields() {
		names = append(names, field.Name)
	}
	if want := []string{"StartsAt", "StartsOnDay", "Agenda", "Participants"}; !slices.Equal(names, want) {
		t.Errorf("the preparation facts are %v, want %v; a new fact changes every stored revision", names, want)
	}
}
