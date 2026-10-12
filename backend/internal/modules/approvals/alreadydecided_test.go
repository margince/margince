// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The tool surface renders httperr.Classify's verdict, so a decision that
// classifies as 409 already_decided here is the same answer on REST and MCP.
func TestDecidingADecidedApprovalClassifiesAsAlreadyDecided(t *testing.T) {
	err := fmt.Errorf("decide: %w", &AlreadyDecidedError{Status: "approved"})

	fault, ok := httperr.Classify(err)
	if !ok {
		t.Fatal("an already-decided approval fell outside the taxonomy, so MCP reports an internal fault")
	}
	if fault.Status != http.StatusConflict || fault.Code != "already_decided" {
		t.Errorf("got %d %q, want %d already_decided", fault.Status, fault.Code, http.StatusConflict)
	}
	if fault.Detail != "approval is already approved" {
		t.Errorf("detail %q does not name the standing verdict", fault.Detail)
	}
}
