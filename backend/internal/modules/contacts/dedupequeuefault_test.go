// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestPairDecisionRefusalsCarryTheContractCode(t *testing.T) {
	cases := map[string]struct {
		err  error
		code string
	}{
		"second decision": {errAlreadyDisposed("not_a_duplicate"), "already_disposed"},
		"undo of a merge": {ErrNotUndoable, "not_undoable"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fault, ok := httperr.Classify(tc.err)
			if !ok {
				t.Fatalf("Classify(%v) did not classify", tc.err)
			}
			if fault.Status != http.StatusConflict || fault.Code != tc.code {
				t.Fatalf("got %d %q, want 409 %q", fault.Status, fault.Code, tc.code)
			}
		})
	}
}
