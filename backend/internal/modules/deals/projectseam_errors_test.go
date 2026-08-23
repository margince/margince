// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deal↔project seam's refusal reaches a caller over the wire, so it carries
// the same obligation every refusal on this surface does: an index or
// constraint name describes our tables and tells a caller nothing it can act on.

import (
	"strings"
	"testing"
)

func TestTheDealProjectSeamRefusalKeepsSchemaNamesOffTheWire(t *testing.T) {
	err := &DealProjectOrgMismatchError{}
	for _, leak := range []string{dealProjectSameOrgConstraint, "uq_", "SQLSTATE"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("%T leaks %q to the caller: %q", err, leak, err.Error())
		}
	}
}
