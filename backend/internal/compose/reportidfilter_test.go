// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
)

// idShapeGaps names each id field of the corpus whose expression the id check
// does not know. Every uuid field of the schema descriptors is named this way.
func idShapeGaps(specs map[string]reportSpec) (judged int, gaps []string) {
	for _, key := range slices.Sorted(maps.Keys(specs)) {
		spec := specs[key]
		for _, vocabulary := range []map[string]string{spec.filters, spec.dimensions} {
			for name, expr := range vocabulary {
				if name != "id" && !strings.HasSuffix(name, "_id") {
					continue
				}
				judged++
				if columnValueShapes[expr] != analyticsquery.ShapeID {
					gaps = append(gaps, key+"."+name)
				}
			}
		}
	}
	slices.Sort(gaps)
	return judged, slices.Compact(gaps)
}

func TestEveryIDNamedFieldReadsAnIDShapedColumn(t *testing.T) {
	judged, gaps := idShapeGaps(everyReportSpec(t))
	if judged == 0 {
		t.Fatal("no id field was judged; the corpus no longer reaches the report specs")
	}
	for _, gap := range gaps {
		t.Errorf("%s holds record ids, but its expression is not in columnValueShapes as ShapeID, "+
			"so a malformed id reaches Postgres and is logged as a server fault", gap)
	}
}

func TestTheIDShapeCensusCatchesAnIDColumnItDoesNotKnow(t *testing.T) {
	planted := map[string]reportSpec{
		"named": {filters: map[string]string{"lead_id": "t.lead_id"}},
		"row":   {dimensions: map[string]string{"id": "t.unknown_row"}},
		"text":  {filters: map[string]string{"source": "t.source"}},
	}
	_, gaps := idShapeGaps(planted)
	if want := []string{"named.lead_id", "row.id"}; !slices.Equal(gaps, want) {
		t.Errorf("gaps = %v, want %v: a text column was judged an id, or an id column was missed", gaps, want)
	}
}

func TestAReportSpecReadsItsIDFiltersAsIDShaped(t *testing.T) {
	spec := prebuiltReports["deals-by-stage"]
	for name, want := range map[string]analyticsquery.ColumnShape{
		fieldPipelineID: analyticsquery.ShapeID, fieldOwnerID: analyticsquery.ShapeID, fieldStageID: analyticsquery.ShapeID,
		fieldStatus: analyticsquery.ShapeText, "unknown": analyticsquery.ShapeText,
	} {
		if got := spec.valueShape(name); got != want {
			t.Errorf("deals-by-stage %s reads shape %q, want %q", name, got, want)
		}
	}
}

func TestEverySchemaUUIDFieldIsNamedAsAnID(t *testing.T) {
	for _, obj := range schemaObjects {
		for _, f := range obj.Fields {
			if f.Type == "uuid" && !strings.HasSuffix(f.Name, "_id") {
				t.Errorf("%s.%s holds ids under a name the id census does not read", obj.Type, f.Name)
			}
		}
	}
}
