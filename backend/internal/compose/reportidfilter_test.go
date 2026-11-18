// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

func TestAMalformedIDFilterIsRefusedAsTheCallersMistake(t *testing.T) {
	spec := prebuiltReports["deals-by-stage"]
	for _, filter := range []string{fieldPipelineID, fieldOwnerID, fieldCompanyID} {
		err := requireIDFilterValues(spec, map[string]any{filter: "undefined"})
		var refusal *FilterIDValueError
		if !errors.As(err, &refusal) || refusal.Filter != filter {
			t.Fatalf("%s=undefined: err = %v, want a FilterIDValueError naming the filter", filter, err)
		}
		fault, ok := httperr.Classify(err)
		if !ok || fault.Status != http.StatusUnprocessableEntity || fault.Code != reportFieldNotAllowedCode {
			t.Errorf("%s=undefined answers %d %q, want 422 %s", filter, fault.Status, fault.Code, reportFieldNotAllowedCode)
		}
		if fault.InfraCause != nil {
			t.Errorf("%s=undefined carries an infrastructure cause, which is logged as an ERROR", filter)
		}
	}
}

func TestAWellFormedOrNonIDFilterPassesTheIDCheck(t *testing.T) {
	spec := prebuiltReports["deals-by-stage"]
	filters := map[string]any{
		fieldPipelineID: ids.NewV7().String(),
		fieldOwnerID:    nil,
		fieldStatus:     "won",
		"unknown":       "undefined",
	}
	if err := requireIDFilterValues(spec, filters); err != nil {
		t.Fatalf("a canonical id, an unset id and a text filter were refused: %v", err)
	}
	var notBindable *FilterValueNotAllowedError
	if err := requireIDFilterValues(spec, map[string]any{fieldPipelineID: true}); !errors.As(err, &notBindable) {
		t.Errorf("a yes-or-no for an id filter: err = %v, want FilterValueNotAllowedError", err)
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
