// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// RecordKindPrefix opens every record kind a unit declares, so a stored kind
// says at a glance that it is not one of core's. The spelling is permanent once
// data carries it: every closed kind list in the schema accepts it as a pattern.
const RecordKindPrefix = "ext:"

// maxRecordKindLength caps the kind half. With the unit name capped at 32 and
// the two separators, a full name stays inside 68 characters.
const maxRecordKindLength = 32

// recordKindGrammar is the kind half of `ext:<unit>:<kind>`: a lower-case
// letter, then lower-case letters, digits and underscores. The schema CHECKs
// spell the same shape, so a name this accepts is one every table accepts.
var recordKindGrammar = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// DeletePolicy says what removing a record of a kind means.
type DeletePolicy string

const (
	// DeleteArchiveOnly keeps the row and marks it archived. It is the default.
	DeleteArchiveOnly DeletePolicy = "archive_only"
	// DeleteErasable lets the row be erased for good, through the same
	// erasure path core records use.
	DeleteErasable DeletePolicy = "erasable"
)

// RecordKindField is one named value a record of the kind carries.
type RecordKindField struct {
	// Name is the column the unit's table keeps the value in.
	Name string
	// Label is what a screen shows, in English.
	Label string
}

// RecordKindDeclaration is one kind of record a unit adds beside core's own. It
// is inert data. The unit owns the table and its rows. Core reads this to treat
// the kind as a real record in links, tags, lists, search and the audit log. It
// is not RecordKind, which is what an ingress run lands.
type RecordKindDeclaration struct {
	// Kind is the short name, `[a-z][a-z0-9_]*`, at most 32 characters.
	Kind string
	// DisplayName is what a screen calls one record of the kind.
	DisplayName string
	// Table is the unit's own table, `ext_<unit>_<name>`. It keeps an `id uuid`
	// primary key and an `archived_at timestamptz` column, which is the archive
	// state core reads to refuse a link to an archived record.
	Table string
	// Fields are the values a record carries.
	Fields []RecordKindField
	// NaturalKey names the fields that together identify one record. The unit's
	// table keeps a unique index over them.
	NaturalKey []string
	// Defaults are each system role's starting rights on the kind. A role the
	// list does not name holds none: every action is denied until granted.
	Defaults []RbacDefault
	// DeletePolicy defaults to DeleteArchiveOnly.
	DeletePolicy DeletePolicy
	// Timeline, when set, puts the kind's rows into the History of the core
	// record they reference. Core builds the entry; the unit names three columns.
	Timeline *RecordKindTimeline
}

// RecordKindTimeline says how a row of a kind reads as a History entry.
type RecordKindTimeline struct {
	// LabelField is the declared field whose text is the entry's title.
	LabelField string
	// AtColumn is the timestamptz column that places the entry in time.
	AtColumn string
	// Subjects are the core records a row can reference, one column each.
	Subjects []TimelineSubject
}

// TimelineSubject is one core record a row references.
type TimelineSubject struct {
	// Entity is `contact`, `company` or `deal`.
	Entity string
	// Column is the uuid column of the unit's table that holds that record's id.
	Column string
}

// timelineEntities are the core records a History is read for. A lead or a
// project has no extension rows today.
var timelineEntities = map[string]bool{"contact": true, "company": true, "deal": true}

func (k RecordKindDeclaration) validateTimeline() error {
	tl := k.Timeline
	if tl == nil {
		return nil
	}
	if !slices.ContainsFunc(k.Fields, func(f RecordKindField) bool { return f.Name == tl.LabelField }) {
		return fmt.Errorf("record kind %q timeline label names %q, which is not a declared field", k.Kind, tl.LabelField)
	}
	if !recordKindGrammar.MatchString(tl.AtColumn) {
		return fmt.Errorf("record kind %q timeline time %q is not a column name", k.Kind, tl.AtColumn)
	}
	if len(tl.Subjects) == 0 {
		return fmt.Errorf("record kind %q timeline names no core record", k.Kind)
	}
	seen := map[string]bool{}
	for _, s := range tl.Subjects {
		if !timelineEntities[s.Entity] {
			return fmt.Errorf("record kind %q timeline names %q, which is not contact, company or deal", k.Kind, s.Entity)
		}
		if !recordKindGrammar.MatchString(s.Column) {
			return fmt.Errorf("record kind %q timeline column %q is not a column name", k.Kind, s.Column)
		}
		if seen[s.Entity] {
			return fmt.Errorf("record kind %q timeline names %q twice", k.Kind, s.Entity)
		}
		seen[s.Entity] = true
	}
	return nil
}

// RecordKindName is the stored and wire form of a unit's kind. It is the one
// place the form is built, so a unit can only name a kind under its own name.
func RecordKindName(unit Name, kind string) string {
	return RecordKindPrefix + string(unit) + ":" + kind
}

// RecordKindObject is the RBAC object that gates the kind. The unit name's
// hyphens become underscores here, so two units can derive one object; compose
// refuses that at boot.
func RecordKindObject(unit Name, kind string) string {
	return NamespacePrefix + strings.ReplaceAll(string(unit), "-", "_") + "_" + kind
}

// ParseRecordKindName splits `ext:<unit>:<kind>` and reports false for anything
// else, including a core kind and a name whose halves break their grammar.
func ParseRecordKindName(name string) (Name, string, bool) {
	rest, ok := strings.CutPrefix(name, RecordKindPrefix)
	if !ok {
		return "", "", false
	}
	unit, kind, ok := strings.Cut(rest, ":")
	if !ok || Name(unit).Validate() != nil || validRecordKindName(kind) != nil {
		return "", "", false
	}
	return Name(unit), kind, true
}

func validRecordKindName(kind string) error {
	if !recordKindGrammar.MatchString(kind) || len(kind) > maxRecordKindLength {
		return fmt.Errorf("record kind %q is not a valid kind name (a lower-case letter, then lower-case letters, digits and underscores, at most %d characters)", kind, maxRecordKindLength)
	}
	return nil
}

// Validate refuses a declaration core could not carry. The unit is the one
// declaring it, because the table must sit in that unit's namespace.
func (k RecordKindDeclaration) Validate(unit Name) error {
	if err := validRecordKindName(k.Kind); err != nil {
		return err
	}
	if strings.TrimSpace(k.DisplayName) == "" {
		return fmt.Errorf("record kind %q has no display name", k.Kind)
	}
	ns, err := unit.Namespace()
	if err != nil {
		return err
	}
	if !strings.HasPrefix(k.Table, ns+"_") || strings.ContainsAny(k.Table, ". \"") {
		return fmt.Errorf("record kind %q names table %q, which is not in the unit's own namespace %s_<table>", k.Kind, k.Table, ns)
	}
	if err := k.validateFields(); err != nil {
		return err
	}
	if err := k.validateTimeline(); err != nil {
		return err
	}
	switch k.DeletePolicy {
	case "", DeleteArchiveOnly, DeleteErasable:
	default:
		return fmt.Errorf("record kind %q has delete policy %q, which is neither %q nor %q", k.Kind, k.DeletePolicy, DeleteArchiveOnly, DeleteErasable)
	}
	for _, d := range k.Defaults {
		if strings.TrimSpace(d.Role) == "" {
			return fmt.Errorf("record kind %q has a default grant with no role", k.Kind)
		}
		for _, a := range d.Actions {
			if err := a.Validate(); err != nil {
				return fmt.Errorf("record kind %q: %w", k.Kind, err)
			}
		}
	}
	return nil
}

func (k RecordKindDeclaration) validateFields() error {
	if len(k.Fields) == 0 {
		return fmt.Errorf("record kind %q declares no fields", k.Kind)
	}
	declared := map[string]bool{}
	for _, f := range k.Fields {
		if !recordKindGrammar.MatchString(f.Name) {
			return fmt.Errorf("record kind %q has field %q, which is not a column name", k.Kind, f.Name)
		}
		if strings.TrimSpace(f.Label) == "" {
			return fmt.Errorf("record kind %q field %q has no label", k.Kind, f.Name)
		}
		if declared[f.Name] {
			return fmt.Errorf("record kind %q declares field %q twice", k.Kind, f.Name)
		}
		declared[f.Name] = true
	}
	if len(k.NaturalKey) == 0 {
		return fmt.Errorf("record kind %q has no natural key", k.Kind)
	}
	seen := map[string]bool{}
	for _, n := range k.NaturalKey {
		if !declared[n] {
			return fmt.Errorf("record kind %q natural key names %q, which is not a declared field", k.Kind, n)
		}
		if seen[n] {
			return fmt.Errorf("record kind %q natural key names %q twice", k.Kind, n)
		}
		seen[n] = true
	}
	return nil
}
