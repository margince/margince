// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The drill-through writes each row's display name into a column of its own,
// and that column's name has to stay free.
//
// A spec that declared its own `label` dimension or measure would have the
// enrichment overwrite the queried value after the scan — silently, because
// the row already has the key and the arithmetic never reads it. The number
// would still reconcile and the column would show a display name where the
// spec promised something else.
//
// Both corpora come from the registries themselves, so a report or a schema
// field added tomorrow is checked by being declared rather than by being
// remembered here.

import "testing"

func TestNoReportSpecClaimsTheDrillThroughsLabelColumn(t *testing.T) {
	t.Parallel()
	if len(prebuiltReports) == 0 {
		t.Fatal("found no prebuilt reports at all — the scan is reading the wrong " +
			"registry, and would report PASS over a tree where every spec collided")
	}
	for name, spec := range prebuiltReports {
		if _, taken := spec.dimensions[derivationLabelColumn]; taken {
			t.Errorf("report %q declares a %q dimension, which the drill-through's "+
				"display name would overwrite after the query ran. Rename the "+
				"dimension, or give the label column a name no spec uses.",
				name, derivationLabelColumn)
		}
		if _, taken := spec.measures[derivationLabelColumn]; taken {
			t.Errorf("report %q declares a %q measure, which the drill-through's "+
				"display name would overwrite after the query ran. Rename the "+
				"measure, or give the label column a name no spec uses.",
				name, derivationLabelColumn)
		}
	}
}

// The ad-hoc plan (runAdHocPlan) builds its vocabulary from the schema
// descriptors rather than from prebuiltReports: EVERY declared field becomes a
// dimension. So a field named "label" collides through a path the check above
// cannot see, and a gate blind to half its subject reads PASS either way.
func TestNoSchemaFieldClaimsTheDrillThroughsLabelColumn(t *testing.T) {
	t.Parallel()
	if len(schemaObjects) == 0 {
		t.Fatal("found no schema objects at all — the scan is reading the wrong " +
			"registry, and would report PASS over a tree where every field collided")
	}
	for _, obj := range schemaObjects {
		for _, field := range obj.Fields {
			if field.Name == derivationLabelColumn {
				t.Errorf("%s declares a %q field, which becomes an ad-hoc report "+
					"dimension and would be overwritten by the drill-through's "+
					"display name. Rename the field, or give the label column a "+
					"name no schema uses.", obj.Type, derivationLabelColumn)
			}
		}
	}
}
