// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func TestAFieldOutsideTheDealsPipelineRefusesAValueButNotANull(t *testing.T) {
	home, away := ids.New[ids.PipelineKind](), ids.New[ids.PipelineKind]()
	columns := []fieldcatalog.Column{
		{Name: "cf_scoped", Type: fieldcatalog.TypeText, PipelineIDs: []ids.UUID{home.UUID}},
		{Name: "cf_everywhere", Type: fieldcatalog.TypeText},
	}
	values := map[string]any{"cf_scoped": "gold", "cf_everywhere": "north"}

	if err := checkPipelineScope(home, columns, values); err != nil {
		t.Fatalf("a value on the field's own pipeline was refused: %v", err)
	}
	err := checkPipelineScope(away, columns, values)
	field, refused := FieldOutsidePipeline(err)
	if !refused || field != "cf_scoped" {
		t.Fatalf("a value on another pipeline answered %v, want a refusal naming cf_scoped", err)
	}
	if err := checkPipelineScope(away, columns, map[string]any{"cf_scoped": nil, "cf_everywhere": "north"}); err != nil {
		t.Fatalf("a null on another pipeline was refused: %v", err)
	}
	if _, refused := FieldOutsidePipeline(nil); refused {
		t.Fatal("no error named a field")
	}
}
