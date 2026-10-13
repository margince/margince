// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// The edges a delimited file carries.
//
// A flat file has no edge table, and for a long time this source answered that
// it therefore has no edges at all. That was true of its SHAPE and false of its
// CONTENT: a contact export names each contact's employer in a column, which is a
// relationship between two records written the only way a spreadsheet can write
// one — by name, in a cell on the row.
//
// So one edge per row that names a company, and the endpoints are found in two
// different ways. The contact is named by the run's source key, which the identity
// map can resolve because this run is landing that contact. The company is named
// by TEXT, which no identity map can resolve, so it travels as text and the
// writer resolves it. `AssocTargetCompanyName` exists to say that
// explicitly: an endpoint that is a name and not an id must not be handed to a
// lookup that answers ids.

import (
	"context"
	"fmt"
	"strings"
)

// AssocTargetCompanyName is the `To` endpoint kind for an edge whose other
// end is a company NAME rather than a company id.
//
// Named apart from ObjectCompany on purpose. A writer that saw
// `company` there would be right to send the value to its identity map,
// which holds ids — and would get nothing, silently, for every row.
//
// It doubles as the mapping target a contact file points its company column at.
// compose assigns its own csvEmployerName from this constant rather than
// spelling it again, so both sides are one value by construction.
const AssocTargetCompanyName = "company_name"

// AssocTargetCompanyRef is the `To` endpoint kind for an edge naming a company
// by its ref in the source system. The company is a row of this upload or of an
// earlier import, or one Margince already holds.
const AssocTargetCompanyRef = "company_ref"

// employerTargets are the mapping targets whose column names a contact's
// employer, each emitted as an edge of its own kind.
var employerTargets = []string{AssocTargetCompanyName, AssocTargetCompanyRef}

// assocCategoryEmployment is what the edge means: this contact works here.
const assocCategoryEmployment = "employment"

// Associations answers one edge per row naming an employer.
//
// It walks the file once, as Counts does. The upload cap bounds the file and one
// edge is two short strings, so the whole answer costs a few hundred kilobytes
// at most. The bound is the upload limit, not the row count, so it needs no
// paging.
//
// Empty for every object but a contact run, and for a contact run whose mapping
// names no company column: a company or lead import is unaffected by this
// file existing.
func (s *CSVSource) Associations(ctx context.Context) ([]Assoc, error) {
	if s.object != ObjectContact {
		return nil, nil
	}
	columns := map[string]string{}
	for _, target := range employerTargets {
		if column := s.columnFor(target); column != "" {
			columns[target] = column
		}
	}
	if len(columns) == 0 {
		return nil, nil
	}

	var out []Assoc
	// The SAME claim rule Rows applies, and it is load-bearing rather than
	// defensive. A row whose source key an earlier line already claimed is NOT
	// delivered — so no contact is landed for it — and the identity map holds the
	// earlier row's contact under that key. An edge emitted for the undelivered
	// row would resolve its `From` endpoint to the FIRST row's contact and attach
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
		for _, target := range employerTargets {
			if edge, ok := employerEdge(row.ExternalID, target, columns[target], record, index); ok {
				out = append(out, edge)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("reading the employer column: %w", err)
	}
	return out, nil
}

// employerEdge is the edge one employer column of one row names. An empty cell
// says nothing about the contact's employer, which is not the same as saying
// they have none: no edge, no disclosure.
func employerEdge(from, target, column string, record []string, index map[string]int) (Assoc, bool) {
	at, ok := index[column]
	if column == "" || !ok || at >= len(record) {
		return Assoc{}, false
	}
	to := strings.TrimSpace(record[at])
	if to == "" {
		return Assoc{}, false
	}
	return Assoc{FromType: ObjectContact, FromID: from, ToType: target, ToID: to, Category: assocCategoryEmployment}, true
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
