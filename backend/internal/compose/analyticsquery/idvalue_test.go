// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"errors"
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func ownerSchema() Schema {
	schema := testSchema()
	entity := schema.Entities["deals"]
	fields := maps.Clone(entity.Fields)
	fields["owner_id"] = Field{Name: "owner_id", Expr: "t.owner_id", Kind: KindFilter, Shape: ShapeID}
	entity.Fields = fields
	schema.Entities["deals"] = entity
	return schema
}

//craft:ignore naked-any a filter value is caller JSON, any by construction
func compileOwnerFilter(op FilterOp, value any) error {
	_, err := Compile(Query{
		Entity:   "deals",
		Measures: []Measure{{Fn: CountAll, As: "n"}},
		Filters:  []Filter{{Field: "owner_id", Op: op, Value: value}},
	}, ownerSchema(), noScope)
	return err
}

func TestTextThatIsNoIDIsRefusedOnAnIDColumnBeforeItBinds(t *testing.T) {
	t.Parallel()
	for _, op := range []FilterOp{OpEq, OpNe, OpGt} {
		err := compileOwnerFilter(op, "undefined")
		var refusal *RefusalError
		if !errors.As(err, &refusal) || refusal.Kind != RefusalInvalid || refusal.Suggest != IDValueAdvice {
			t.Errorf("owner_id %s undefined: err = %v, want an invalid refusal advising the id form", op, err)
		}
	}
	var refusal *RefusalError
	if err := compileOwnerFilter(OpEq, float64(7)); !errors.As(err, &refusal) || refusal.Suggest != IDValueAdvice {
		t.Errorf("owner_id eq 7: err = %v, want the id form as the advice", err)
	}
}

func TestACanonicalIDAndANullCheckCompileOnAnIDColumn(t *testing.T) {
	t.Parallel()
	if err := compileOwnerFilter(OpEq, "0190a6a4-7b7e-7cc0-8d3a-3f6c2b1e9a10"); err != nil {
		t.Errorf("a canonical id was refused: %v", err)
	}
	if err := compileOwnerFilter(OpIsNull, nil); err != nil {
		t.Errorf("a null check was refused: %v", err)
	}
}

func idShaped(name string) ColumnShape {
	if name == "owner_id" {
		return ShapeID
	}
	return ShapeText
}

func TestAReportFilterThatIsNoIDIsRefusedWithTheDoorsCode(t *testing.T) {
	t.Parallel()
	for _, value := range []any{"undefined", true} {
		err := RequireIDValues(map[string]any{"owner_id": value, "status": "won"}, "report_field_not_allowed", idShaped)
		var refusal *IDValueError
		if !errors.As(err, &refusal) || refusal.Filter != "owner_id" {
			t.Fatalf("owner_id=%v: err = %v, want an IDValueError naming the filter", value, err)
		}
		fault, ok := httperr.Classify(err)
		if !ok || fault.Status != http.StatusUnprocessableEntity || fault.Code != "report_field_not_allowed" || fault.InfraCause != nil {
			t.Errorf("owner_id=%v answers %d %q (cause %v), want 422 report_field_not_allowed and no cause",
				value, fault.Status, fault.Code, fault.InfraCause)
		}
	}
}

func TestACanonicalOrUnsetIDAndATextFilterPassTheReportIDCheck(t *testing.T) {
	t.Parallel()
	filters := map[string]any{"owner_id": "0190a6a4-7b7e-7cc0-8d3a-3f6c2b1e9a10", "status": "undefined"}
	if err := RequireIDValues(filters, "report_field_not_allowed", idShaped); err != nil {
		t.Errorf("a canonical id and a text filter were refused: %v", err)
	}
	if err := RequireIDValues(map[string]any{"owner_id": nil}, "report_field_not_allowed", idShaped); err != nil {
		t.Errorf("a null check was refused: %v", err)
	}
}
