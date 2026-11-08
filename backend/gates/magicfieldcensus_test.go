// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every column of a record a reader edits is shown on a receipt or declared
// internal. A receipt shows only what internal/compose/magic/presentation.go
// presents, so an unclassified column is hidden without anyone deciding it.
// The corpus is the committed schema: a new column fails here until classified.

import (
	"fmt"
	"slices"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/compose/magic"
)

// minColumnsPerRecordTable guards against a parse that lost a table's columns:
// the smallest governed table has more than twenty.
const minColumnsPerRecordTable = 15

// recordTables is the schema side of the census: each table's columns, and
// the table each column's foreign key names. A composite key is filed under its
// first column, which is the one that names the row (`stage_id` of
// `(stage_id, pipeline_id)`).
type recordTables struct {
	columns    map[string][]string
	references map[string]map[string]string
}

func readRecordTables(t *testing.T, tables []string) recordTables {
	t.Helper()
	schema := parseHeadCatalog(t)
	out := recordTables{columns: map[string][]string{}, references: map[string]map[string]string{}}
	for _, name := range tables {
		table, ok := schema.tables[name]
		if !ok || len(table.columns) < minColumnsPerRecordTable {
			t.Fatalf("the head catalog holds too few columns for %s to be the schema", name)
		}
		for _, column := range table.columns {
			out.columns[name] = append(out.columns[name], column.name)
		}
		out.references[name] = map[string]string{}
		for _, fk := range table.foreignKeys {
			if m := catalogForeignKey.FindStringSubmatch(fk.def); m != nil {
				out.references[name][splitList(m[1])[0]] = m[2]
			}
		}
	}
	return out
}

// fieldClassification is the magic package's answer for one record type.
type fieldClassification struct {
	presents func(entityType, column string) bool
	internal func(entityType, column string) bool
	reading  func(entityType, column string) string
	declared map[string][]string
}

func liveFieldClassification() fieldClassification {
	return fieldClassification{
		presents: magic.PresentsField, internal: magic.DeclaredInternal,
		reading: magic.ReadingOf, declared: magic.InternalColumnDeclarations(),
	}
}

// referenceReadings is the reading a presented column must have when its
// foreign key names one of these tables, so its value is labelled by name.
var referenceReadings = map[string]string{"app_user": "member", "stage": "stage"}

func magicFieldFindings(schema recordTables, rule fieldClassification) []string {
	var findings []string
	for table, columns := range schema.columns {
		for _, column := range columns {
			presented, internal := rule.presents(table, column), rule.internal(table, column)
			switch {
			case !presented && !internal:
				findings = append(findings, fmt.Sprintf("%s.%s is neither presented nor declared internal", table, column))
				continue
			case presented && internal:
				findings = append(findings, fmt.Sprintf("%s.%s is declared internal but presented", table, column))
			}
			want, labelled := referenceReadings[schema.references[table][column]]
			if presented && labelled && rule.reading(table, column) != want {
				findings = append(findings, fmt.Sprintf("%s.%s names a %s but reads as %q, so its value is never named",
					table, column, schema.references[table][column], rule.reading(table, column)))
			}
		}
	}
	for table, declared := range rule.declared {
		for _, column := range declared {
			if !slices.Contains(schema.columns[table], column) {
				findings = append(findings, fmt.Sprintf("%s.%s is declared internal but the table has no such column", table, column))
			}
		}
	}
	sort.Strings(findings)
	return findings
}

func TestEveryRecordColumnIsPresentedOrDeclaredInternal(t *testing.T) {
	t.Parallel()
	governed := magic.GovernedRecordTypes()
	if len(governed) < 6 {
		t.Fatalf("magic governs %d record types; the six a reader edits are the floor", len(governed))
	}
	for _, finding := range magicFieldFindings(readRecordTables(t, governed), liveFieldClassification()) {
		t.Error(finding + " — classify it in internal/compose/magic (presentation.go or fieldclasses.go)")
	}
}

// The server shows a hand-built audit key only where the History tab labels it.
// So the server's copy and the frontend map must hold one set.
func TestTheServerShowsExactlyTheSyntheticKeysTheHistoryTabLabels(t *testing.T) {
	t.Parallel()
	_, labelled := historyLabelMaps(t)
	for _, finding := range syntheticKeyFindings(magic.SyntheticAuditFields(), labelled) {
		t.Error(finding)
	}
}

func syntheticKeyFindings(server []string, labelled map[string]bool) []string {
	var findings []string
	for _, key := range server {
		if !labelled[key] {
			findings = append(findings, fmt.Sprintf("the server shows %q but SYNTHETIC_AUDIT_FIELD_LABELS has no word for it", key))
		}
	}
	for key := range labelled {
		if !slices.Contains(server, key) {
			findings = append(findings, fmt.Sprintf("SYNTHETIC_AUDIT_FIELD_LABELS labels %q but the server never shows it", key))
		}
	}
	sort.Strings(findings)
	return findings
}
