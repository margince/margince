// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"
)

// A view's path value that spells a fixed sibling route lands on that route. So
// a view must have no human-only sibling for its id to reach.
func TestNoViewRouteHasAHumanOnlySibling(t *testing.T) {
	views := 0
	for route, view := range agentPolicies {
		parent, isIDRoute := strings.CutSuffix(route, "/{id}")
		if !isIDRoute || view.Tool != "read_view" {
			continue
		}
		views++
		for sibling, policy := range agentPolicies {
			name, isChild := strings.CutPrefix(sibling, parent+"/")
			if isChild && !strings.ContainsAny(name, "/{") && policy.Access == accessHumanOnly {
				t.Errorf("%s is human-only, and read_view reaches it through %s", sibling, route)
			}
		}
	}
	if views == 0 {
		t.Fatal("no view takes an id, so this checked nothing")
	}
}
