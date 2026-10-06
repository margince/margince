// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every embedding model a shipped preset binds has a measured grounding floor, or
// is named as not yet measured (issue 6911).
//
// A binding with no measured value has no floor, so a preset that ships one
// ships a corpus control that gates nothing. The corpus is the preset directory,
// so adding a preset with a new embedding model fails here until its row exists.

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
)

// unmeasuredPresetModels are bound by a shipped preset and not yet measured;
// startup warns for each. Entries leave this set when the registry gains the
// row, and the test fails on an entry that has been measured.
var unmeasuredPresetModels = map[string]bool{
	"bge-m3":      true,
	"BAAI/bge-m3": true,
}

func TestEveryPresetEmbeddingModelHasAMeasuredFloor(t *testing.T) {
	bound := 0
	for _, path := range presetFiles(t) {
		emb := routingFromPreset(t, path).Embeddings
		if emb.Model == "" {
			continue
		}
		bound++
		identity := fmt.Sprintf("%s/%s@%d", emb.Provider, emb.Model, emb.Dimensions)
		_, measured := knowledge.MeasurementFor(identity)
		if measured && unmeasuredPresetModels[emb.Model] {
			t.Errorf("%q is measured now: drop it from unmeasuredPresetModels", emb.Model)
		}
		if !measured && !unmeasuredPresetModels[emb.Model] {
			t.Errorf("%s binds embedding model %q with no measured grounding floor: measure it and add it to measuredFloors in modules/knowledge/groundingfloor.go", path, emb.Model)
		}
	}
	if bound == 0 {
		t.Fatal("no preset binds an embedding model, so this gate read an empty subject")
	}
}
