// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"slices"
	"testing"
)

// Every source a rule declares it Needs is one the coverage vocabulary can
// actually report. The two sides live on opposite ends of a seam — the rule in
// this module, the source name in the compose coverage reader — and a typo
// ("mailbox") would make the rule's findings permanently un-clearable while
// every test that watches the guard hold kept passing.
//
// The list is the assurance_source_coverage_source_check constraint's, which
// owns the vocabulary; a source added there is added here in the same change.
func TestEveryDeclaredNeedIsACoverageSource(t *testing.T) {
	t.Parallel()
	sources := []string{"mail", "calendar", "documents", "contracts", "offers", "incumbent"}
	for _, rule := range Rules() {
		for _, need := range rule.Needs {
			if !slices.Contains(sources, need) {
				t.Errorf("rule %s needs %q, which no coverage row can ever report — "+
					"its findings would be permanently un-clearable", rule.Type, need)
			}
		}
	}
}
