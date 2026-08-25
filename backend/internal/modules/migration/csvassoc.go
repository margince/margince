// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The edges a delimited file carries.
//
// A flat file has no edge table, and for a long time this source answered that
// it therefore has no edges at all. That was true of its SHAPE and false of its
// CONTENT: a contact export names each person's employer in a column, which is a
// relationship between two records written the only way a spreadsheet can write
// one — by name, in a cell on the row.
//
// So one edge per row that names a company, and the endpoints are found in two
// different ways. The person is named by the run's source key, which the identity
// map can resolve because this run is landing that person. The company is named
// by TEXT, which no identity map can resolve, so it travels as text and the
// writer resolves it. `AssocTargetOrganizationName` exists to say that
// explicitly: an endpoint that is a name and not an id must not be handed to a
// lookup that answers ids.

import (
	"context"
	"fmt"
	"strings"
)

// AssocTargetOrganizationName is the `To` endpoint kind for an edge whose other
// end is a company NAME rather than a company id.
//
// Named apart from ObjectOrganization on purpose. A writer that saw
// `organization` there would be right to send the value to its identity map,
// which holds ids — and would get nothing, silently, for every row.
//
// It doubles as the mapping target a person file points its company column at:
// the string the caller maps to and the string the edge travels under are one
// value, so the two sides cannot drift.
const AssocTargetOrganizationName = "organization_name"

// assocCategoryEmployment is what the edge means: this person works here.
const assocCategoryEmployment = "employment"

// Associations answers one edge per row naming an employer.
//
// It re-walks the file, which is the same thing every other read of this source
// does — Counts walks it, and each page of Rows walks it from the top. The whole
// file is bounded by the upload cap, and one edge is two short strings, so the
// all-at-once shape the Source interface asks for costs a few hundred kilobytes
// on the largest file this door accepts. It does not need paging; a later reader
// tempted to add some should know the bound is the upload limit, not the row
// count.
//
// Empty for every object but a person run, and for a person run whose mapping
// names no company column: an organization or lead import is unaffected by this
// file existing.
func (s *CSVSource) Associations(ctx context.Context) ([]Assoc, error) {
	if s.object != ObjectPerson {
		return nil, nil
	}
	column := s.columnFor(AssocTargetOrganizationName)
	if column == "" {
		return nil, nil
	}

	var out []Assoc
	// The SAME claim rule Rows applies, and it is load-bearing rather than
	// defensive. A row whose source key an earlier line already claimed is NOT
	// delivered — so no person is landed for it — and the identity map holds the
	// earlier row's person under that key. An edge emitted for the undelivered
	// row would resolve its `From` endpoint to the FIRST row's person and attach
	// a human to a company they have nothing to do with. The rule is not
	// duplicated for tidiness; leaving it out writes wrong data.
	claimed := map[string]bool{}
	err := s.walk(ctx, func(line int, record []string, index map[string]int) error {
		row, ok := s.rowFrom(line, record, index)
		if !ok {
			// Already disclosed by rowFrom on its own walk. Saying it twice would
			// put one unusable row in the report as two.
			return nil
		}
		if claimed[row.ExternalID] {
			return nil
		}
		claimed[row.ExternalID] = true

		at, ok := index[column]
		if !ok || at >= len(record) {
			return nil
		}
		name := strings.TrimSpace(record[at])
		if name == "" {
			// The file said nothing about this person's employer, which is not
			// the same as saying they have none. No edge, no disclosure.
			return nil
		}
		out = append(out, Assoc{
			FromType: ObjectPerson,
			FromID:   row.ExternalID,
			ToType:   AssocTargetOrganizationName,
			ToID:     name,
			Category: assocCategoryEmployment,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the employer column: %w", err)
	}
	return out, nil
}

// columnFor answers which source column was mapped onto a target.
func (s *CSVSource) columnFor(target string) string {
	for column, mapped := range s.mapping {
		if mapped == target {
			return column
		}
	}
	return ""
}
