// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import "fmt"

// ValidateDeclarations refuses the record kinds, link types and signal types of
// one unit that core could not carry. It runs each one's Validate and refuses a
// name the unit declares twice. Core's boot runs it for every unit. A unit's
// tests run it through extensiontest, so both answer by one set of rules.
func (e Extension) ValidateDeclarations() error {
	if err := declaredOnce("link type", e.LinkTypes, func(t LinkType) (string, error) { return t.Type, t.Validate() }); err != nil {
		return err
	}
	if err := declaredOnce("record kind", e.RecordKinds, func(k RecordKindDeclaration) (string, error) { return k.Kind, k.Validate(e.Name) }); err != nil {
		return err
	}
	return declaredOnce("signal type", e.SignalTypes, func(d SignalTypeDeclaration) (string, error) { return d.Kind, d.Validate() })
}

// declaredOnce validates each declaration of one kind and refuses a name used
// twice.
func declaredOnce[D any](what string, declarations []D, check func(D) (string, error)) error {
	seen := make(map[string]bool, len(declarations))
	for _, d := range declarations {
		name, err := check(d)
		if err != nil {
			return err
		}
		if seen[name] {
			return fmt.Errorf("declares %s %q twice", what, name)
		}
		seen[name] = true
	}
	return nil
}
