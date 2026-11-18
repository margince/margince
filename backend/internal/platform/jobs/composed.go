// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// The extension tier's half of the declaration table.
//
// specs_gen.go is CLOSED: it is compiled from backend/api/jobs.yaml, it is
// committed, and it is drift-gated, so it must be byte-identical on every
// installation. An extension's kinds cannot live there — a composed
// installation would then have a different generated file from the vanilla one
// and the gate would fail on every build that enabled a unit.
//
// So the composed kinds arrive at RUN time, through this seam, from the
// generated composition module (which re-emits them as literals). Everything
// the running system asks the declaration — SpecFor, Declared, MustBeTotal —
// answers over both tables, so a composed kind is as declared as a core one and
// River's silent one-minute default is no more reachable for one than for the
// other.
//
// The seam is deliberately NARROW in the one direction that matters: it accepts
// nothing outside the ext_ namespace and it refuses to shadow a core kind. A
// registration surface that could redefine `send_email` would be a way to
// change a core job's wall clock from a directory an installation dropped in.

import (
	"fmt"
	"maps"
	"strings"

	"github.com/margince/margince/backend/pkg/extension"
)

// Composed is one composition's extension job table: the kinds its units
// declare and the failure vocabulary each kind carries. It answers over the
// compiled core table as well, so a composed kind is as declared as a core one.
//
// The zero value is the core table alone, which is what a composition with no
// units holds. A Composed is settled when NewComposed returns and never
// changes, so readers need no lock.
type Composed struct {
	specs map[string]Spec
	// classes is the failure vocabulary keyed by kind and then by sentence.
	classes map[string]map[string]extension.FailureClass
	// declared holds each kind's classes in declaration order, which the
	// sentence-keyed table cannot.
	declared map[string][]extension.FailureClass
}

// NewComposed builds the table of one composition's extension job kinds and
// their failure vocabulary. The composition root calls it once at boot, before
// any runner is built.
//
// Validate-then-apply, like every other reconciliation in this tier: a set with
// one bad entry builds nothing. A refused boot cannot leave half an extension's
// kinds declared while the other half falls through to River's default.
func NewComposed(declarations []Spec, classes map[string][]extension.FailureClass) (Composed, error) {
	table, err := composedSpecs(declarations)
	if err != nil {
		return Composed{}, err
	}
	bySentence, ordered, err := composedFailureClasses(classes)
	if err != nil {
		return Composed{}, err
	}
	return Composed{specs: table, classes: bySentence, declared: ordered}, nil
}

// composedSpecs is the validated kind table. The parameter is not named
// `specs`, the compiled core table's name. Shadowing it would turn the
// core-collision check into a check against the argument itself.
func composedSpecs(declarations []Spec) (map[string]Spec, error) {
	table := make(map[string]Spec, len(declarations))
	for _, s := range declarations {
		if !strings.HasPrefix(s.Kind, extension.NamespacePrefix) {
			return nil, fmt.Errorf("jobs: %q is not an extension kind — this seam declares the %s namespace and nothing else", s.Kind, extension.NamespacePrefix)
		}
		if _, core := specs[s.Kind]; core {
			return nil, fmt.Errorf("jobs: %q is already declared by api/jobs.yaml — a composed kind adds a declaration, it never redefines one", s.Kind)
		}
		if _, dup := table[s.Kind]; dup {
			return nil, fmt.Errorf("jobs: %q is declared twice in the composed set", s.Kind)
		}
		// Cloned in, because a Spec copies by value but its two slices do not:
		// the table must not share Args or Registration.When with the caller.
		table[s.Kind] = s.clone()
	}
	return table, nil
}

// allSpecs is the declaration table every consumer reads: the compiled core
// kinds plus this composition's extension kinds. Core wins on a collision.
// NewComposed refuses one, and the order still gives the merge a defined answer.
func (c Composed) allSpecs() map[string]Spec {
	table := make(map[string]Spec, len(specs)+len(c.specs))
	maps.Copy(table, c.specs)
	maps.Copy(table, specs)
	return table
}

// IsExtensionKind reports whether a kind string sits in the extension
// namespace, WITHOUT asking whether this process declares it.
//
// That distinction is the whole point of the function. A vanilla-built process
// scraping a composed database sees ext_ rows it has no declaration for on
// every rolling deploy and every rollback, and a metric that called those
// "a kind nobody declares" would page somebody for a build skew that is
// expected and temporary. See compose/jobdeclared.go.
func IsExtensionKind(kind string) bool {
	return strings.HasPrefix(kind, extension.NamespacePrefix)
}
