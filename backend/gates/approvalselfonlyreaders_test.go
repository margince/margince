// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"fmt"
	"sort"
	"testing"
)

const (
	approvalsPackage = "internal/modules/approvals"
	grantHalf        = "requireDecisionGrants"
	seatHalf         = "withheldFromOtherSeats"
	actionHalf       = "authorityRows"
	readableHalf     = "proposalsReadable"
)

func TestEveryApprovalsGrantFilterAlsoAppliesTheSelfOnlyNarrowing(t *testing.T) {
	t.Parallel()
	for _, violation := range approvalScopeViolations(packageCallGraph(t, approvalsPackage)) {
		t.Error(violation)
	}
}

// Staging needs action authority before a proposal exists. Row readers must
// combine that shared check with the seat and evidence checks.
func approvalScopeViolations(graph map[string]*graphFunc) []string {
	var violations []string
	for _, name := range []string{grantHalf, seatHalf, actionHalf, readableHalf, "decidableRows", "requireCallAuthority"} {
		if graph[name] == nil {
			violations = append(violations, "missing approval predicate "+name)
		}
	}
	if len(violations) > 0 {
		return violations
	}
	if !graph[actionHalf].calls[grantHalf] {
		violations = append(violations, "action authority omits decision grants")
	}
	if !graph[readableHalf].calls[seatHalf] {
		violations = append(violations, "proposal readability omits self-only narrowing")
	}
	if !graph["decidableRows"].calls[actionHalf] || !graph["decidableRows"].calls[readableHalf] {
		violations = append(violations, "decidableRows must combine action authority and proposal readability")
	}
	if !graph["requireCallAuthority"].calls[actionHalf] {
		violations = append(violations, "call admission omits action authority")
	}
	for name, fn := range graph {
		// These two return permission only. Every other consumer of either half
		// must apply the other half before it can serve an approval.
		if name == actionHalf || name == "requireCallAuthority" {
			continue
		}
		if fn.calls[grantHalf] && !fn.calls[seatHalf] {
			violations = append(violations, fmt.Sprintf("%s checks grants without self-only narrowing", name))
		}
		if fn.calls[actionHalf] && !fn.calls[readableHalf] {
			violations = append(violations, fmt.Sprintf("%s checks action authority without proposal readability", name))
		}
		if fn.calls[readableHalf] && !fn.calls[actionHalf] {
			violations = append(violations, fmt.Sprintf("%s checks readability without action authority", name))
		}
	}
	sort.Strings(violations)
	return violations
}

func TestTheApprovalScopeCensusRejectsEitherMissingHalfAndNewReaders(t *testing.T) {
	for _, defect := range []string{"none", "missing predicate", "missing grants", "missing seat", "missing action", "missing readability", "grant reader", "action reader", "readability reader"} {
		t.Run(defect, func(t *testing.T) {
			node := func(names ...string) *graphFunc {
				calls := map[string]bool{}
				for _, name := range names {
					calls[name] = true
				}
				return &graphFunc{calls: calls}
			}
			graph := map[string]*graphFunc{
				grantHalf: node(), seatHalf: node(), actionHalf: node(grantHalf), readableHalf: node(seatHalf),
				"decidableRows": node(actionHalf, readableHalf), "requireCallAuthority": node(grantHalf, actionHalf),
			}
			switch defect {
			case "missing predicate":
				delete(graph, grantHalf)
			case "missing grants":
				delete(graph[actionHalf].calls, grantHalf)
			case "missing seat":
				delete(graph[readableHalf].calls, seatHalf)
			case "missing action":
				delete(graph["decidableRows"].calls, actionHalf)
			case "missing readability":
				delete(graph["decidableRows"].calls, readableHalf)
			case "grant reader":
				graph["newReader"] = node(grantHalf)
			case "action reader":
				graph["newReader"] = node(actionHalf)
			case "readability reader":
				graph["newReader"] = node(readableHalf)
			}
			violations := approvalScopeViolations(graph)
			if (len(violations) == 0) != (defect == "none") {
				t.Fatalf("defect %s: %v", defect, violations)
			}
		})
	}
}
