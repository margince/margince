// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The two halves of automatic apply agree about which kinds it covers.
//
// A kind reaches an automatic apply through two places. The sweep's query
// decides what it looks for, and the applier's check decides what it admits.
// A disagreement fails silently in either direction.

import (
	"os"
	"strings"
	"testing"
)

const autoApplyFile = "internal/compose/autoapply.go"

// The sweep looks for exactly the kinds the applier will admit.
func TestTheAutoApplySweepScansTheKindsTheApplierAdmits(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(autoApplyFile)
	if err != nil {
		t.Fatalf("reading %s: %v", autoApplyFile, err)
	}
	src := string(raw)
	for _, set := range []string{"approvals.SortedAutoApplyKinds()", "approvals.AutoApplyKinds["} {
		if !strings.Contains(src, set) {
			t.Errorf("%s no longer reads %s: the sweep and the applier read "+
				"different sets, so a kind is either never looked for or never "+
				"admitted", autoApplyFile, set)
		}
	}
}
