// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// What a confirm-first extension operation puts in front of a human.
//
// A 🟡 tool's refused call is parked as an approval, and an approval is a
// judgment about a THING: the inbox shows the row, the decision authority is
// derived from it, and the person answering has to be someone who may see it.
// Core verbs answer that from the record they name — a lead, a deal, an
// activity. An extension operation names nothing the core knows about, so the
// unit has to say which row its call is about, and where that row lives.
//
// Both halves are needed and neither can be derived. The ARGUMENT is the only
// place a call carries its subject, and which argument that is is a fact about
// the operation's own schema. The TABLE is what makes the staged row
// answerable at all: the inbox refuses to show a staging whose target it
// cannot prove still exists, so something has to be able to ask.
//
// Declared only for confirm-first operations. A 🟢 tool stages nothing, so a
// subject on one would be a fact nothing reads. Whether a 🟡 operation MUST
// carry one is not this grammar's question: a handler-less declaration is a
// manifest request that stages nothing either, and only the composed set knows
// which verbs have behavior — so the serving adapter demands it, and this
// checks that whatever was declared is coherent.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Subject is the row a confirm-first operation's approval is staged against.
type Subject struct {
	// Arg is the input property that carries the row's id, as a UUID string.
	// It must be a property the operation's own InputSchema declares — an
	// argument nothing sends is a subject that would be absent on every call.
	Arg string
	// Table is the unit-owned table the row lives in, namespaced like every
	// other identifier a unit brings. It is the staged target's TYPE and the
	// table the inbox's existence probe reads, so it is a table name rather
	// than a label: a name nothing can be selected from would leave the staged
	// row unprovable and therefore invisible.
	Table string
}

// IsZero reports a subject nothing declared.
func (s Subject) IsZero() bool { return s.Arg == "" && s.Table == "" }

// subjectArgGrammar is the JSON property spelling this surface accepts, which
// is the tool-argument spelling every other schema here uses.
var subjectArgGrammar = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// subjectTableGrammar is a SQL identifier, checked because this name is
// interpolated into the probe that reads it. The namespace check below is the
// authority rule; this is the shape rule, and both run before the name reaches
// a statement.
var subjectTableGrammar = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)*$`)

// validateSubject holds a subject to the operation that declares it: present
// exactly when the tier stages, spelled as an identifier, inside the unit's
// namespace, and naming an argument the operation actually takes.
func (v Verb) validateSubject() error {
	if v.Tier != TierConfirmationRequired {
		if !v.Subject.IsZero() {
			return fmt.Errorf("operation %s declares a staging subject but requests tier %q — only a "+
				"confirm-first operation stages, so nothing would ever read it",
				v.OperationID, string(v.Tier))
		}
		return nil
	}
	if v.Subject.IsZero() {
		// NOT refused here. A confirm-first operation with no handler is a
		// manifest request rather than a served capability — it publishes a
		// route that answers 501 and stages nothing — and this grammar cannot
		// see whether a handler exists. The demand belongs where the serving
		// decision is made, so compose/extensiontools.go refuses a SERVED 🟡
		// tool that declares no subject, and a contract-only one keeps working
		// exactly as it did.
		return nil
	}
	if !subjectArgGrammar.MatchString(v.Subject.Arg) {
		return fmt.Errorf("operation %s names staging subject argument %q, which is not an argument name "+
			"(lower snake_case, e.g. note_id)", v.OperationID, v.Subject.Arg)
	}
	if !subjectTableGrammar.MatchString(v.Subject.Table) {
		return fmt.Errorf("operation %s names staging subject table %q, which is not a table identifier",
			v.OperationID, v.Subject.Table)
	}
	if want := NamespacePrefix + strings.ReplaceAll(string(v.Unit), "-", "_") + "_"; !strings.HasPrefix(v.Subject.Table, want) {
		return fmt.Errorf("operation %s stages against table %q, which is outside extension %q's %s namespace — "+
			"a unit may put its own rows in front of a human and no others",
			v.OperationID, v.Subject.Table, v.Unit, want)
	}
	// The grant a human needs to DECIDE this is the one the operation itself
	// gates on, so a staged call with no object would be one nobody can be
	// required to hold — decidable by any seat that can see the inbox. The
	// mutating-scope rule below already demands an object for write and draft;
	// this demands one for every confirm-first operation, including a read that
	// asks for a human anyway.
	if v.RbacObject == "" {
		return fmt.Errorf("operation %s requests %q but declares no RBAC object — deciding a staged call "+
			"requires the grant performing it requires, and there would be nothing to require",
			v.OperationID, string(TierConfirmationRequired))
	}
	return v.subjectArgIsDeclared()
}

// subjectArgIsDeclared refuses a subject naming an argument the operation does
// not take. Without it a unit could stage against `note_id` while its schema
// declares `id`, and every call would fail at the staging read — the dead
// capability in a new shape, discovered one refusal at a time instead of at
// generation.
func (v Verb) subjectArgIsDeclared() error {
	var doc struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if v.InputSchema == nil {
		return fmt.Errorf("operation %s stages against argument %q but declares no arguments at all",
			v.OperationID, v.Subject.Arg)
	}
	if err := json.Unmarshal(v.InputSchema, &doc); err != nil {
		return fmt.Errorf("operation %s declares an InputSchema this reader cannot walk, so it cannot be "+
			"told whether it takes the staging subject %q", v.OperationID, v.Subject.Arg)
	}
	property, declared := doc.Properties[v.Subject.Arg]
	if !declared {
		return fmt.Errorf("operation %s stages against argument %q, which its own InputSchema does not "+
			"declare — the id would be absent on every call", v.OperationID, v.Subject.Arg)
	}
	// REQUIRED, and a STRING, because staging reads it as one on every call: it
	// unmarshals the property as a JSON string and parses that as a uuid. An
	// optional subject is a call that stages on some invocations and is refused
	// on others; a subject typed as anything else is a call that is refused on
	// all of them. Both are declarations a client can generate a call from and
	// a boot will serve, so they are refused at the declaration rather than
	// discovered one staging at a time.
	if property.Type != "string" {
		return fmt.Errorf("operation %s stages against argument %q, which its InputSchema types as %q — "+
			"a staged subject is read as a uuid string on every call, so any other type is a call that "+
			"can never stage", v.OperationID, v.Subject.Arg, property.Type)
	}
	for _, name := range doc.Required {
		if name == v.Subject.Arg {
			return nil
		}
	}
	return fmt.Errorf("operation %s stages against argument %q but does not require it — a call that omits "+
		"it names no record, so the operation would stage on some invocations and be refused on others",
		v.OperationID, v.Subject.Arg)
}
