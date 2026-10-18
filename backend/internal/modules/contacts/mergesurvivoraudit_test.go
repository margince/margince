// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheSurvivorsMergeRowNamesTheRecordFoldedIn(t *testing.T) {
	source := ids.NewV7()
	before, after := survivorMergeImages(source, map[string]any{auditKeyFilled: map[string]any{"title": "CTO"}})

	if before[auditKeyMergedFrom] != nil {
		t.Errorf("before = %v, want the survivor to have had nothing folded in", before)
	}
	if after[auditKeyMergedFrom] != source {
		t.Errorf("after names %v, want %v", after[auditKeyMergedFrom], source)
	}
	if _, kept := after[auditKeyFilled]; !kept {
		t.Errorf("after = %v, lost what the survivor took in", after)
	}
}
