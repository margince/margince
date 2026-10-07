// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// An outside service's answer is text a remote party chose: its vendor, its
// account and its limits. A caller is told the service gave no usable
// answer, and the cause stays in the log.
func TestAnUnusableProviderAnswerDoesNotCarryTheProvidersText(t *testing.T) {
	cause := fmt.Errorf("every bound tier failed for draft: Rate limit reached for gpt-4o in account acct-XXXX: %w",
		apperrors.ErrProviderUnusable)

	fault, ok := Classify(cause)
	if !ok {
		t.Fatal("an unusable provider answer is not classified")
	}
	if fault.Code != "provider_unusable" {
		t.Errorf("code = %q, want provider_unusable", fault.Code)
	}
	if strings.Contains(fault.Detail, "acct-XXXX") || strings.Contains(fault.Detail, "gpt-4o") {
		t.Errorf("the caller is handed the provider's own text: %q", fault.Detail)
	}
}
