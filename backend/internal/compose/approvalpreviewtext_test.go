// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPreviewValuesRetainTheCompleteProposedText(t *testing.T) {
	text := strings.Repeat("A paragraph to review.\n", 40) + "Final condition: payment in thirty days."
	raw, err := json.Marshal(text)
	if err != nil {
		t.Fatal(err)
	}
	value, err := (approvalPreviewReader{}).value(t.Context(), "description", raw)
	if err != nil {
		t.Fatal(err)
	}
	if value.Kind != previewValueText || value.Text != text {
		t.Fatalf("preview changed or truncated the proposed value: %q", value.Text)
	}
}
