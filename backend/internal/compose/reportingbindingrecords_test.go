// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"slices"
	"testing"
)

// Subject erasure and access match a bound filter by the subject's contact
// id, lead ids and addresses. A filter naming a contact or a lead by another
// id would slip past both. No filter names either today, and this census fails
// when one starts to.
func TestABindableFilterNamesNoContactOrLead(t *testing.T) {
	t.Parallel()
	named := 0
	for entity, spec := range prebuiltReports {
		fields := slices.Concat(slices.Collect(maps.Keys(spec.dimensions)), slices.Collect(maps.Keys(spec.filters)))
		for _, field := range fields {
			if slices.Contains(spec.rowKeys, field) {
				continue
			}
			table, names := filterRecordType(spec, field)
			if !names {
				continue
			}
			named++
			if table == tableContact || table == tableLead {
				t.Errorf("%s.%s filters by the id of a %s, which no binding scrub matches", entity, field, table)
			}
		}
	}
	if named == 0 {
		t.Fatal("no filter field names a record: the census reads nothing")
	}
}
