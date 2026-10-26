// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package privacy carries the personal-data declaration of the published
// extension surface. A unit states, per table it owns, which core record each
// row belongs to, which columns hold personal data, and the retention class.
// The core's eraser, export and dependent-record guard read it; a unit never
// runs its own erasure.
//
// These types evolve additively or through versioned successors, never in
// place.
//
//margince:extension-surface
package privacy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
)

// RecordKind is a core record a unit row can belong to. Closed: the eraser can
// only resolve the kinds it erases or merges.
type RecordKind string

// The record kinds a unit row can link to.
const (
	Contact  RecordKind = "contact"
	Lead     RecordKind = "lead"
	Company  RecordKind = "company"
	Activity RecordKind = "activity"
)

// Validate enforces membership in the closed set.
func (k RecordKind) Validate() error {
	switch k {
	case Contact, Lead, Company, Activity:
		return nil
	}
	return fmt.Errorf("record kind %q is not one the eraser resolves (contact, lead, company, activity)", string(k))
}

// Link says that one column of a unit table holds the id of a core record.
type Link struct {
	Column string
	Record RecordKind
}

// Table is the declaration for one table the unit owns.
type Table struct {
	// Name is the complete `ext_<namespace>_<table>` identifier, unqualified.
	// A table with personal columns must have a uuid `id` column: the core
	// audits each row it clears by that id.
	Name string

	// Links are the columns pointing at core records. A row is the subject's
	// when any link names the subject. A table with personal columns needs one,
	// or the core cannot find its rows. A table with neither declares that it
	// holds no personal data.
	Links []Link

	// Personal are the columns that hold personal data. The core clears them
	// when a linked record is erased and serves them in a subject export.
	Personal []string

	// Retention is the class the rows are kept under, chosen from the core's
	// closed set; a unit cannot add one.
	Retention jurisdiction.RetentionClassName

	// AgeColumn is the timestamp or date column the core counts a row's age
	// from, in a retention sweep and against the retention class. A table that
	// leaves it empty is never swept: the core has no age to compare. The tag
	// keeps the digest of a declaration made before this field unchanged.
	AgeColumn string `json:"age_column,omitempty"`
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate checks one table against the unit's namespace (`ext_<name>`), so a
// unit cannot declare a table it does not own.
func (t Table) Validate(namespace string) error {
	if !strings.HasPrefix(t.Name, namespace+"_") || !identifier.MatchString(t.Name) || len(t.Name) > 63 {
		return fmt.Errorf("privacy table %q is not an identifier in this unit's %s_ namespace", t.Name, namespace)
	}
	if len(t.Links) == 0 && len(t.Personal) > 0 {
		return fmt.Errorf("privacy table %q declares personal columns and no link to a core record, so the core cannot find a subject's rows in it", t.Name)
	}
	if len(t.Personal) > 0 && !slices.ContainsFunc(t.Links, func(l Link) bool { return l.Record != Company }) {
		return fmt.Errorf("privacy table %q declares personal columns and links only to a company; erasure and export find rows by contact, lead or activity", t.Name)
	}
	columns := map[string]bool{}
	for _, l := range t.Links {
		if !identifier.MatchString(l.Column) {
			return fmt.Errorf("privacy table %q: link column %q is not an identifier", t.Name, l.Column)
		}
		if err := l.Record.Validate(); err != nil {
			return fmt.Errorf("privacy table %q: %w", t.Name, err)
		}
		if columns[l.Column] {
			return fmt.Errorf("privacy table %q links column %q twice", t.Name, l.Column)
		}
		columns[l.Column] = true
	}
	if err := t.validatePersonal(columns); err != nil {
		return err
	}
	if err := t.validateAgeColumn(); err != nil {
		return err
	}
	if err := t.Retention.Validate(); err != nil {
		return fmt.Errorf("privacy table %q: %w", t.Name, err)
	}
	return nil
}

// validateAgeColumn holds the age column to being an identifier the core does
// not clear. A cleared age would put the row back in front of the sweep.
func (t Table) validateAgeColumn() error {
	if t.AgeColumn != "" && (!identifier.MatchString(t.AgeColumn) || slices.Contains(t.Personal, t.AgeColumn)) {
		return fmt.Errorf("privacy table %q: age column %q is not an identifier, or is a personal column the core clears", t.Name, t.AgeColumn)
	}
	return nil
}

// validatePersonal checks the personal columns against the link columns, which
// hold the ids the core finds rows by and so cannot be cleared.
func (t Table) validatePersonal(links map[string]bool) error {
	personal := map[string]bool{}
	for _, c := range t.Personal {
		switch {
		case !identifier.MatchString(c):
			return fmt.Errorf("privacy table %q: personal column %q is not an identifier", t.Name, c)
		case personal[c]:
			return fmt.Errorf("privacy table %q lists personal column %q twice", t.Name, c)
		case links[c] || c == "id":
			return fmt.Errorf("privacy table %q: column %q is a link or the row id and cannot be cleared", t.Name, c)
		}
		personal[c] = true
	}
	return nil
}

// Digest is a stable fingerprint of a declaration set, independent of the order
// tables, links and columns were written in. The core records it per extension
// version, so a changed declaration shows up as a digest that no longer matches.
func Digest(tables []Table) (string, error) {
	canonical := make([]Table, len(tables))
	for i, t := range tables {
		t.Links = slices.SortedFunc(slices.Values(t.Links), func(a, b Link) int { return strings.Compare(a.Column, b.Column) })
		t.Personal = slices.Sorted(slices.Values(t.Personal))
		canonical[i] = t
	}
	slices.SortFunc(canonical, func(a, b Table) int { return strings.Compare(a.Name, b.Name) })
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("privacy declaration digest: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
