// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H3

package gates

// The receipt field census, falsified: each finding it reports, planted.

import (
	"slices"
	"testing"
)

// plantedRule presents `name` and `owner_id` on a deal, reading the owner as
// `reading`, and declares the given columns internal.
func plantedRule(reading string, internal ...string) fieldClassification {
	return fieldClassification{
		presents: func(_, column string) bool { return column == "name" || column == "owner_id" },
		internal: func(_, column string) bool { return slices.Contains(internal, column) },
		reading: func(_, column string) string {
			if column == "owner_id" {
				return reading
			}
			return "text"
		},
		declared: map[string][]string{"deal": internal},
	}
}

func plantedDealTable(columns ...string) recordTables {
	return recordTables{
		columns:    map[string][]string{"deal": columns},
		references: map[string]map[string]string{"deal": {"owner_id": "app_user"}},
	}
}

func TestAColumnNobodyClassifiedIsReported(t *testing.T) {
	t.Parallel()
	findings := magicFieldFindings(plantedDealTable("name", "owner_id", "version", "planted_column"),
		plantedRule("member", "version"))
	if !mentionsAll(findings, "deal.planted_column", "neither presented nor declared internal") || len(findings) != 1 {
		t.Errorf("an unclassified column was not reported alone: %v", findings)
	}
}

func TestAnInternalDeclarationForAMissingColumnIsReported(t *testing.T) {
	t.Parallel()
	findings := magicFieldFindings(plantedDealTable("name", "owner_id"), plantedRule("member", "dropped_column"))
	if !mentionsAll(findings, "deal.dropped_column", "no such column") {
		t.Errorf("a stale internal declaration was not reported: %v", findings)
	}
}

func TestAPresentedColumnDeclaredInternalIsReported(t *testing.T) {
	t.Parallel()
	findings := magicFieldFindings(plantedDealTable("name", "owner_id"), plantedRule("member", "name"))
	if !mentionsAll(findings, "deal.name", "declared internal but presented") {
		t.Errorf("a contradictory classification was not reported: %v", findings)
	}
}

func TestAMemberColumnReadAsAPlainReferenceIsReported(t *testing.T) {
	t.Parallel()
	findings := magicFieldFindings(plantedDealTable("name", "owner_id"), plantedRule("reference"))
	if !mentionsAll(findings, "deal.owner_id", "never named") {
		t.Errorf("a member column read without its name was not reported: %v", findings)
	}
}

func TestASyntheticKeyOnOneSideOnlyIsReported(t *testing.T) {
	t.Parallel()
	findings := syntheticKeyFindings([]string{"note", "server_only"}, map[string]bool{"note": true, "label_only": true})
	if !mentionsAll(findings, `"server_only"`, "no word for it") || !mentionsAll(findings, `"label_only"`, "never shows it") {
		t.Errorf("a one-sided synthetic key was not reported both ways: %v", findings)
	}
}
