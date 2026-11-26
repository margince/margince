// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"slices"
	"testing"
)

// A declared type that is misspelled, or rides a stream the group does not
// read, never reaches the handler. The group then goes quiet with no error.
func TestEveryDeclaredGroupTypeIsACatalogTypeOnTheGroupsStreams(t *testing.T) {
	declared := 0
	for _, g := range Groups() {
		if !g.Types.Declared() {
			continue
		}
		declared++
		types := g.Types.List()
		if len(types) == 0 {
			t.Errorf("%s declares an empty type list, so it acks every entry unread", g.Name)
		}
		for _, eventType := range types {
			if _, ok := catalog[eventType]; !ok {
				t.Errorf("%s declares %q, which is not a catalog type", g.Name, eventType)
				continue
			}
			stream, err := StreamFor(eventType)
			if err != nil {
				t.Fatalf("routing %s: %v", eventType, err)
			}
			if !slices.Contains(g.Streams, stream) {
				t.Errorf("%s declares %s, which rides %s; the group reads %v", g.Name, eventType, stream, g.Streams)
			}
		}
	}
	if declared == 0 {
		t.Fatal("no group declares its types, so this test checks nothing")
	}
}

func TestTheZeroTypeSetHoldsEveryType(t *testing.T) {
	var all TypeSet
	if all.Declared() || !all.Has("contact.created") || all.List() != nil {
		t.Fatal("the zero TypeSet must take every type and declare none")
	}
	one := NewTypeSet("contact.created")
	if !one.Declared() || !one.Has("contact.created") || one.Has("contact.updated") {
		t.Fatal("a declared TypeSet must hold exactly its types")
	}
}

func TestTheContextGraphTypesAreTheContentChangingTypesOnItsStreams(t *testing.T) {
	for _, eventType := range []string{"contact.created", "deal.updated", "activity.captured", "contact.merged"} {
		if !ContextGraphTypes.Has(eventType) {
			t.Errorf("cg:context-graph must embed after %s", eventType)
		}
	}
	for _, eventType := range []string{"contact.archived", "consent.changed", "approval.requested"} {
		if ContextGraphTypes.Has(eventType) {
			t.Errorf("cg:context-graph must not take %s", eventType)
		}
	}
}
