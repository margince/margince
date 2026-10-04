// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A written reason is bounded by one number. The contract's
// RetentionOverrideRequest says it, statedreason.Max is the Go spelling both the
// controller's overrides and the undo of a project filing use, and nothing else
// may carry its own.

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/statedreason"
)

func TestTheStatedReasonBoundIsTheContractsMaxLength(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	block := regexp.MustCompile(`(?s)\n {4}RetentionOverrideRequest:\n(.*?)\n {4}\S`).FindSubmatch(raw)
	if block == nil {
		t.Fatal("RetentionOverrideRequest is gone from the contract; the reason bound has nothing to mirror")
	}
	found := regexp.MustCompile(`reason:\s*\{[^}]*maxLength:\s*(\d+)`).FindSubmatch(block[1])
	if found == nil {
		t.Fatal("RetentionOverrideRequest.reason declares no maxLength; the Go bound has nothing to mirror")
	}
	if want, _ := strconv.Atoi(string(found[1])); want != statedreason.Max {
		t.Errorf("statedreason.Max = %d, the contract's reason maxLength = %d", statedreason.Max, want)
	}
}
