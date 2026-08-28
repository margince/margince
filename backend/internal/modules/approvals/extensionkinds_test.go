// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// wellFormedKind is the registration every case below mutates one field of.
func wellFormedKind() ExtensionKind {
	return ExtensionKind{
		Verb:        "notes_forget",
		TargetTable: "ext_notes_note",
		RbacObject:  "ext_notes_note",
		RbacAction:  principal.ActionDelete,
	}
}

// registerOnly installs a set for one test and puts the process back the way it
// found it. The registry is boot state, and a test that left its own kinds
// behind would govern every test after it.
func registerOnly(t *testing.T, kinds ...ExtensionKind) {
	t.Helper()
	if err := RegisterExtensionKinds(kinds); err != nil {
		t.Fatalf("registering the fixture kinds: %v", err)
	}
	t.Cleanup(func() {
		if err := RegisterExtensionKinds(nil); err != nil {
			t.Errorf("clearing the fixture kinds: %v", err)
		}
	})
}

// A registered verb is decidable in all three of the ways a staged kind has to
// be: it derives its decision grants, its target classifies, and its target has
// an existence query. A kind carrying fewer than three is half-governed, which
// is the dead authority object the visibility rules exist to prevent.
func TestARegisteredExtensionKindIsDecidableInEveryWayAStagedKindMustBe(t *testing.T) {
	kind := wellFormedKind()
	registerOnly(t, kind)

	grants, err := decisionGrantsFor(kind.Verb, nil)
	if err != nil {
		t.Fatalf("deriving the decision grants: %v", err)
	}
	if len(grants) != 1 || grants[0].Object != kind.RbacObject || grants[0].Action != kind.RbacAction {
		t.Errorf("grants = %v, want the operation's own object and action — deciding takes the grant "+
			"performing it takes", grants)
	}
	if probe := probeFor(kind.TargetTable); probe != probeExistence {
		t.Errorf("probe = %v, want existence: a unit's store applies no row scope of its own", probe)
	}
	if query := extensionExistenceQuery(kind.TargetTable); !strings.Contains(query, `"ext"."ext_notes_note"`) {
		t.Errorf("the existence probe reads %q — it must name the ext schema explicitly rather than "+
			"resolve through a search path", query)
	}
	types := ClassifiedTargetTypes()
	if !contains(types, kind.TargetTable) {
		t.Errorf("%q is not in the classified vocabulary, so the fan-out's parity gate would never see it",
			kind.TargetTable)
	}
}

// An unregistered verb stays undecidable, which is what makes the registration
// load-bearing rather than decoration.
func TestAnUnregisteredVerbIsStillUndecidable(t *testing.T) {
	registerOnly(t)
	if _, err := decisionGrantsFor("notes_forget", nil); err == nil {
		t.Error("an unregistered verb derived decision grants — a kind nothing registered must not be " +
			"decidable, or a staged row would be releasable by anyone")
	}
	if probe := probeFor("ext_notes_note"); probe != probeNoRule {
		t.Errorf("probe = %v, want probeNoRule: an unregistered target must fail closed", probe)
	}
}

// Every bound the registration refuses, each because the alternative is a kind
// that is governed by something other than what a reader would think.
func TestRegisterExtensionKindsRefusesWhatCannotBeGoverned(t *testing.T) {
	for name, kinds := range map[string][]ExtensionKind{
		// A unit re-governing a verb this module decides.
		"a core kind's name": {{
			Verb: "promote_lead", TargetTable: "ext_notes_note",
			RbacObject: "ext_notes_note", RbacAction: principal.ActionDelete,
		}},
		// Its existence probe would read another store's table.
		"a core target type": {{
			Verb: "notes_forget", TargetTable: tablePerson,
			RbacObject: "ext_notes_note", RbacAction: principal.ActionDelete,
		}},
		// The name is read into a statement, so it is checked as an identifier
		// here as well as at declaration.
		"a table that is not a namespaced identifier": {{
			Verb: "notes_forget", TargetTable: "note",
			RbacObject: "ext_notes_note", RbacAction: principal.ActionDelete,
		}},
		"a table carrying a statement": {{
			Verb: "notes_forget", TargetTable: `ext_notes_note" WHERE true--`,
			RbacObject: "ext_notes_note", RbacAction: principal.ActionDelete,
		}},
		// Nothing to require of the decider is decidable by any seat that can
		// see the inbox.
		"no RBAC object": {{
			Verb: "notes_forget", TargetTable: "ext_notes_note", RbacAction: principal.ActionDelete,
		}},
		"no RBAC action": {{
			Verb: "notes_forget", TargetTable: "ext_notes_note", RbacObject: "ext_notes_note",
		}},
		"no verb": {{
			TargetTable: "ext_notes_note", RbacObject: "ext_notes_note", RbacAction: principal.ActionDelete,
		}},
		"one verb registered twice": {wellFormedKind(), wellFormedKind()},
		// Who may decide a staged row must not depend on which verb parked it.
		"two verbs over one table demanding different grants": {
			wellFormedKind(),
			{
				Verb: "notes_rename", TargetTable: "ext_notes_note",
				RbacObject: "ext_notes_note", RbacAction: principal.ActionUpdate,
			},
		},
	} {
		if err := RegisterExtensionKinds(kinds); err == nil {
			t.Errorf("%s: registered, and it must not", name)
			if clearErr := RegisterExtensionKinds(nil); clearErr != nil {
				t.Fatalf("clearing after an accepted set: %v", clearErr)
			}
		}
	}
}

// Two verbs over ONE table is legitimate — a unit may confirm both an edit and
// a removal of the same row — as long as they agree about who decides.
func TestTwoVerbsMayStageAgainstOneTableWhenTheyAgreeOnTheDecider(t *testing.T) {
	registerOnly(t, wellFormedKind(), ExtensionKind{
		Verb: "notes_rename", TargetTable: "ext_notes_note",
		RbacObject: "ext_notes_note", RbacAction: principal.ActionDelete,
	})
	for _, verb := range []string{"notes_forget", "notes_rename"} {
		if _, err := decisionGrantsFor(verb, nil); err != nil {
			t.Errorf("%s: %v", verb, err)
		}
	}
	if types := ClassifiedTargetTypes(); count(types, "ext_notes_note") != 1 {
		t.Errorf("the shared table appears %d times in the vocabulary, want once",
			count(types, "ext_notes_note"))
	}
}

// A second registration REPLACES the set, so a process that rebuilds its
// composition cannot accumulate kinds no unit serves any more.
func TestRegisteringAgainReplacesTheSet(t *testing.T) {
	registerOnly(t, wellFormedKind())
	if err := RegisterExtensionKinds([]ExtensionKind{{
		Verb: "notes_rename", TargetTable: "ext_notes_other",
		RbacObject: "ext_notes_other", RbacAction: principal.ActionUpdate,
	}}); err != nil {
		t.Fatalf("re-registering: %v", err)
	}
	if _, err := decisionGrantsFor("notes_forget", nil); err == nil {
		t.Error("a verb from the previous set is still decidable — the registration must replace, not add")
	}
}

func contains(values []string, want string) bool { return count(values, want) > 0 }

func count(values []string, want string) int {
	n := 0
	for _, value := range values {
		if value == want {
			n++
		}
	}
	return n
}
