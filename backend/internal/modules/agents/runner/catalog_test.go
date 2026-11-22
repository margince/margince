// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"strings"
	"testing"
)

// Each scheduled agent's goal states the shape of its one write. A run nobody
// watches then writes what the store accepts and a reader can find. The brief
// is one call holding every finding. A sweep note is linked to its deal, with
// the finding in its body.
func TestEachScheduledGoalStatesTheShapeOfItsWrite(t *testing.T) {
	want := map[string][]string{
		"morning_brief":           {"annotate_brief ONCE", "every item in that same call"},
		"overnight_at_risk_sweep": {"linked to that deal", "entity_type: deal", "its body states the finding"},
	}
	for _, spec := range Catalog() {
		for _, phrase := range want[spec.Name] {
			if !strings.Contains(spec.Goal, phrase) {
				t.Errorf("the %s goal does not say %q", spec.Name, phrase)
			}
		}
		delete(want, spec.Name)
	}
	for name := range want {
		t.Errorf("no scheduled agent named %s", name)
	}
}
