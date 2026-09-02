// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// What the audience gate does NOT cover, and why that is safe today.
//
// The gate reads one thing: the trigger's subject, when it is an activity. Two
// nearby shapes are outside it, and both are safe only because nothing in the
// tree currently produces them. A comment saying so would go stale silently —
// the whole point of the rule that a claim about the tree needs a test — so
// these assert the conditions rather than describing them.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestNoCatalogTriggerCarriesASignalSubject(t *testing.T) {
	// A signal's evidence is activity-derived and carries its own visibility
	// (platform/auth's SignalScopeClause), which the audience gate does not
	// consult. A trigger that fired on one would fan a signal's evidence into a
	// task with no check at all.
	//
	// Asserted over the closed trigger registry rather than over the handlers,
	// because the registry is what decides which event types a human-authored
	// automation can ever be configured for.
	for _, kind := range AllTriggerKinds() {
		def, ok := triggerDefs[kind]
		if !ok {
			t.Fatalf("trigger %q is in AllTriggerKinds but not in triggerDefs, so this "+
				"scan reads a smaller registry than exists", kind)
		}
		if def.EventType == "" {
			continue
		}
		if entityTypeOfEvent(def.EventType) == "signal" {
			t.Errorf("trigger %q fires on %q, whose subject is a signal. The audience gate "+
				"reads activity subjects only, so a signal's activity-derived evidence would "+
				"reach a firing unchecked — extend the gate before shipping this trigger",
				kind, def.EventType)
		}
	}
}

// entityTypeOfEvent maps a catalog event type to the entity its envelope names.
//
// A small table rather than a contract lookup: the generated payload types live
// in internal/contracts, which this module does not import, and the four event
// types the trigger registry pins are the whole corpus. A trigger added with a
// new event type fails the assertion below rather than passing unclassified.
func entityTypeOfEvent(eventType string) string {
	switch eventType {
	case eventDealStageChanged:
		return "deal"
	case eventEngagementReply:
		return "activity"
	default:
		return "unclassified"
	}
}

func TestEveryPinnedTriggerEventIsClassified(t *testing.T) {
	// The census's own guard. TestNoCatalogTriggerCarriesASignalSubject reports
	// PASS for an event type it cannot classify, so an unclassified one has to
	// fail here instead — under-recognition is the one way this must not break.
	classified := 0
	for _, kind := range AllTriggerKinds() {
		def := triggerDefs[kind]
		if def.EventType == "" {
			continue
		}
		classified++
		if entityTypeOfEvent(def.EventType) == "unclassified" {
			t.Errorf("trigger %q pins event %q, which entityTypeOfEvent does not know. "+
				"Classify it: an unknown subject reads as 'not a signal' and passes the "+
				"scan next door without anybody deciding it should", kind, def.EventType)
		}
	}
	if classified == 0 {
		t.Fatal("no trigger pins an event type, so both scans in this file read nothing")
	}
}

func TestTheGateReadsTheSubjectTheEngineActuallyCarries(t *testing.T) {
	// activityEntity is compared against workflow.Event.Entity.Type, which is a
	// datasource.EntityType. If the two vocabularies ever diverge — a rename on
	// either side — the gate silently matches nothing and every firing passes.
	// A string constant cannot fail to compile into that shape, so it is
	// asserted.
	if activityEntity != datasource.EntityType("activity") {
		t.Errorf("activityEntity = %q, want %q: the gate compares this against the event's "+
			"own entity type, and a mismatch admits every firing while looking like a gate",
			activityEntity, "activity")
	}
}
