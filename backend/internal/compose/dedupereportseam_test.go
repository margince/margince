// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

func TestEvidenceReadsTheCreatedRecordOnTheLeftWhicheverSideItWasStoredOn(t *testing.T) {
	stored := func() []agents.DuplicateEvidence {
		return []agents.DuplicateEvidence{{Field: "full_name", Left: "Anna Meyer", Right: "Anna Meier-Brandt"}}
	}

	if got := orientEvidence(stored(), true)[0]; got.Left != "Anna Meyer" || got.Right != "Anna Meier-Brandt" {
		t.Errorf("created on the stored left: %+v, want the stored order kept", got)
	}
	if got := orientEvidence(stored(), false)[0]; got.Left != "Anna Meier-Brandt" || got.Right != "Anna Meyer" {
		t.Errorf("created on the stored right: %+v, want the values swapped so left is the created record's", got)
	}
}
