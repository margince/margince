// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAMaskedFieldIsNeverNamedAsModelWritten(t *testing.T) {
	t.Parallel()
	written := []string{"amount_minor", "currency", "description"}
	masked := []string{"amount_minor", "currency"}
	d := crmcontracts.Deal{AiWrittenFields: &written, MaskedFields: &masked}
	withholdModelWritten(&d)
	if d.AiWrittenFields == nil || !slices.Equal(*d.AiWrittenFields, []string{"description"}) {
		t.Fatalf("ai_written_fields = %v, want only the description the reader may see", d.AiWrittenFields)
	}
	all := []string{"description"}
	d = crmcontracts.Deal{AiWrittenFields: &written, MaskedFields: &all}
	withholdModelWritten(&d)
	if d.AiWrittenFields == nil || len(*d.AiWrittenFields) != 2 {
		t.Fatalf("ai_written_fields = %v, want the two unmasked fields", d.AiWrittenFields)
	}
}
