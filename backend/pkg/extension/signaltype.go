// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"slices"
	"strings"
)

// SignalKindPrefix opens every signal kind a unit declares. The spelling is
// permanent once a row carries it, because the signal table's kind CHECK
// accepts it as a pattern.
const SignalKindPrefix = "ext:"

// SignalFieldType is the type of one evidence field.
type SignalFieldType string

// The evidence member types a layout may declare.
const (
	SignalFieldText   SignalFieldType = "text"
	SignalFieldNumber SignalFieldType = "number"
	SignalFieldBool   SignalFieldType = "bool"
)

// SignalEvidenceField is one named member of a signal type's evidence layout.
type SignalEvidenceField struct {
	// Name is `[a-z][a-z0-9_]*`, at most 32 characters.
	Name string
	// Label is what a screen shows, in English.
	Label string
	Type  SignalFieldType
	// Required makes a signal without the member a refusal.
	Required bool
	// Personal marks a member that holds personal data. Erasing the contact a
	// signal is about removes every such member from it.
	Personal bool
}

// SignalTypeDeclaration is one kind of signal a unit adds beside core's own. It
// is inert data: the layout says what the evidence may hold, and core checks
// every signal against it. Core adds a `provenance` member to every signal,
// which the unit can neither set nor omit.
type SignalTypeDeclaration struct {
	// Kind is the short name, `[a-z][a-z0-9_]*`, at most 32 characters.
	Kind string
	// Label is what a screen shows, in English. It is also the summary of a
	// signal whose evidence names none.
	Label    string
	Evidence []SignalEvidenceField
}

// SignalEvidence is what a unit reports with a signal.
type SignalEvidence struct {
	// Summary is the sentence a reader sees. Empty means the type's label.
	Summary string
	// Fields are the layout's members. Their names and types are checked
	// against the declared layout.
	Fields map[string]any
}

// SignalSubject is the record a signal is about: a deal, company, contact or
// project.
type SignalSubject struct {
	Type string
	ID   string
}

// SignalKindName is the stored and wire form of a unit's signal kind. It is the
// one place the form is built, so a unit can only name a kind under its own name.
func SignalKindName(unit Name, kind string) string {
	return SignalKindPrefix + string(unit) + ":" + kind
}

// Validate refuses a type core could not carry.
func (d SignalTypeDeclaration) Validate() error {
	if !recordKindGrammar.MatchString(d.Kind) || len(d.Kind) > maxRecordKindLength {
		return fmt.Errorf("signal type %q is not a valid name (a lower-case letter, then lower-case letters, digits and underscores, at most %d characters)", d.Kind, maxRecordKindLength)
	}
	if strings.TrimSpace(d.Label) == "" {
		return fmt.Errorf("signal type %q has no label", d.Kind)
	}
	seen := make(map[string]bool, len(d.Evidence))
	for _, f := range d.Evidence {
		if !recordKindGrammar.MatchString(f.Name) || len(f.Name) > maxRecordKindLength {
			return fmt.Errorf("signal type %q: %q is not a valid evidence field name", d.Kind, f.Name)
		}
		if seen[f.Name] {
			return fmt.Errorf("signal type %q declares evidence field %q twice", d.Kind, f.Name)
		}
		seen[f.Name] = true
		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("signal type %q: evidence field %q has no label", d.Kind, f.Name)
		}
		if !slices.Contains([]SignalFieldType{SignalFieldText, SignalFieldNumber, SignalFieldBool}, f.Type) {
			return fmt.Errorf("signal type %q: evidence field %q has unknown type %q", d.Kind, f.Name, f.Type)
		}
	}
	return nil
}

// CheckFields refuses evidence that does not fit the layout. It refuses a member
// the layout does not name, a required member left out and a member of the wrong
// type. A number is a float64 after JSON decoding and an int from Go code.
func (d SignalTypeDeclaration) CheckFields(fields map[string]any) error {
	declared := make(map[string]SignalEvidenceField, len(d.Evidence))
	for _, f := range d.Evidence {
		declared[f.Name] = f
	}
	for name, v := range fields {
		f, ok := declared[name]
		if !ok {
			return fmt.Errorf("evidence member %q is not in the layout of signal type %q", name, d.Kind)
		}
		if !fieldTypeFits(f.Type, v) {
			return fmt.Errorf("evidence member %q of signal type %q must be a %s", name, d.Kind, f.Type)
		}
	}
	for _, f := range d.Evidence {
		if _, ok := fields[f.Name]; f.Required && !ok {
			return fmt.Errorf("evidence member %q of signal type %q is required", f.Name, d.Kind)
		}
	}
	return nil
}

//craft:ignore naked-any evidence members are JSON values of the declared type, checked here
func fieldTypeFits(t SignalFieldType, v any) bool {
	switch t {
	case SignalFieldText:
		_, ok := v.(string)
		return ok
	case SignalFieldBool:
		_, ok := v.(bool)
		return ok
	case SignalFieldNumber:
		switch v.(type) {
		case float64, float32, int, int32, int64:
			return true
		}
	}
	return false
}

// PersonalFields names the layout's members that hold personal data.
func (d SignalTypeDeclaration) PersonalFields() []string {
	var out []string
	for _, f := range d.Evidence {
		if f.Personal {
			out = append(out, f.Name)
		}
	}
	return out
}
