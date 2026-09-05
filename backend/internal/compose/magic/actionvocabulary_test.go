// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// What a machine did that a reader should see.
//
// The cases are about the closed list holding its shape: admission costs a
// sentence and a stated undo policy, and an action nobody has decided about is
// refused rather than shown with a blank line.

import "testing"

// THE rule the surface exists to keep. actor_type says a machine wrote the row;
// it does not say the write means anything to a customer. A maintenance sweep is
// a machine write, and showing it would turn internal churn into apparent value.
func TestAnActionNobodyDecidedAboutIsNotMagic(t *testing.T) {
	for _, action := range []string{"export", "anonymize", "reset_data", "connect"} {
		if _, ok := meaningOf(action); ok {
			t.Errorf("%q is admitted as Magic — it is machine housekeeping, and a "+
				"receipt showing it congratulates the product for keeping its own books", action)
		}
	}
}

// Every admitted action carries the three things admission costs. An action with
// a blank sentence would reach the reader as an empty line.
func TestEveryAdmittedActionSaysWhatItDidAndWhetherItCanBeTakenBack(t *testing.T) {
	if len(admitted) == 0 {
		t.Fatal("no action is admitted at all: the vocabulary is empty and this " +
			"gate is judging nothing")
	}
	for action, meaning := range admitted {
		if meaning.sentence == "" {
			t.Errorf("%q is admitted with no sentence key — it would draw as a blank line", action)
		}
	}
}

// A sent message cannot be unsent, and a booked meeting is on somebody else's
// calendar. Saying so is the point: a control that looked reversible here would
// promise something the world does not allow.
func TestWhatCannotBeTakenBackSaysSo(t *testing.T) {
	for _, action := range []string{"send_email", "schedule"} {
		meaning, ok := meaningOf(action)
		if !ok {
			t.Fatalf("%q is not admitted, so this case is testing nothing", action)
		}
		if meaning.reversible {
			t.Errorf("%q is offered as reversible — the world does not allow it back", action)
		}
	}
}

// Human actors never appear. This surface reports what ran WITHOUT being asked,
// and handing a person their own change back as machinery is a lie about who
// did it.
func TestAHumansOwnChangeIsNeverReportedAsMachinery(t *testing.T) {
	for _, actor := range machineActors() {
		if actor == "human" {
			t.Fatal("the human actor type is reported as machinery")
		}
	}
	if len(machineActors()) != 3 {
		t.Errorf("machine actors = %v, want agent, system and connector", machineActors())
	}
}
