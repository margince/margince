// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The contract counts a revision reason in characters, and a reason must say
// something a reader can see.
func TestAFrameworkReasonIsBoundedInCharactersAndMustBeVisible(t *testing.T) {
	input := func(reason string) crmcontracts.ReportingFrameworkInput {
		return crmcontracts.ReportingFrameworkInput{Reason: reason, Template: reportingSales}
	}
	if err := validateFrameworkInput(input(strings.Repeat("é", 1000))); err != nil {
		t.Fatalf("1000 characters (2000 bytes) refused: %v", err)
	}
	for name, reason := range map[string]string{
		"1001 characters": strings.Repeat("é", 1001),
		"zero-width":      string(rune(0x200B)),
		"blank":           "  ",
	} {
		if err := validateFrameworkInput(input(reason)); err == nil {
			t.Errorf("%s reason was accepted", name)
		}
	}
}
