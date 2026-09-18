// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H3

package gates

// What the contract says an import can receive, against what it actually can.
//
// Two places in crm.yaml stated that a mapping is validated against the
// object's live field catalog, "custom fields included". importTargets does the
// opposite, deliberately and with its reason written beside it: an import lands
// through the stores' caller-opened transaction seams, which refuse custom
// fields, so a `cf_` target would be accepted, reported as written, and
// dropped.
//
// The code was right and the text was wrong, which is the failure this holds.
// It is not a cosmetic one: `targets` is the list an agent reads to decide what
// a column can map to, so a model mapping a `cf_`-shaped column had been told
// the target exists. A promise in the contract is a promise whoever reads it
// acts on.
//
// Held from both ends, in the two places each end belongs. The behaviour half
// sits beside the behaviour, in compose's own csvfields_test.go, where the
// target table is callable and its corpus is every object that HAS one. This is
// the other end: the contract can only be held by a phrase, because no test
// reads English for meaning, so it names the claim that was false and refuses
// its return.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestTheContractDoesNotPromiseCustomFieldsToAnImport is the text half.
//
// A phrase and not a meaning, because a test cannot read English. What it holds
// is the exact claim that was false: any wording saying custom fields are
// INCLUDED in what an import may be mapped to. A future author who makes the
// promise true — by carrying custom fields through the seam — deletes this
// along with the limit it describes, which is the right order.
func TestTheContractDoesNotPromiseCustomFieldsToAnImport(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	// Any spelling of "custom field(s) included", across a line break, since
	// the contract wraps its prose.
	claim := regexp.MustCompile(`(?i)custom\s+fields?\s+included`)
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		window := line
		if i+1 < len(lines) {
			window = line + " " + strings.TrimSpace(lines[i+1])
		}
		if claim.MatchString(window) {
			t.Errorf("backend/api/crm.yaml:%d promises an import custom fields:\n\t%s\n"+
				"importTargets refuses them, so the column would be accepted, reported as "+
				"written, and dropped. Carry them through the seam or do not offer them.",
				i+1, strings.TrimSpace(line))
		}
	}
}
