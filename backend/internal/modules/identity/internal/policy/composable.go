// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package policy

// The RBAC object vocabulary, made composable.
//
// coreObjects (policy.go) is CLOSED and stays closed: it is the set the
// contract's RbacObject enum mirrors, the seeded role documents grant, and the
// merge-blocking parity test holds equal. An extension cannot join it —
// coreObjects is compiled in, and an installation's units are not.
//
// What an extension can do is register its own object into the vocabulary its
// composition reads. That is what makes an extension screen possible at all.
// Without it, Parse drops a grant on `ext_<unit>_<object>`. The object then
// never reaches the /me snapshot the web client gates its affordances on. Such
// a screen renders nothing, and looks like a frontend bug rather than a missing
// vocabulary entry.
//
// The vocabulary is a value the composition builds once in RegisterExtensions
// and hands to every reader, so two compositions in one process never share it.

import (
	"fmt"
	"regexp"
	"slices"
)

// Object is an RBAC object name an extension contributes to the vocabulary.
// It is a distinct type from the plain strings coreObjects holds so that a
// caller cannot pass an arbitrary string into the vocabulary without meeting
// Validate first.
type Object string

// extensionObjectGrammar: `ext_<unit>_<object>`, with the unit and the object
// as lower-case [a-z0-9] runs joined by single underscores. It is deliberately
// NOT the same grammar as a core object's (which has no prefix): the whole
// point of the namespace token is that a core object can never be mistaken for
// an extension's, in either direction, at a glance or by a parser.
var extensionObjectGrammar = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)+$`)

// Validate enforces the namespace and the grammar. An object outside it is
// refused rather than accepted-and-ignored, because the vocabulary is what
// Parse admits a stored document by: an unnamespaced entry would let a unit
// widen what a core role document may grant, from a directory an operator
// dropped in.
func (o Object) Validate() error {
	if !extensionObjectGrammar.MatchString(string(o)) {
		return fmt.Errorf("RBAC object %q is not a valid extension object (ext_<unit>_<object>, lower-case [a-z0-9] joined by single underscores)", string(o))
	}
	// The core set is closed, and this is the one place a name could be added
	// that collides with it. A shadowing entry would not widen anything today
	// (the grammar's ext_ prefix already precludes it), but the check is what
	// keeps that a FACT rather than a consequence of two regexes agreeing.
	if slices.Contains(coreObjects, string(o)) {
		return fmt.Errorf("RBAC object %q is already a core object — an extension may not redefine one", string(o))
	}
	return nil
}

// Vocabulary is the extension objects one composition registers. The zero
// value registers none, so a reader handed nothing grants only core objects.
type Vocabulary struct {
	objects map[Object]bool
}

// NewVocabulary builds a composition's vocabulary. A set with one bad name
// builds nothing, so a boot that reports failure has not widened what a role
// document may grant.
//
// Naming the same object twice is an error, not a no-op. Two units both
// claiming one object name would be a wiring defect where each thinks it owns
// the grants. The ext_<unit>_ prefix means it can only happen if one unit
// declares the same name twice, which is a mistake worth naming.
func NewVocabulary(objects ...Object) (Vocabulary, error) {
	v := Vocabulary{objects: make(map[Object]bool, len(objects))}
	for _, o := range objects {
		if err := o.Validate(); err != nil {
			return Vocabulary{}, err
		}
		if v.objects[o] {
			return Vocabulary{}, fmt.Errorf("RBAC object %q is already registered", string(o))
		}
		v.objects[o] = true
	}
	return v, nil
}

// IsRegistered reports whether an extension registered the object.
func (v Vocabulary) IsRegistered(object string) bool {
	return v.objects[Object(object)]
}

// Registered returns the registered set, sorted. It is what the merge seeds
// the /me snapshot from, and sorted so a snapshot does not depend on map
// iteration order.
func (v Vocabulary) Registered() []string {
	out := make([]string, 0, len(v.objects))
	for o := range v.objects {
		out = append(out, string(o))
	}
	slices.Sort(out)
	return out
}

// IsGrantable reports whether an object name is one a role document may
// grant: a core object, or one an extension of this composition registered.
// This is the question Parse asks, and the question an authority gate asks
// before trusting a requirement is satisfiable.
//
// IsCoreObject stays what its name says — the closed compiled-in set — because
// the parity test against the contract's enum reads exactly that, and widening
// it would make the gate pass for an object no client can express.
func (v Vocabulary) IsGrantable(object string) bool {
	return IsCoreObject(object) || v.IsRegistered(object)
}
