// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"strings"
	"testing"
)

// The reference the store keys on is the trimmed one, so its length is judged
// after trimming: padding that trims away cannot refuse a short reference.
func TestASourceReferenceIsMeasuredAfterTrimming(t *testing.T) {
	base := IngestSourceInput{Kind: "email", SourceLabel: "Sent mail", Content: "Hello there."}

	padded := base
	padded.SourceRef = strings.Repeat(" ", maxSourceRefChars+1) + "ref-1"
	if _, _, err := validateDeclaredSource(padded); err != nil {
		t.Errorf("a short reference padded with spaces was refused: %v", err)
	}

	long := base
	long.SourceRef = strings.Repeat("x", maxSourceRefChars+1)
	if _, _, err := validateDeclaredSource(long); err == nil {
		t.Error("a reference over the bound was accepted")
	}
}
