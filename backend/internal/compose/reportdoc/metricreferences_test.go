// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportdoc

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMetricReferencesCannotMasqueradeAsLiveRuns(t *testing.T) {
	edition := &crmcontracts.ReportEditionReference{EditionId: openapi_types.UUID(ids.NewV7()), Metric: "bookings_won"}
	doc := Document{Blocks: []Block{{Kind: KindStatStrip, Cells: []Cell{{EditionRef: edition}}}}}
	runs, err := Validate(doc)
	if err != nil || len(runs) != 0 {
		t.Fatalf("frozen reference was treated as a run: %v %v", runs, err)
	}
	doc.Blocks[0].Cells[0].RunID = ids.NewV7().String()
	if _, err := Validate(doc); err == nil {
		t.Fatal("ambiguous live and frozen reference accepted")
	}
	doc.Blocks[0].Cells[0].RunID = ""
	doc.Blocks[0].Cells[0].Column = "total"
	if _, err := Validate(doc); err == nil {
		t.Fatal("frozen reference accepted a live-run column")
	}
}
