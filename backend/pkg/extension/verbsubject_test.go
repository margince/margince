// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"encoding/json"
	"strings"
	"testing"
)

// confirmFirst is wellFormed asking for a human: the tier that stages, and the
// subject that gives its approval something to be about.
func confirmFirst() Verb {
	v := wellFormed()
	v.Tier = TierConfirmationRequired
	v.InputSchema = json.RawMessage(`{"type":"object","required":["widget_id"],
		"properties":{"widget_id":{"type":"string","format":"uuid"}}}`)
	v.Subject = Subject{Arg: "widget_id", Table: "ext_crm_demo_widget"}
	return v
}

func TestAConfirmFirstOperationDeclaresWhatItStagesAgainst(t *testing.T) {
	if err := confirmFirst().Validate(); err != nil {
		t.Fatalf("a confirm-first declaration naming its subject must validate: %v", err)
	}

	for name, mutate := range map[string]func(*Verb){
		// Half a subject is not a subject. Each half answers a different
		// question — which argument carries the id, and where the row lives —
		// so neither substitutes for the other.
		"a subject naming no argument": func(v *Verb) { v.Subject.Arg = "" },
		"a subject naming no table":    func(v *Verb) { v.Subject.Table = "" },
		// The id would be absent on every call, which is the dead capability
		// again — discovered one refusal at a time instead of at generation.
		"an argument the schema does not declare": func(v *Verb) { v.Subject.Arg = "gadget_id" },
		// Staging reads the subject as a uuid string on every call, so an
		// optional one stages on some invocations and is refused on others,
		// and any other type is a call that can never stage at all.
		"a subject the schema does not require": func(v *Verb) {
			v.InputSchema = json.RawMessage(`{"type":"object",
				"properties":{"widget_id":{"type":"string","format":"uuid"}}}`)
		},
		"a subject typed as something other than a string": func(v *Verb) {
			v.InputSchema = json.RawMessage(`{"type":"object","required":["widget_id"],
				"properties":{"widget_id":{"type":"integer"}}}`)
		},
		"an operation that takes no arguments": func(v *Verb) { v.InputSchema = nil },
		// A unit may put its OWN rows in front of a human and no others.
		"a table in another unit's namespace": func(v *Verb) { v.Subject.Table = "ext_other_widget" },
		"a table that is not an identifier":   func(v *Verb) { v.Subject.Table = "ext_crm_demo_widget; DROP" },
		"an argument that is not a name":      func(v *Verb) { v.Subject.Arg = "Widget-Id" },
		// Deciding a staged call requires the grant performing it requires, so
		// an operation with no object is one nobody can be required to hold.
		"no RBAC object to require of the decider": func(v *Verb) { v.RbacObject, v.RbacAction = "", "" },
	} {
		v := confirmFirst()
		mutate(&v)
		if err := v.Validate(); err == nil {
			t.Errorf("%s: validated, and it must not", name)
		}
	}
}

// A confirm-first operation with NO subject is not this grammar's refusal: a
// handler-less one publishes a 501 route and stages nothing, and only the
// composed set knows which verbs have behavior. The serving adapter demands it
// (compose/extensiontools.go); here it is simply the ordinary manifest shape.
func TestAConfirmFirstDeclarationWithNoSubjectIsLeftToTheServingAdapter(t *testing.T) {
	v := confirmFirst()
	v.Subject = Subject{}
	if err := v.Validate(); err != nil {
		t.Fatalf("a contract-only confirm-first declaration must validate: %v", err)
	}
}

// A subject is meaningful only where something stages. Declared on a 🟢 tool it
// is a fact nothing reads, which is worth refusing rather than ignoring: an
// author who wrote one believes their operation asks for a human.
func TestASubjectOnATierThatNeverStagesIsRefused(t *testing.T) {
	v := wellFormed()
	v.Subject = Subject{Arg: "widget_id", Table: "ext_crm_demo_widget"}
	err := v.Validate()
	if err == nil {
		t.Fatal("an auto-execute operation declaring a staging subject validated")
	}
	if !strings.Contains(err.Error(), "only a") {
		t.Errorf("the refusal must say why nothing would read it, got: %v", err)
	}
}

// The zero value is what every other tier carries, so it must not be mistaken
// for a half-declared one.
func TestNoSubjectIsTheOrdinaryCase(t *testing.T) {
	if !(Subject{}).IsZero() {
		t.Error("the zero subject must report itself as absent")
	}
	if (Subject{Arg: "widget_id"}).IsZero() {
		t.Error("a half-declared subject must NOT report itself as absent — it is a fault, not an omission")
	}
}
