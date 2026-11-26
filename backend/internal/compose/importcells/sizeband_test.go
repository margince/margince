// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importcells

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/importguide"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A refused size band lists the bands from the smallest company to the largest.
func TestARefusedSizeBandListsTheBandsBySize(t *testing.T) {
	if got := strings.Join(SizeBandVocabulary(), ", "); got != "1-10, 11-50, 51-200, 201-500, 501-1000, 1001-5000, 5000+" {
		t.Fatalf("bands = %q, want them listed by size", got)
	}
}

func TestTheSizeBandExampleNamesOnlyBandsTheImportTakes(t *testing.T) {
	for _, lang := range textlang.Shipped {
		for band := range strings.SplitSeq(importguide.Columns["size_band"].Example.In(lang), " · ") {
			if !IsSizeBand(band) {
				t.Errorf("%s example names %q, which the import refuses", lang, band)
			}
		}
	}
}
